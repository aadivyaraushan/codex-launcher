import Foundation
import OSLog

enum AccountReadOperation: String, Sendable {
    case googleCalendarEvents, googleDriveFiles, outlookInbox, outlookCalendarEvents, slackChannels, slackHistory, spotifySearch, spotifyPlayback
    var provider: OAuthProvider {
        switch self {
        case .googleCalendarEvents, .googleDriveFiles: .google
        case .outlookInbox, .outlookCalendarEvents: .microsoftOutlook
        case .slackChannels, .slackHistory: .slack
        case .spotifySearch, .spotifyPlayback: .spotify
        }
    }
}

struct AccountReadRequest: Sendable {
    let operation: AccountReadOperation; let query: String?; let channel: String?; let timeMin: String?; let timeMax: String?; let limit: Int; let cursor: String?
}

struct AccountReadPage: Sendable {
    let payloadJSON: String; let count: Int; let nextCursor: String?
}

enum AccountReadError: Error, Equatable, Sendable { case invalidRequest, notConnected, permissionDenied, rateLimited(retryAfterSeconds: Int), unavailable, invalidResponse }

actor DirectAccountReader {
    private let transport: any PhoneHTTPTransport
    private let bearer: @Sendable (OAuthProvider) async throws -> String
    private let logger = Logger(subsystem: "app.operator.ios", category: "account-read")

    init(transport: any PhoneHTTPTransport = URLSessionPhoneHTTPTransport(), bearer: @escaping @Sendable (OAuthProvider) async throws -> String) {
        self.transport = transport; self.bearer = bearer
    }

    func read(_ input: AccountReadRequest) async throws -> AccountReadPage {
        guard self.valid(input) else { throw AccountReadError.invalidRequest }
        let url = try self.url(for: input)
        let token: String
        do { token = try await self.bearer(input.operation.provider) } catch { throw AccountReadError.notConnected }
        var request = URLRequest(url: url); request.httpMethod = "GET"; request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        self.logger.info("[account-read] request provider=\(input.operation.provider.rawValue, privacy: .public) operation=\(input.operation.rawValue, privacy: .public) limit=\(input.limit)")
        let data: Data; let response: URLResponse
        do { (data, response) = try await self.transport.data(for: request) } catch { throw AccountReadError.unavailable }
        guard let http = response as? HTTPURLResponse else { throw AccountReadError.unavailable }
        guard data.count <= 512_000 else { throw AccountReadError.invalidResponse }
        if input.operation == .spotifyPlayback, http.statusCode == 204 { return .init(payloadJSON: "[]", count: 0, nextCursor: nil) }
        switch http.statusCode { case 200: break; case 401: throw AccountReadError.notConnected; case 403: throw AccountReadError.permissionDenied; case 429: throw AccountReadError.rateLimited(retryAfterSeconds: max(1, Int(http.value(forHTTPHeaderField: "Retry-After") ?? "") ?? 1)); case 500...599: throw AccountReadError.unavailable; default: throw AccountReadError.unavailable }
        return try self.page(data, input: input)
    }

    private func valid(_ r: AccountReadRequest) -> Bool {
        guard (1...20).contains(r.limit), (r.query?.count ?? 0) <= 200, (r.channel?.count ?? 0) <= 100, (r.cursor?.count ?? 0) <= 500 else { return false }
        if (r.operation == .spotifySearch || r.operation == .spotifyPlayback) && r.limit > 10 { return false }
        if let cursor = r.cursor, cursor.contains("://") { return false }
        switch r.operation {
        case .googleCalendarEvents, .outlookCalendarEvents:
            guard let minText = r.timeMin, let maxText = r.timeMax, r.channel == nil,
                  let min = Self.rfc3339(minText), let max = Self.rfc3339(maxText) else { return false }
            return min < max
        case .googleDriveFiles: return !(r.query?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true) && r.channel == nil
        case .spotifySearch:
            guard !(r.query?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true), r.channel == nil else { return false }
            guard let cursor = r.cursor else { return true }
            guard let value = Int(cursor) else { return false }
            return value >= 0
        case .slackHistory: return !(r.channel?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true)
        case .slackChannels, .outlookInbox: return r.channel == nil
        case .spotifyPlayback:
            guard r.channel == nil else { return false }
            guard let cursor = r.cursor else { return true }
            guard let value = Int(cursor) else { return false }
            return value >= 0
        }
    }

    private func url(for r: AccountReadRequest) throws -> URL {
        let base: String; var path: String; var items: [URLQueryItem] = []
        switch r.operation {
        case .googleCalendarEvents:
            base = "https://www.googleapis.com"; path = "/calendar/v3/calendars/primary/events"; items = [.init(name:"singleEvents",value:"true"),.init(name:"orderBy",value:"startTime"),.init(name:"timeMin",value:r.timeMin),.init(name:"timeMax",value:r.timeMax),.init(name:"maxResults",value:String(r.limit))]; if let q=r.query { items.append(.init(name:"q",value:q)) }; if let c=r.cursor { items.append(.init(name:"pageToken",value:c)) }
        case .googleDriveFiles:
            base = "https://www.googleapis.com"; path = "/drive/v3/files"; let safe = r.query!.replacingOccurrences(of:"\\",with:"\\\\").replacingOccurrences(of:"'",with:"\\'"); items=[.init(name:"q",value:"name contains '\(safe)' and trashed = false"),.init(name:"spaces",value:"drive"),.init(name:"pageSize",value:String(r.limit)),.init(name:"fields",value:"nextPageToken,files(id,name,mimeType)")]; if let c=r.cursor { items.append(.init(name:"pageToken",value:c)) }
        case .outlookInbox:
            base = "https://graph.microsoft.com"; path = "/v1.0/me/mailFolders/inbox/messages"; items=[.init(name:"$top",value:String(r.limit)),.init(name:"$select",value:"id,subject,from,receivedDateTime,bodyPreview")]; if let c=r.cursor { items.append(.init(name:"$skip",value:c)) }
        case .outlookCalendarEvents:
            base = "https://graph.microsoft.com"; path = "/v1.0/me/calendarView"; items=[.init(name:"startDateTime",value:r.timeMin),.init(name:"endDateTime",value:r.timeMax),.init(name:"$top",value:String(r.limit)),.init(name:"$orderby",value:"start/dateTime"),.init(name:"$select",value:"id,subject,start,end,location,isAllDay,webLink,organizer")]; if let c=r.cursor { items.append(.init(name:"$skip",value:c)) }
        case .slackChannels:
            base="https://slack.com"; path="/api/conversations.list"; items=[.init(name:"exclude_archived",value:"true"),.init(name:"types",value:"public_channel,private_channel"),.init(name:"limit",value:String(r.limit))]; if let c=r.cursor { items.append(.init(name:"cursor",value:c)) }
        case .slackHistory:
            base="https://slack.com"; path="/api/conversations.history"; items=[.init(name:"channel",value:r.channel),.init(name:"limit",value:String(r.limit))]; if let c=r.cursor { items.append(.init(name:"cursor",value:c)) }
        case .spotifySearch:
            base="https://api.spotify.com"; path="/v1/search"; items=[.init(name:"q",value:r.query),.init(name:"type",value:"track"),.init(name:"limit",value:String(r.limit)),.init(name:"offset",value:r.cursor ?? "0")]
        case .spotifyPlayback:
            base="https://api.spotify.com"; path="/v1/me/player"; items=[]
        }
        var c=URLComponents(string:base)!; c.path=path; c.queryItems=items; guard let url=c.url else { throw AccountReadError.invalidRequest }; return url
    }

    private func page(_ data: Data, input: AccountReadRequest) throws -> AccountReadPage {
        guard let object = try? JSONSerialization.jsonObject(with:data) as? [String:Any] else { throw AccountReadError.invalidResponse }
        if input.operation == .slackChannels || input.operation == .slackHistory { guard object["ok"] as? Bool == true else { throw AccountReadError.unavailable } }
        let array: [Any]?; let next: String?
        switch input.operation {
        case .googleCalendarEvents: array=object["items"] as? [Any]; next=object["nextPageToken"] as? String
        case .googleDriveFiles: array=object["files"] as? [Any]; next=object["nextPageToken"] as? String
        case .outlookInbox:
            array=object["value"] as? [Any]; next=try self.microsoftCursor(object["@odata.nextLink"] as? String, path:"/v1.0/me/mailFolders/inbox/messages")
        case .outlookCalendarEvents:
            array=object["value"] as? [Any]; next=try self.microsoftCursor(object["@odata.nextLink"] as? String, path:"/v1.0/me/calendarView")
        case .slackChannels: array=object["channels"] as? [Any]; next=((object["response_metadata"] as? [String:Any])?["next_cursor"] as? String)
        case .slackHistory: array=object["messages"] as? [Any]; next=((object["response_metadata"] as? [String:Any])?["next_cursor"] as? String)
        case .spotifySearch: let tracks=object["tracks"] as? [String:Any]; array=tracks?["items"] as? [Any]; let offset=tracks?["offset"] as? Int ?? 0; next=(tracks?["next"] as? String) == nil ? nil : String(offset + input.limit)
        case .spotifyPlayback: array = object.isEmpty ? nil : [object]; next=nil
        }
        guard let array, array.count <= input.limit else { throw AccountReadError.invalidResponse }
        let safe = array.compactMap { self.sanitize($0, operation: input.operation) }
        guard safe.count == array.count, let encoded = try? JSONSerialization.data(withJSONObject: safe, options: [.sortedKeys]), encoded.count <= 256_000 else { throw AccountReadError.invalidResponse }
        return .init(payloadJSON:String(decoding: encoded, as: UTF8.self),count:safe.count,nextCursor:next?.isEmpty == true ? nil : next)
    }

    private func microsoftCursor(_ link: String?, path: String) throws -> String? {
        guard let link else { return nil }; guard let url=URL(string:link), url.scheme=="https", url.host=="graph.microsoft.com", url.path==path, let skip=URLComponents(url:url,resolvingAgainstBaseURL:false)?.queryItems?.first(where:{$0.name=="$skip"})?.value, let value = Int(skip), value >= 0 else { throw AccountReadError.invalidResponse }; return String(value)
    }

    private static func rfc3339(_ value: String) -> Date? {
        let formatter = ISO8601DateFormatter(); formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.date(from: value) ?? { formatter.formatOptions = [.withInternetDateTime]; return formatter.date(from: value) }()
    }

    private func sanitize(_ value: Any, operation: AccountReadOperation) -> [String: Any]? {
        guard let object = value as? [String: Any] else { return nil }
        let keys: Set<String>
        switch operation {
        case .googleCalendarEvents: keys = ["id", "summary", "description", "start", "end", "htmlLink"]
        case .googleDriveFiles: keys = ["id", "name", "mimeType"]
        case .outlookInbox: keys = ["id", "subject", "from", "receivedDateTime", "bodyPreview"]
        case .outlookCalendarEvents: keys = ["id", "subject", "start", "end", "location", "isAllDay", "webLink", "organizer"]
        case .slackChannels: keys = ["id", "name", "is_private", "is_archived", "topic", "purpose"]
        case .slackHistory: keys = ["ts", "user", "text", "thread_ts"]
        case .spotifySearch: keys = ["id", "name", "artists", "album", "duration_ms", "external_urls"]
        case .spotifyPlayback: keys = ["device", "item", "is_playing", "progress_ms", "timestamp"]
        }
        var result: [String: Any] = [:]
        for key in keys { if let item = object[key], Self.safeJSON(item) { result[key] = item } }
        return result
    }

    private static func safeJSON(_ value: Any) -> Bool {
        if let string = value as? String { return string.utf8.count <= 8_192 }
        if value is NSNull || value is NSNumber { return true }
        if let array = value as? [Any] { return array.count <= 100 && array.allSatisfy(safeJSON) }
        if let object = value as? [String: Any] { return object.count <= 50 && object.allSatisfy { $0.key.utf8.count <= 128 && safeJSON($0.value) } }
        return false
    }
}
