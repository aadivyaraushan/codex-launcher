import Foundation
import OperatorCore
import XCTest
@testable import OperatorApp

final class ForegroundAccountReadServiceTests: XCTestCase {
    @MainActor
    func testWrongTypeOptionalFieldsNeverRequestCredentials() async {
        let service = ForegroundAccountReadService(reader: DirectAccountReader(bearer: { _ in
            XCTFail("invalid input must not request credentials")
            throw AccountReadError.notConnected
        }))
        for field in ["query", "channel", "timeMin", "timeMax", "cursor"] {
            let result = await service.handleNodeCommand("connections.read", paramsJSON: "{\"operation\":\"spotifyPlayback\",\"limit\":1,\"\(field)\":true}", timeoutMilliseconds: nil)
            XCTAssertEqual(result, .failure(code: "INVALID_REQUEST", message: "Connection read parameters were invalid"))
        }
    }

    @MainActor
    func testBecomingInactiveDuringReadDiscardsResults() async {
        let active = ActiveBox()
        let transport = ServiceTransport(body: #"{"items":[]}"#, beforeResponse: { await MainActor.run { active.value = false } })
        let service = ForegroundAccountReadService(reader: DirectAccountReader(transport: transport, bearer: { _ in "token" }), isAppActive: { active.value })
        let result = await service.handleNodeCommand("connections.read", paramsJSON: #"{"operation":"googleCalendarEvents","limit":1,"timeMin":"2026-09-09T00:00:00Z","timeMax":"2026-09-10T00:00:00Z"}"#, timeoutMilliseconds: nil)
        XCTAssertEqual(result, .failure(code: "APP_NOT_ACTIVE", message: "Open Operator to read connected accounts"))
    }
    @MainActor
    func testStrictInputAndPostAwaitInactiveGuard() async {
        let transport = ServiceTransport(body: #"{"items":[{"id":"1","summary":"ok","secret":"drop"}]}"#)
        let active = ActiveBox()
        let service = ForegroundAccountReadService(reader: DirectAccountReader(transport: transport, bearer: { _ in "token" }), isAppActive: { active.value })
        let malformed = await service.handleNodeCommand("connections.read", paramsJSON: #"{"operation":"googleCalendarEvents","limit":true,"timeMin":"2026-09-09T00:00:00Z","timeMax":"2026-09-10T00:00:00Z"}"#, timeoutMilliseconds: nil)
        XCTAssertEqual(malformed, .failure(code: "INVALID_REQUEST", message: "Connection read parameters were invalid"))
        active.value = false
        let inactive = await service.handleNodeCommand("connections.read", paramsJSON: #"{"operation":"googleCalendarEvents","limit":1,"timeMin":"2026-09-09T00:00:00Z","timeMax":"2026-09-10T00:00:00Z"}"#, timeoutMilliseconds: nil)
        XCTAssertEqual(inactive, .failure(code: "APP_NOT_ACTIVE", message: "Open Operator to read connected accounts"))
    }

    @MainActor
    func testServiceReturnsBoundedSanitizedPage() async {
        let transport = ServiceTransport(body: #"{"items":[{"id":"1","summary":"ok","secret":"drop"}]}"#)
        let service = ForegroundAccountReadService(reader: DirectAccountReader(transport: transport, bearer: { _ in "token" }))
        let result = await service.handleNodeCommand("connections.read", paramsJSON: #"{"operation":"googleCalendarEvents","limit":1,"timeMin":"2026-09-09T00:00:00Z","timeMax":"2026-09-10T00:00:00Z"}"#, timeoutMilliseconds: nil)
        guard case let .success(payload) = result else { return XCTFail("expected success") }
        XCTAssertTrue(payload.contains("summary")); XCTAssertFalse(payload.contains("secret"))
    }

    @MainActor
    func testCancelledCommandIsNotStarted() async {
        let service = ForegroundAccountReadService(reader: DirectAccountReader(bearer: { _ in XCTFail("cancelled command must not request a token"); return "token" }))
        let task = Task { @MainActor in
            await withTaskCancellationHandler(operation: {
                await service.handleNodeCommand("connections.read", paramsJSON: #"{"operation":"spotifyPlayback","limit":1}"#, timeoutMilliseconds: nil)
            }, onCancel: {})
        }
        task.cancel()
        let result = await task.value
        XCTAssertEqual(result, .failure(code: "CANCELLED", message: "Connection read was cancelled"))
    }
}

private actor ServiceTransport: PhoneHTTPTransport {
    let body: String
    let beforeResponse: @Sendable () async -> Void
    init(body: String, beforeResponse: @escaping @Sendable () async -> Void = {}) { self.body = body; self.beforeResponse = beforeResponse }
    func data(for request: URLRequest) async throws -> (Data, URLResponse) {
        await beforeResponse()
        return (Data(body.utf8), HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
    }
}

@MainActor private final class ActiveBox { var value = true }
