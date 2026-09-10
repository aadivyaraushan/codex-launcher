import Foundation
import OperatorCore
import OSLog

@MainActor
protocol AppHandoffOpener: AnyObject {
    func open(_ url: URL) async -> Bool
}

enum AppHandoffCatalog {
    private struct Entry: Decodable {
        let id: String
        let url: String?
        let verification: String?
    }
    enum InvalidCatalog: Error { case invalidDestination }

    static func decode(_ data: Data) throws -> [String: URL] {
        let entries = try JSONDecoder().decode([Entry].self, from: data)
        var result: [String: URL] = [:]
        var seen = Set<String>()
        for entry in entries {
            guard !entry.id.isEmpty, seen.insert(entry.id).inserted else { throw InvalidCatalog.invalidDestination }
            guard let raw = entry.url else {
                guard ["nativeHandled", "excluded"].contains(entry.verification) else { throw InvalidCatalog.invalidDestination }
                continue
            }
            guard let url = URL(string: raw), isSafeDestination(url) else { throw InvalidCatalog.invalidDestination }
            result[entry.id] = url
        }
        return result
    }

    static func isSafeDestination(_ url: URL) -> Bool {
        guard let parts = URLComponents(url: url, resolvingAgainstBaseURL: false) else { return false }
        return parts.scheme == "https" && !(parts.host ?? "").isEmpty
            && parts.user == nil && parts.password == nil && parts.port == nil
            && parts.query == nil && parts.fragment == nil
    }
}

@MainActor
final class ForegroundAppHandoffService: GatewayNodeCommandHandler {
    private let destinations: [String: URL]
    private let opener: any AppHandoffOpener
    private let isAppActive: @MainActor @Sendable () -> Bool
    private let logger = Logger(subsystem: "app.operator.ios", category: "app-handoff")

    init(destinations: [String: URL], opener: any AppHandoffOpener, isAppActive: @escaping @MainActor @Sendable () -> Bool) {
        self.destinations = destinations
        self.opener = opener
        self.isAppActive = isAppActive
    }
    func handleNodeCommand(_ command: String, paramsJSON: String?, timeoutMilliseconds: Int?) async -> GatewayNodeCommandResult {
        guard command == "apps.open" else {
            return .failure(code: "UNSUPPORTED_COMMAND", message: "This iPhone node does not support \(command)")
        }
        guard let paramsJSON, paramsJSON.utf8.count <= 32768,
              let raw = try? JSONSerialization.jsonObject(with: Data(paramsJSON.utf8)),
              let params = raw as? [String: Any],
              Set(params.keys).isSubset(of: ["appID", "draft"]),
              let appID = params["appID"] as? String,
              params["draft"] == nil || params["draft"] is String
        else {
            self.logger.info("[app-handoff] rejected invalid parameter shape")
            return .failure(code: "INVALID_REQUEST", message: "apps.open requires appID and an optional draft; URLs are not accepted")
        }
        guard let url = self.destinations[appID], AppHandoffCatalog.isSafeDestination(url) else {
            self.logger.info("[app-handoff] rejected unavailable destination")
            return .failure(code: "APP_UNAVAILABLE", message: "This app has no supported website hand-off")
        }
        guard self.isAppActive() else {
            self.logger.info("[app-handoff] rejected while Operator inactive")
            return .failure(code: "APP_NOT_ACTIVE", message: "Open Operator before opening another app or website")
        }
        self.logger.info("[app-handoff] opening listed website; draft stays in chat")
        guard await self.opener.open(url) else {
            self.logger.error("[app-handoff] system refused website open")
            return .failure(code: "OPEN_FAILED", message: "The website could not be opened; nothing was completed")
        }
        self.logger.info("[app-handoff] website opened actionCompleted=false draftTransferred=false")
        return .success(payloadJSON: #"{"opened":true,"destinationKind":"website","actionCompleted":false,"draftTransferred":false,"nextStep":"Only the listed website was opened. Keep the prepared draft in chat. The owner must complete any action in the destination."}"#)
    }
}
