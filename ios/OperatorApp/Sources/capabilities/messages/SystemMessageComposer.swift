import MessageUI
import OSLog
import UIKit

@MainActor
final class SystemMessageComposer: NSObject, MessageComposePresenter, @preconcurrency MFMessageComposeViewControllerDelegate {
    private let application: UIApplication
    private let logger = Logger(subsystem: "app.operator.ios", category: "message-compose")
    private var composer: MFMessageComposeViewController?

    init(application: UIApplication = .shared) {
        self.application = application
        super.init()
    }

    var isAvailable: Bool {
        MFMessageComposeViewController.canSendText()
    }

    func present(recipients: [String], body: String) -> Bool {
        guard self.application.applicationState == .active,
              self.composer == nil,
              let root = self.activeRootViewController(),
              root.presentedViewController == nil
        else {
            self.logger.info("[message-compose] system composer presentation blocked active=\(self.application.applicationState == .active) busy=\(self.composer != nil)")
            return false
        }

        let composer = MFMessageComposeViewController()
        composer.messageComposeDelegate = self
        composer.recipients = recipients
        composer.body = body
        self.composer = composer
        root.present(composer, animated: true)
        self.logger.info("[message-compose] system composer presentation requested recipients=\(recipients.count)")
        return true
    }

    func messageComposeViewController(
        _ controller: MFMessageComposeViewController,
        didFinishWith result: MessageComposeResult)
    {
        let outcome: String
        switch result {
        case .sent: outcome = "sent"
        case .cancelled: outcome = "cancelled"
        case .failed: outcome = "failed"
        @unknown default: outcome = "unknown"
        }
        self.logger.info("[message-compose] system composer finished outcome=\(outcome, privacy: .public)")
        controller.dismiss(animated: true) { [weak self, weak controller] in
            guard let self, self.composer === controller else { return }
            self.composer = nil
        }
    }

    private func activeRootViewController() -> UIViewController? {
        self.application.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .filter { $0.activationState == .foregroundActive }
            .flatMap(\.windows)
            .first(where: \.isKeyWindow)?
            .rootViewController
    }
}
