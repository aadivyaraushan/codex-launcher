import Foundation
import XCTest
@testable import OperatorCore

final class ConversationStoreTests: XCTestCase {
    func testStageUserMessagePersistsMessageDraftAndStableOutboxIdentity() async throws {
        let fileURL = temporaryFileURL()
        let store = ConversationStore(fileURL: fileURL)

        try await store.updateDraft("book a table")
        let staged = try await store.stageUserMessage(text: "book a table")

        XCTAssertEqual(staged.messages.map(\.text), ["book a table"])
        XCTAssertEqual(staged.messages.first?.delivery, .waiting)
        XCTAssertEqual(staged.draft, "")
        XCTAssertEqual(staged.outbox.count, 1)
        XCTAssertEqual(staged.outbox.first?.messageID, staged.messages.first?.id)
        XCTAssertEqual(staged.outbox.first?.idempotencyKey, staged.messages.first?.id.uuidString.lowercased())

        let relaunched = ConversationStore(fileURL: fileURL)
        let restored = try await relaunched.load()
        XCTAssertEqual(restored, staged)
    }

    func testSendingEntryReturnsToWaitingAfterRelaunchWithoutChangingItsKey() async throws {
        let fileURL = temporaryFileURL()
        let store = ConversationStore(fileURL: fileURL)
        let staged = try await store.stageUserMessage(text: "send once")
        let entry = try XCTUnwrap(staged.outbox.first)

        _ = try await store.markSending(entryID: entry.id)

        let relaunched = ConversationStore(fileURL: fileURL)
        let restored = try await relaunched.load()
        XCTAssertEqual(restored.outbox.first?.state, .waiting)
        XCTAssertEqual(restored.outbox.first?.idempotencyKey, entry.idempotencyKey)
        XCTAssertEqual(restored.messages.first?.delivery, .waiting)
    }

    func testActiveConnectionFailureReturnsEntryToWaitingWithoutChangingItsKey() async throws {
        let store = ConversationStore(fileURL: temporaryFileURL())
        let staged = try await store.stageUserMessage(text: "retry after reconnect")
        let entry = try XCTUnwrap(staged.outbox.first)
        _ = try await store.markSending(entryID: entry.id)

        let waiting = try await store.markWaiting(entryID: entry.id)

        XCTAssertEqual(waiting.outbox.first?.state, .waiting)
        XCTAssertEqual(waiting.outbox.first?.idempotencyKey, entry.idempotencyKey)
        XCTAssertEqual(waiting.messages.first?.delivery, .waiting)
    }

    func testAcceptedSendLeavesReadableMessageAndRemovesOutboxEntry() async throws {
        let store = ConversationStore(fileURL: temporaryFileURL())
        let staged = try await store.stageUserMessage(text: "hello")
        let entry = try XCTUnwrap(staged.outbox.first)

        let accepted = try await store.markAccepted(entryID: entry.id)

        XCTAssertTrue(accepted.outbox.isEmpty)
        XCTAssertEqual(accepted.messages.first?.delivery, .accepted)
    }

    func testCallerSuppliedMessageIdentitySurvivesPersistenceAndRetry() async throws {
        let messageID = UUID(uuidString: "11111111-2222-3333-4444-555555555555")!
        let fileURL = temporaryFileURL()
        let store = ConversationStore(fileURL: fileURL)

        let staged = try await store.stageUserMessage(
            id: messageID,
            text: "keep this identity")
        let restored = try await ConversationStore(fileURL: fileURL).load()

        XCTAssertEqual(staged.messages.first?.id, messageID)
        XCTAssertEqual(staged.outbox.first?.id, messageID)
        XCTAssertEqual(restored.outbox.first?.idempotencyKey, messageID.uuidString.lowercased())
    }

    private func temporaryFileURL() -> URL {
        FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
            .appendingPathComponent("conversation.json", isDirectory: false)
    }
}
