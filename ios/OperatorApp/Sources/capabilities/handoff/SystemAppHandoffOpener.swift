import Foundation
import OSLog
import SafariServices
import UIKit

@MainActor
final class SystemAppHandoffOpener: AppHandoffOpener {
    func open(_ url: URL) async -> Bool {
        guard AppHandoffCatalog.isSafeDestination(url),
              UIApplication.shared.applicationState == .active,
              let root = UIApplication.shared.connectedScenes
                .compactMap({ $0 as? UIWindowScene })
                .filter({ $0.activationState == .foregroundActive })
                .flatMap(\.windows).first(where: \.isKeyWindow)?.rootViewController,
              root.presentedViewController == nil
        else { return false }
        // Keep the node connection alive while reporting the opened page.
        // Leaving Operator for the Safari app disconnects it before its result.
        let browser = SFSafariViewController(url: url)
        return await withCheckedContinuation { continuation in
            root.present(browser, animated: true) {
                continuation.resume(returning: root.presentedViewController === browser)
            }
        }
    }
}

extension ForegroundAppHandoffService {
    convenience init(bundle: Bundle = .main) {
        let logger = Logger(subsystem: "app.operator.ios", category: "app-handoff")
        let destinations: [String: URL]
        do {
            guard let file = bundle.url(forResource: "android-handoff-catalog", withExtension: "json") else {
                throw AppHandoffCatalog.InvalidCatalog.invalidDestination
            }
            destinations = try AppHandoffCatalog.decode(Data(contentsOf: file))
            logger.info("[app-handoff] loaded destination count=\(destinations.count)")
        } catch {
            // Missing or malformed packaging must not open arbitrary URLs.
            destinations = [:]
            logger.error("[app-handoff] catalog unavailable; app opening disabled errorType=\(String(reflecting: type(of: error)), privacy: .public)")
        }
        self.init(destinations: destinations, opener: SystemAppHandoffOpener(), isAppActive: {
            UIApplication.shared.applicationState == .active
        })
    }
}
