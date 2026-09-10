import Foundation

public struct GatewayRequest<Params: Encodable & Sendable>: Encodable, Sendable {
    public let type = "req"
    public let id: String
    public let method: String
    public let params: Params
}

public struct GatewayEventFrame<Payload: Decodable & Sendable>: Decodable, Sendable {
    public let type: String
    public let event: String
    public let payload: Payload
    public let sequence: Int?

    private enum CodingKeys: String, CodingKey {
        case type
        case event
        case payload
        case sequence = "seq"
    }
}

public struct GatewayResponseFrame: Decodable, Sendable {
    public struct Payload: Decodable, Sendable {
        public let runID: String?
        public let status: String?

        private enum CodingKeys: String, CodingKey {
            case runID = "runId"
            case status
        }
    }

    public struct Failure: Decodable, Equatable, Sendable {
        public let code: String
        public let message: String
        public let retryable: Bool?
        public let retryAfterMilliseconds: Int?

        private enum CodingKeys: String, CodingKey {
            case code
            case message
            case retryable
            case retryAfterMilliseconds = "retryAfterMs"
        }
    }

    public let type: String
    public let id: String
    public let ok: Bool
    public let payload: Payload?
    public let error: Failure?
}

public struct GatewayRPCResponseFrame<Payload: Decodable & Sendable>: Decodable, Sendable {
    public let type: String
    public let id: String
    public let ok: Bool
    public let payload: Payload?
    public let error: GatewayResponseFrame.Failure?
}

public struct GatewayConnectParams: Codable, Sendable {
    public struct Client: Codable, Sendable {
        public let id: String
        public let displayName: String
        public let version: String
        public let platform: String
        public let deviceFamily: String
        public let mode: String
        public let instanceId: String
    }

    public struct Auth: Codable, Sendable {
        public let token: String
    }

    public let minProtocol: Int
    public let maxProtocol: Int
    public let client: Client
    public let role: String
    public let scopes: [String]
    public let caps: [String]
    public let auth: Auth
    public let device: GatewaySignedDevice
}

public struct GatewayChatSendParams: Codable, Sendable {
    public let sessionKey: String
    public let message: String
    public let fastMode: String?
    public let idempotencyKey: String
}

public struct GatewayChatAbortParams: Codable, Sendable {
    public let sessionKey: String
    public let runId: String?
}

public enum GatewayRequestFactory {
    public static func connect(
        requestID: String,
        token: String,
        identity: GatewayDeviceIdentity,
        challenge: GatewayConnectChallenge,
        appVersion: String,
        platform: String,
        instanceID: String) throws -> GatewayRequest<GatewayConnectParams>
    {
        let scopes = ["operator.admin", "operator.read", "operator.write"]
        let proof = try GatewayDeviceProof.make(
            identity: identity,
            challenge: challenge,
            token: token,
            clientID: "openclaw-ios",
            clientMode: "ui",
            role: "operator",
            scopes: scopes)
        return GatewayRequest(
            id: requestID,
            method: "connect",
            params: GatewayConnectParams(
                minProtocol: 4,
                maxProtocol: 4,
                client: .init(
                    id: "openclaw-ios",
                    displayName: "Operator",
                    version: appVersion,
                    platform: platform,
                    deviceFamily: "iPhone",
                    mode: "ui",
                    instanceId: instanceID),
                role: "operator",
                scopes: scopes,
                caps: [],
                auth: .init(token: token),
                device: proof.device))
    }

    public static func chatSend(
        requestID: String,
        sessionKey: String,
        message: String,
        idempotencyKey: String,
        fastMode: String? = "auto") -> GatewayRequest<GatewayChatSendParams>
    {
        GatewayRequest(
            id: requestID,
            method: "chat.send",
            params: GatewayChatSendParams(
                sessionKey: sessionKey,
                message: message,
                fastMode: fastMode,
                idempotencyKey: idempotencyKey))
    }

    public static func chatAbort(
        requestID: String,
        sessionKey: String,
        runID: String?) -> GatewayRequest<GatewayChatAbortParams>
    {
        GatewayRequest(
            id: requestID,
            method: "chat.abort",
            params: GatewayChatAbortParams(sessionKey: sessionKey, runId: runID))
    }
}
