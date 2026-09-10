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
