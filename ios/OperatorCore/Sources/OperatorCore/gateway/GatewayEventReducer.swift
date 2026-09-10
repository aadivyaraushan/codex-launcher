import Foundation

public struct GatewayChatEvent: Decodable, Equatable, Sendable {
    public enum State: String, Codable, Sendable {
        case status
        case delta
        case final
        case error
        case aborted
    }

    public let runID: String
    public let sessionKey: String
    public let sequence: Int
    public let state: State
    public let deltaText: String?
    public let replace: Bool
    public let messageText: String?
    public let errorMessage: String?

    public init(
        runID: String,
        sessionKey: String,
        sequence: Int,
        state: State,
        deltaText: String? = nil,
        replace: Bool = false,
        messageText: String? = nil,
        errorMessage: String? = nil)
    {
        self.runID = runID
        self.sessionKey = sessionKey
        self.sequence = sequence
        self.state = state
        self.deltaText = deltaText
        self.replace = replace
        self.messageText = messageText
        self.errorMessage = errorMessage
    }

    private enum CodingKeys: String, CodingKey {
        case runID = "runId"
        case sessionKey
        case sequence = "seq"
        case state
        case deltaText
        case replace
        case message
        case errorMessage
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        self.runID = try container.decode(String.self, forKey: .runID)
        self.sessionKey = try container.decode(String.self, forKey: .sessionKey)
        self.sequence = try container.decode(Int.self, forKey: .sequence)
        self.state = try container.decode(State.self, forKey: .state)
        self.deltaText = try container.decodeIfPresent(String.self, forKey: .deltaText)
        self.replace = try container.decodeIfPresent(Bool.self, forKey: .replace) ?? false
        self.errorMessage = try container.decodeIfPresent(String.self, forKey: .errorMessage)
        if let text = try? container.decode(String.self, forKey: .message) {
            self.messageText = text
        } else if let projection = try? container.decode(MessageProjection.self, forKey: .message) {
            self.messageText = projection.visibleText
        } else {
            self.messageText = nil
        }
    }
}

private struct MessageProjection: Decodable {
    struct Content: Decodable {
        let type: String?
        let text: String?
    }

    let text: String?
    let content: [Content]?

    var visibleText: String? {
        if let text, !text.isEmpty {
            return text
        }
        let joined = (self.content ?? [])
            .filter { $0.type == nil || $0.type == "text" }
            .compactMap(\.text)
            .joined()
        return joined.isEmpty ? nil : joined
    }
}

public enum GatewayConversationEvent: Equatable, Sendable {
    case working(runID: String)
    case stream(runID: String, text: String)
    case reply(runID: String, text: String)
    case failed(runID: String, message: String)
    case stopped(runID: String)
}

public struct GatewayEventReducer: Sendable {
    private struct Run: Sendable {
        var text = ""
        var lastSequence = -1
        var announcedWorking = false
    }

    private let sessionKey: String
    private var runs: [String: Run] = [:]
    private var finished = Set<String>()
    private var finishedOrder: [String] = []
    private let finishedLimit = 128

    public init(sessionKey: String) {
        self.sessionKey = sessionKey
    }

    public mutating func apply(_ event: GatewayChatEvent) -> [GatewayConversationEvent] {
        guard event.sessionKey == self.sessionKey, !self.finished.contains(event.runID) else {
            return []
        }
        var run = self.runs[event.runID] ?? Run()
        guard event.sequence > run.lastSequence else {
            return []
        }
        run.lastSequence = event.sequence
        var output: [GatewayConversationEvent] = []
        if !run.announcedWorking {
            run.announcedWorking = true
            output.append(.working(runID: event.runID))
        }

        switch event.state {
        case .status:
            self.runs[event.runID] = run
        case .delta:
            if event.replace {
                run.text = event.deltaText ?? ""
            } else {
                run.text += event.deltaText ?? ""
            }
            self.runs[event.runID] = run
            if !run.text.isEmpty {
                output.append(.stream(runID: event.runID, text: run.text))
            }
        case .final:
            let streamedReply = run.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                ? nil
                : run.text
            let reply = streamedReply ?? event.messageText
            if let reply, !reply.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                output.append(.reply(runID: event.runID, text: reply))
            } else {
                output.append(.failed(
                    runID: event.runID,
                    message: "No readable result was received. Completion isn't verified."))
            }
            self.finish(event.runID)
        case .error:
            output.append(.failed(
                runID: event.runID,
                message: event.errorMessage ?? "Operator hit an error"))
            self.finish(event.runID)
        case .aborted:
            output.append(.stopped(runID: event.runID))
            self.finish(event.runID)
        }
        return output
    }

    private mutating func finish(_ runID: String) {
        self.runs.removeValue(forKey: runID)
        self.finished.insert(runID)
        self.finishedOrder.append(runID)
        if self.finishedOrder.count > self.finishedLimit {
            let oldest = self.finishedOrder.removeFirst()
            self.finished.remove(oldest)
        }
    }
}
