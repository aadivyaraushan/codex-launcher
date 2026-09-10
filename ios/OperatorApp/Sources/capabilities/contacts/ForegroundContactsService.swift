import Contacts
import Foundation
import OperatorCore
import OSLog
#if canImport(UIKit)
import UIKit
#endif

// Contacts is a lookup, never a listing.
//
// The whole value of this connector is turning "text Mom" into a handle
// sms.compose and whatsapp.compose can actually use. That job needs a name
// in and at most a handful of matches out. It never needs the address book,
// so there is no way to ask for it: a query is required, it must be
// non-empty, and there is no verb here that enumerates.
//
// That is a deliberate ceiling rather than an unfinished one. An agent that
// can page through every contact has copied the owner's social graph into a
// transcript, and no amount of good behaviour afterwards takes it back.

enum ContactsAccess: Equatable, Sendable {
    case notDetermined
    /// iOS 18 lets the owner grant a chosen subset. Matches inside that
    /// subset are returned normally; the ones outside it are indistinguishable
    /// from contacts that do not exist, which is the point of the setting.
    case limited
    case full
    case denied
}

struct ContactMatch: Sendable, Equatable {
    let displayName: String
    let phoneNumbers: [String]
    let emailAddresses: [String]
}

@MainActor
protocol ContactDirectory: AnyObject {
    var access: ContactsAccess { get }
    func requestAccess() async -> Bool
    func search(query: String, limit: Int) async -> [ContactMatch]
}

@MainActor
final class ForegroundContactsService: GatewayNodeCommandHandler {
    static let maximumLimit = 10
    static let defaultLimit = 5
    static let maximumQueryLength = 100
    /// Handles per contact. Someone with nine numbers has one useful number
    /// and eight the agent should not be guessing between.
    static let maximumHandlesPerContact = 5

    private struct Payload: Encodable {
        struct Match: Encodable {
            let name: String
            let phones: [String]
            let emails: [String]
        }

        let matches: [Match]
        let partialAccess: Bool
    }

    private let directory: any ContactDirectory
    private let isAppActive: @MainActor @Sendable () -> Bool
    private let logger = Logger(subsystem: "app.operator.ios", category: "foreground-contacts")

    init(
        directory: any ContactDirectory,
        isAppActive: @escaping @MainActor @Sendable () -> Bool)
    {
        self.directory = directory
        self.isAppActive = isAppActive
    }

    func handleNodeCommand(
        _ command: String,
        paramsJSON: String?,
        timeoutMilliseconds _: Int?) async -> GatewayNodeCommandResult
    {
        guard command == "contacts.resolve" else {
            return .failure(code: "UNSUPPORTED_COMMAND", message: "This iPhone node does not support \(command)")
        }
        guard let request = Self.request(from: paramsJSON) else {
            self.logger.info("[contacts] refused branch=invalid_params")
            return .failure(
                code: "INVALID_REQUEST",
                message: "contacts.resolve requires a nonempty query and an optional limit between 1 and \(Self.maximumLimit)")
        }
        guard self.isAppActive() else {
            self.logger.info("[contacts] refused branch=app_not_active")
            return .failure(code: "APP_NOT_ACTIVE", message: "Open Operator to look up a contact")
        }
        if self.directory.access == .denied {
            self.logger.info("[contacts] refused branch=permission_denied")
            return .failure(code: "PERMISSION_DENIED", message: "Contacts permission was denied")
        }
        if self.directory.access == .notDetermined {
            let granted = await self.directory.requestAccess()
            guard self.isAppActive() else {
                self.logger.info("[contacts] refused branch=app_left_during_permission")
                return .failure(code: "APP_NOT_ACTIVE", message: "Open Operator to look up a contact")
            }
            guard granted else {
                self.logger.info("[contacts] refused branch=permission_request_denied")
                return .failure(code: "PERMISSION_DENIED", message: "Contacts permission was denied")
            }
        }

        let partial = self.directory.access == .limited
        let matches = await self.directory.search(query: request.query, limit: request.limit)
            .prefix(request.limit)
            .map { match in
                Payload.Match(
                    name: match.displayName,
                    phones: Array(match.phoneNumbers.prefix(Self.maximumHandlesPerContact)),
                    emails: Array(match.emailAddresses.prefix(Self.maximumHandlesPerContact)))
            }
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        guard let data = try? encoder.encode(Payload(matches: Array(matches), partialAccess: partial)),
              let payloadJSON = String(data: data, encoding: .utf8)
        else {
            self.logger.error("[contacts] failed branch=encode count=\(matches.count)")
            return .failure(code: "INTERNAL_ERROR", message: "Operator could not look up that contact")
        }
        // The query is not logged. A name being searched for is the content
        // of the request, not its shape.
        self.logger.info("[contacts] returned count=\(matches.count) partial=\(partial)")
        return .success(payloadJSON: payloadJSON)
    }

    struct Request: Equatable, Sendable {
        let query: String
        let limit: Int
    }

    static func request(from paramsJSON: String?) -> Request? {
        guard let paramsJSON, paramsJSON.utf8.count <= 4096,
              let value = try? JSONSerialization.jsonObject(with: Data(paramsJSON.utf8)),
              let object = value as? [String: Any],
              Set(object.keys).isSubset(of: ["query", "limit"]),
              let rawQuery = object["query"] as? String
        else { return nil }

        let query = rawQuery.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !query.isEmpty, query.count <= Self.maximumQueryLength else { return nil }

        guard let rawLimit = object["limit"] else { return Request(query: query, limit: Self.defaultLimit) }
        guard let number = rawLimit as? NSNumber, !(rawLimit is Bool),
              Double(number.intValue) == number.doubleValue
        else { return nil }
        let limit = number.intValue
        guard (1 ... Self.maximumLimit).contains(limit) else { return nil }
        return Request(query: query, limit: limit)
    }
}

@MainActor
final class SystemContactDirectory: ContactDirectory {
    private let store = CNContactStore()

    var access: ContactsAccess {
        let status = CNContactStore.authorizationStatus(for: .contacts)
        if status == .authorized { return .full }
        if status == .notDetermined { return .notDetermined }
        // Partial access is an iOS-only status; the macOS SDK the fixture
        // suites build against does not define it.
        #if canImport(UIKit)
        if #available(iOS 18.0, *), status == .limited { return .limited }
        #endif
        return .denied
    }

    func requestAccess() async -> Bool {
        await withCheckedContinuation { continuation in
            self.store.requestAccess(for: .contacts) { granted, _ in
                continuation.resume(returning: granted)
            }
        }
    }

    func search(query: String, limit: Int) async -> [ContactMatch] {
        let keys: [any CNKeyDescriptor] = [
            CNContactFormatter.descriptorForRequiredKeys(for: .fullName),
            CNContactPhoneNumbersKey as any CNKeyDescriptor,
            CNContactEmailAddressesKey as any CNKeyDescriptor,
        ]
        let predicate = CNContact.predicateForContacts(matchingName: query)
        let contacts = (try? self.store.unifiedContacts(matching: predicate, keysToFetch: keys)) ?? []
        return contacts.prefix(limit).map { contact in
            ContactMatch(
                displayName: CNContactFormatter.string(from: contact, style: .fullName) ?? "",
                phoneNumbers: contact.phoneNumbers.map(\.value.stringValue),
                emailAddresses: contact.emailAddresses.map { $0.value as String })
        }
    }
}

#if canImport(UIKit)
extension ForegroundContactsService {
    convenience init() {
        self.init(
            directory: SystemContactDirectory(),
            isAppActive: { UIApplication.shared.applicationState == .active })
    }
}
#endif
