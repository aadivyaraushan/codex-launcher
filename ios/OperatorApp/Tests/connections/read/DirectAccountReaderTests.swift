import Foundation
import XCTest
@testable import OperatorApp

final class DirectAccountReaderTests: XCTestCase {
    func testGoogleCalendarBuildsBoundedReadOnlyRequestAfterValidation() async throws {
        let transport = ReadFixtureTransport(body: #"{"items":[],"nextPageToken":"cursor-2"}"#)
        let reader = DirectAccountReader(transport: transport, bearer: { _ in "token" })
        let page = try await reader.read(.init(operation: .googleCalendarEvents, query: "standup", channel: nil, timeMin: "2026-09-09T00:00:00Z", timeMax: "2026-09-10T00:00:00Z", limit: 5, cursor: nil))
        let captured = await transport.request
        let request = try XCTUnwrap(captured)
        let query = URLComponents(url: try XCTUnwrap(request.url), resolvingAgainstBaseURL: false)?.queryItems
        XCTAssertEqual(request.httpMethod, "GET")
        XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer token")
        XCTAssertEqual(query?.value(for: "q"), "standup")
        XCTAssertEqual(query?.value(for: "maxResults"), "5")
        XCTAssertEqual(page.nextCursor, "cursor-2")
    }

    func testInvalidInputAndForeignCursorNeverRequestBearer() async {
        let transport = ReadFixtureTransport(body: "{}")
        let calls = TokenCalls()
        let reader = DirectAccountReader(transport: transport, bearer: { _ in await calls.called(); return "token" })
        await XCTAssertThrowsErrorAsync(try await reader.read(.init(operation: .spotifySearch, query: "", channel: nil, timeMin: nil, timeMax: nil, limit: 4, cursor: nil)))
        await XCTAssertThrowsErrorAsync(try await reader.read(.init(operation: .googleDriveFiles, query: "x", channel: nil, timeMin: nil, timeMax: nil, limit: 4, cursor: "https://evil.invalid")))
        let callCount = await calls.value
        let request = await transport.request
        XCTAssertEqual(callCount, 0)
        XCTAssertNil(request)
    }

    func testSlackFalseOKAndHttpStatusesAreSanitized() async throws {
        let slack = DirectAccountReader(transport: ReadFixtureTransport(body: #"{"ok":false,"error":"not_authed"}"#), bearer: { _ in "token" })
        await XCTAssertThrowsErrorAsync(try await slack.read(.init(operation: .slackChannels, query: nil, channel: nil, timeMin: nil, timeMax: nil, limit: 2, cursor: nil))) { error in
            XCTAssertEqual(error as? AccountReadError, .unavailable)
        }
        let unauthorized = DirectAccountReader(transport: ReadFixtureTransport(status: 401, body: "{}"), bearer: { _ in "token" })
        await XCTAssertThrowsErrorAsync(try await unauthorized.read(.init(operation: .spotifyPlayback, query: nil, channel: nil, timeMin: nil, timeMax: nil, limit: 1, cursor: nil))) { error in
            XCTAssertEqual(error as? AccountReadError, .notConnected)
        }
    }

    func testCalendarWindowIsRFC3339AndOrderedAndSpotifyEmptyPlaybackIsValid() async throws {
        let reader = DirectAccountReader(transport: ReadFixtureTransport(status: 204, body: ""), bearer: { _ in "token" })
        let empty = try await reader.read(.init(operation: .spotifyPlayback, query: nil, channel: nil, timeMin: nil, timeMax: nil, limit: 1, cursor: nil))
        XCTAssertEqual(empty.count, 0)
        let calendar = DirectAccountReader(transport: ReadFixtureTransport(body: #"{"items":[]}"#), bearer: { _ in "token" })
        await XCTAssertThrowsErrorAsync(try await calendar.read(.init(operation: .googleCalendarEvents, query: nil, channel: nil, timeMin: "2026-09-10", timeMax: "2026-09-09", limit: 1, cursor: nil)))
    }

    // --- outlookCalendarEvents -------------------------------------------

    func testOutlookCalendarUsesCalendarViewSoRecurringSeriesAreExpanded() async throws {
        let transport = ReadFixtureTransport(body: #"{"value":[]}"#)
        let reader = DirectAccountReader(transport: transport, bearer: { _ in "token" })

        _ = try await reader.read(.init(operation: .outlookCalendarEvents, query: nil, channel: nil, timeMin: "2026-09-01T00:00:00Z", timeMax: "2026-09-08T00:00:00Z", limit: 5, cursor: nil))

        let request = try XCTUnwrap(await transport.request)
        let components = URLComponents(url: try XCTUnwrap(request.url), resolvingAgainstBaseURL: false)
        XCTAssertEqual(request.httpMethod, "GET")
        XCTAssertEqual(components?.host, "graph.microsoft.com")
        // calendarView, not /events. Graph expands a recurring series only
        // here; against /events a weekly standup appears once, on the day it
        // was created. This is the Graph equivalent of Google's singleEvents.
        XCTAssertEqual(components?.path, "/v1.0/me/calendarView")
        XCTAssertEqual(components?.queryItems?.value(for: "startDateTime"), "2026-09-01T00:00:00Z")
        XCTAssertEqual(components?.queryItems?.value(for: "endDateTime"), "2026-09-08T00:00:00Z")
        XCTAssertEqual(components?.queryItems?.value(for: "$top"), "5")
        XCTAssertEqual(components?.queryItems?.value(for: "$orderby"), "start/dateTime")
    }

    func testOutlookCalendarReturnsOnlyAllowlistedKeys() async throws {
        let body = #"""
        {"value":[{"id":"AAA","subject":"Standup","start":{"dateTime":"2026-09-02T09:00:00"},"end":{"dateTime":"2026-09-02T09:15:00"},"isAllDay":false,"location":{"displayName":"Room 1"},"webLink":"https://outlook.office.com/x","organizer":{"emailAddress":{"name":"A"}},"bodyPreview":"private agenda","attendees":[{"emailAddress":{"address":"b@example.com"}}]}]}
        """#
        let reader = DirectAccountReader(transport: ReadFixtureTransport(body: body), bearer: { _ in "token" })

        let page = try await reader.read(.init(operation: .outlookCalendarEvents, query: nil, channel: nil, timeMin: "2026-09-01T00:00:00Z", timeMax: "2026-09-08T00:00:00Z", limit: 5, cursor: nil))

        XCTAssertEqual(page.count, 1)
        let rows = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(page.payloadJSON.utf8)) as? [[String: Any]])
        XCTAssertEqual(Set(rows[0].keys), ["id", "subject", "start", "end", "location", "isAllDay", "webLink", "organizer"])
        // Both were in the response and neither reaches the agent. The
        // allowlist decides what may be seen, not what Graph chose to send.
        XCTAssertNil(rows[0]["bodyPreview"])
        XCTAssertNil(rows[0]["attendees"])
    }

    func testOutlookCalendarRefusesAMissingOrInvertedWindowWithoutCallingOut() async {
        let windows: [(String?, String?)] = [
            (nil, "2026-09-08T00:00:00Z"),
            ("2026-09-01T00:00:00Z", nil),
            ("2026-09-08T00:00:00Z", "2026-09-01T00:00:00Z"),
            ("not-a-date", "2026-09-08T00:00:00Z"),
        ]
        for (timeMin, timeMax) in windows {
            let transport = ReadFixtureTransport(body: "{}")
            let reader = DirectAccountReader(transport: transport, bearer: { _ in "token" })

            await XCTAssertThrowsErrorAsync(try await reader.read(.init(operation: .outlookCalendarEvents, query: nil, channel: nil, timeMin: timeMin, timeMax: timeMax, limit: 5, cursor: nil))) { error in
                XCTAssertEqual(error as? AccountReadError, .invalidRequest, "window=\(String(describing: timeMin))..\(String(describing: timeMax))")
            }
            let request = await transport.request
            XCTAssertNil(request, "a refused request must never reach the network")
        }
    }

    // A nextLink is a URL the server chose. Following one unchecked is how a
    // paging cursor becomes a redirect, so only the exact host, the exact
    // path and a non-negative $skip survive.
    func testOutlookCalendarAcceptsOnlyItsOwnNextLinkAsACursor() async throws {
        let good = DirectAccountReader(
            transport: ReadFixtureTransport(body: #"{"value":[],"@odata.nextLink":"https://graph.microsoft.com/v1.0/me/calendarView?$skip=5"}"#),
            bearer: { _ in "token" })
        let page = try await good.read(.init(operation: .outlookCalendarEvents, query: nil, channel: nil, timeMin: "2026-09-01T00:00:00Z", timeMax: "2026-09-08T00:00:00Z", limit: 5, cursor: nil))
        XCTAssertEqual(page.nextCursor, "5")

        for link in [
            "https://evil.example.com/v1.0/me/calendarView?$skip=5",
            "https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages?$skip=5",
            "https://graph.microsoft.com/v1.0/me/calendarView?$skip=-1",
            "https://graph.microsoft.com/v1.0/me/calendarView",
        ] {
            let reader = DirectAccountReader(
                transport: ReadFixtureTransport(body: "{\"value\":[],\"@odata.nextLink\":\"\(link)\"}"),
                bearer: { _ in "token" })
            await XCTAssertThrowsErrorAsync(try await reader.read(.init(operation: .outlookCalendarEvents, query: nil, channel: nil, timeMin: "2026-09-01T00:00:00Z", timeMax: "2026-09-08T00:00:00Z", limit: 5, cursor: nil))) { error in
                XCTAssertEqual(error as? AccountReadError, .invalidResponse, "link=\(link)")
            }
        }
    }

    func testOutlookCalendarRefusesMoreRowsThanWereAskedFor() async {
        let rows = (0 ..< 6).map { "{\"id\":\"\($0)\",\"subject\":\"s\"}" }.joined(separator: ",")
        let reader = DirectAccountReader(
            transport: ReadFixtureTransport(body: "{\"value\":[\(rows)]}"),
            bearer: { _ in "token" })

        await XCTAssertThrowsErrorAsync(try await reader.read(.init(operation: .outlookCalendarEvents, query: nil, channel: nil, timeMin: "2026-09-01T00:00:00Z", timeMax: "2026-09-08T00:00:00Z", limit: 5, cursor: nil))) { error in
            XCTAssertEqual(error as? AccountReadError, .invalidResponse)
        }
    }

    func testCalendarsReadRidesTheMicrosoftClientThatAlreadySignsIn() {
        XCTAssertEqual(AccountReadOperation.outlookCalendarEvents.provider, .microsoftOutlook)
        XCTAssertTrue(OAuthProvider.microsoftOutlook.scopes.contains("Calendars.Read"))
        XCTAssertTrue(OAuthProvider.microsoftOutlook.requiredAccessTokenScopes.contains("Calendars.Read"))
        // Sign-in metadata is not an API permission and must stay out of the
        // access-token check, or a valid token reads as an incomplete one.
        XCTAssertFalse(OAuthProvider.microsoftOutlook.requiredAccessTokenScopes.contains("openid"))
        XCTAssertFalse(OAuthProvider.microsoftOutlook.requiredAccessTokenScopes.contains("offline_access"))
    }
}

private actor TokenCalls { private var count = 0; func called() { self.count += 1 }; var value: Int { self.count } }
private actor ReadFixtureTransport: PhoneHTTPTransport {
    let status: Int; let body: String; var request: URLRequest?
    init(status: Int = 200, body: String) { self.status = status; self.body = body }
    func data(for request: URLRequest) async throws -> (Data, URLResponse) {
        self.request = request
        return (Data(self.body.utf8), HTTPURLResponse(url: request.url!, statusCode: self.status, httpVersion: nil, headerFields: nil)!)
    }
}
private extension Array where Element == URLQueryItem { func value(for name: String) -> String? { first { $0.name == name }?.value } }
private func XCTAssertThrowsErrorAsync<T>(_ expression: @autoclosure () async throws -> T, _ handler: @escaping (Error) -> Void = { _ in }) async { do { _ = try await expression(); XCTFail("Expected error") } catch { handler(error) } }
