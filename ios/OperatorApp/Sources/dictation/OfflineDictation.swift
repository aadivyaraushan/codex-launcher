import AVFAudio
import Combine
import OSLog
import Speech

enum OfflineDictationEvent: Equatable {
    case transcript(String)
    case finished
    case unavailable(String)
}

enum OfflineDictationState: Equatable {
    case idle
    case requestingPermission
    case recording
    case unavailable(String)
}

@MainActor
protocol OfflineDictationService: AnyObject {
    func start(eventHandler: @escaping @MainActor (OfflineDictationEvent) -> Void) async throws
    func stop()
}

@MainActor
final class OfflineDictationModel: ObservableObject {
    @Published private(set) var state: OfflineDictationState = .idle

    private let service: any OfflineDictationService
    private var draftBeforeTranscript = ""
    private var lastRenderedDraft = ""

    init(service: any OfflineDictationService) {
        self.service = service
    }

    func start(draft: String, updateDraft: @escaping @MainActor (String) -> Void) {
        guard self.state != .recording, self.state != .requestingPermission else { return }

        self.draftBeforeTranscript = draft
        self.lastRenderedDraft = draft
        self.state = .requestingPermission
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                try await self.service.start { [weak self] event in
                    self?.handle(event, updateDraft: updateDraft)
                }
                if self.state == .requestingPermission {
                    self.state = .recording
                }
            } catch {
                self.state = .unavailable(Self.message(for: error))
            }
        }
    }

    func noteDraftChanged(_ draft: String) {
        guard self.state == .recording, draft != self.lastRenderedDraft else { return }
        self.draftBeforeTranscript = draft
    }

    func stop() {
        self.service.stop()
        self.state = .idle
    }

    private func handle(
        _ event: OfflineDictationEvent,
        updateDraft: @escaping @MainActor (String) -> Void
    ) {
        switch event {
        case let .transcript(text):
            guard !text.isEmpty else { return }
            let separator = self.draftBeforeTranscript.isEmpty ? "" : " "
            let updatedDraft = self.draftBeforeTranscript + separator + text
            self.lastRenderedDraft = updatedDraft
            updateDraft(updatedDraft)
        case .finished:
            self.state = .idle
        case let .unavailable(message):
            self.state = .unavailable(message)
        }
    }

    private static func message(for error: Error) -> String {
        if let error = error as? OfflineDictationError {
            return error.message
        }
        return "On-device dictation could not start. Try again."
    }
}

enum OfflineDictationError: Error {
    case speechPermissionDenied
    case microphonePermissionDenied
    case onDeviceRecognitionUnavailable
    case microphoneUnavailable

    var message: String {
        switch self {
        case .speechPermissionDenied:
            "Allow Speech Recognition to use on-device dictation."
        case .microphonePermissionDenied:
            "Allow Microphone access to use on-device dictation."
        case .onDeviceRecognitionUnavailable:
            "On-device speech recognition is unavailable for this language."
        case .microphoneUnavailable:
            "The microphone could not start. Try again."
        }
    }
}

@MainActor
final class AppleOnDeviceDictationService: NSObject, OfflineDictationService {
    private let audioEngine = AVAudioEngine()
    private let audioSession = AVAudioSession.sharedInstance()
    private let recognizer = SFSpeechRecognizer()
    private let logger = Logger(subsystem: "app.operator.ios", category: "dictation")
    private var request: SFSpeechAudioBufferRecognitionRequest?
    private var task: SFSpeechRecognitionTask?
    private var eventHandler: (@MainActor (OfflineDictationEvent) -> Void)?

    func start(eventHandler: @escaping @MainActor (OfflineDictationEvent) -> Void) async throws {
        guard self.task == nil else { return }
        guard await self.speechPermissionGranted() else {
            throw OfflineDictationError.speechPermissionDenied
        }
        guard await self.microphonePermissionGranted() else {
            throw OfflineDictationError.microphonePermissionDenied
        }
        guard let recognizer = self.recognizer,
              recognizer.isAvailable,
              recognizer.supportsOnDeviceRecognition
        else {
            throw OfflineDictationError.onDeviceRecognitionUnavailable
        }

        self.eventHandler = eventHandler
        let request = SFSpeechAudioBufferRecognitionRequest()
        request.requiresOnDeviceRecognition = true
        request.shouldReportPartialResults = true
        self.request = request

        do {
            try self.audioSession.setCategory(.record, mode: .measurement, options: .duckOthers)
            try self.audioSession.setActive(true, options: .notifyOthersOnDeactivation)
            let input = self.audioEngine.inputNode
            let format = input.outputFormat(forBus: 0)
            input.installTap(onBus: 0, bufferSize: 1_024, format: format) { [weak request] buffer, _ in
                request?.append(buffer)
            }
            self.audioEngine.prepare()
            try self.audioEngine.start()
            self.task = recognizer.recognitionTask(with: request) { [weak self] result, error in
                Task { @MainActor [weak self] in
                    self?.handle(result: result, error: error)
                }
            }
            self.logger.info("[dictation] on-device recording started")
        } catch {
            self.stop()
            throw OfflineDictationError.microphoneUnavailable
        }
    }

    func stop() {
        self.request?.endAudio()
        self.task?.cancel()
        self.tearDownAudio()
        self.eventHandler = nil
        self.logger.info("[dictation] recording stopped")
    }

    private func handle(result: SFSpeechRecognitionResult?, error: Error?) {
        if let transcript = result?.bestTranscription.formattedString, !transcript.isEmpty {
            self.eventHandler?(.transcript(transcript))
        }
        if error != nil {
            let eventHandler = self.eventHandler
            self.tearDownAudio()
            self.eventHandler = nil
            eventHandler?(.unavailable("On-device dictation stopped unexpectedly."))
            self.logger.error("[dictation] recognition stopped with an error")
        } else if result?.isFinal == true {
            let eventHandler = self.eventHandler
            self.tearDownAudio()
            self.eventHandler = nil
            eventHandler?(.finished)
            self.logger.info("[dictation] recognition finished")
        }
    }

    private func tearDownAudio() {
        self.audioEngine.stop()
        self.audioEngine.inputNode.removeTap(onBus: 0)
        self.request = nil
        self.task = nil
        try? self.audioSession.setActive(false, options: .notifyOthersOnDeactivation)
    }

    private func speechPermissionGranted() async -> Bool {
        let status = SFSpeechRecognizer.authorizationStatus()
        let resolvedStatus: SFSpeechRecognizerAuthorizationStatus
        if status == .notDetermined {
            resolvedStatus = await withCheckedContinuation { continuation in
                SFSpeechRecognizer.requestAuthorization { continuation.resume(returning: $0) }
            }
        } else {
            resolvedStatus = status
        }
        return resolvedStatus == .authorized
    }

    private func microphonePermissionGranted() async -> Bool {
        switch AVAudioApplication.shared.recordPermission {
        case .undetermined:
            return await withCheckedContinuation { continuation in
                AVAudioApplication.requestRecordPermission { granted in
                    continuation.resume(returning: granted)
                }
            }
        case .granted:
            return true
        default:
            return false
        }
    }
}
