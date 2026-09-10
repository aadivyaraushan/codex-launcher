import Foundation
import OperatorCore
import OSLog
#if canImport(UIKit)
import UIKit
#endif

@MainActor
final class ForegroundAccountReadService: GatewayNodeCommandHandler {
    private let reader: DirectAccountReader
    private let isAppActive: @MainActor @Sendable () -> Bool
    private let logger = Logger(subsystem: "app.operator.ios", category: "account-read-service")

    init(reader: DirectAccountReader, isAppActive: @escaping @MainActor @Sendable () -> Bool = {
        #if canImport(UIKit)
        UIApplication.shared.applicationState == .active
        #else
        true
        #endif
    }) { self.reader = reader; self.isAppActive = isAppActive }

    func handleNodeCommand(_ command: String, paramsJSON: String?, timeoutMilliseconds _: Int?) async -> GatewayNodeCommandResult {
        guard !Task.isCancelled else { return .failure(code: "CANCELLED", message: "Connection read was cancelled") }
        guard command == "connections.read" else { return .failure(code: "UNSUPPORTED_COMMAND", message: "This iPhone node does not support \(command)") }
        guard self.isAppActive() else { return .failure(code: "APP_NOT_ACTIVE", message: "Open Operator to read connected accounts") }
        guard let input = Self.input(paramsJSON) else { return .failure(code: "INVALID_REQUEST", message: "Connection read parameters were invalid") }
        do {
            let page = try await self.reader.read(input)
            guard !Task.isCancelled else { return .failure(code: "CANCELLED", message: "Connection read was cancelled") }
            guard self.isAppActive() else { return .failure(code: "APP_NOT_ACTIVE", message: "Open Operator to read connected accounts") }
            let payload = ["count": page.count, "nextCursor": page.nextCursor as Any, "page": try JSONSerialization.jsonObject(with: Data(page.payloadJSON.utf8))] as [String : Any]
            let data = try JSONSerialization.data(withJSONObject: payload, options: [.sortedKeys])
            return .success(payloadJSON: String(decoding: data, as: UTF8.self))
        } catch let error as AccountReadError {
            self.logger.info("[account-read-service] rejected result=\(String(describing: error), privacy: .public)")
            switch error { case .invalidRequest: return .failure(code:"INVALID_REQUEST",message:"Connection read parameters were invalid"); case .notConnected: return .failure(code:"NOT_CONNECTED",message:"Connect this account before reading it"); case .permissionDenied: return .failure(code:"PERMISSION_DENIED",message:"This account did not grant the needed read permission"); case .rateLimited: return .failure(code:"RATE_LIMITED",message:"This account is temporarily rate limited"); default: return .failure(code:"ACCOUNT_UNAVAILABLE",message:"This account could not complete the read request") }
        } catch { return .failure(code:"ACCOUNT_UNAVAILABLE",message:"This account could not complete the read request") }
    }

    private static func input(_ json: String?) -> AccountReadRequest? {
        guard let json, Data(json.utf8).count <= 16_384, let raw = try? JSONSerialization.jsonObject(with: Data(json.utf8)) as? [String: Any], Set(raw.keys).isSubset(of:["operation","query","channel","timeMin","timeMax","limit","cursor"]), let opText=raw["operation"] as? String, let op=AccountReadOperation(rawValue:opText), let limitNumber = raw["limit"] as? NSNumber, String(cString: limitNumber.objCType) != "c", limitNumber.doubleValue.rounded() == limitNumber.doubleValue else { return nil }
        let limit = limitNumber.intValue
        guard ["query", "channel", "timeMin", "timeMax", "cursor"].allSatisfy({ key in
            raw[key] == nil || raw[key] is String
        }) else { return nil }
        func string(_ key: String) -> String? { guard let v=raw[key] else{return nil}; return v as? String }
        let request=AccountReadRequest(operation:op,query:string("query"),channel:string("channel"),timeMin:string("timeMin"),timeMax:string("timeMax"),limit:limit,cursor:string("cursor"))
        return request
    }
}
