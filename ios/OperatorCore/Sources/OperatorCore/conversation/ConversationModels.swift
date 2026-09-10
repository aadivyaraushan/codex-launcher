import Foundation

public enum ChatRole: String, Codable, Sendable {
    case user
    case assistant
    case system
}

public enum MessageDelivery: String, Codable, Sendable {
    case waiting
    case sending
    case accepted
    case failed
}

public struct ChatMessage: Codable, Equatable, Identifiable, Sendable {
    public let id: UUID
    public let role: ChatRole
    public var text: String
    public let createdAt: Date
    public var delivery: MessageDelivery

    public init(
        id: UUID = UUID(),
        role: ChatRole,
        text: String,
        createdAt: Date = Date(),
        delivery: MessageDelivery = .accepted)
    {
        self.id = id
        self.role = role
        self.text = text
        self.createdAt = createdAt
        self.delivery = delivery
    }
}

public enum OutboxState: String, Codable, Sendable {
    case waiting
    case sending
    case failed
}

public struct OutboxEntry: Codable, Equatable, Identifiable, Sendable {
    public let id: UUID
    public let messageID: UUID
    public let text: String
    public let idempotencyKey: String
    public var state: OutboxState

    public init(
        id: UUID,
        messageID: UUID,
        text: String,
        idempotencyKey: String,
        state: OutboxState)
    {
        self.id = id
        self.messageID = messageID
        self.text = text
        self.idempotencyKey = idempotencyKey
        self.state = state
    }
}

public struct ConversationSnapshot: Codable, Equatable, Sendable {
    public var messages: [ChatMessage]
    public var draft: String
    public var outbox: [OutboxEntry]

    public init(
        messages: [ChatMessage] = [],
        draft: String = "",
        outbox: [OutboxEntry] = [])
    {
        self.messages = messages
        self.draft = draft
        self.outbox = outbox
    }
}
