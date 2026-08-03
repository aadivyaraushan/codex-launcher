package app.codexlauncher.connection.session

import app.codexlauncher.connection.pairing.network.DevicePairingSigner
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.protocol.ProtocolSession
import app.codexlauncher.connection.protocol.Sender
import app.codexlauncher.connection.security.PinnedTlsClientFactory
import app.codexlauncher.diagnostics.AppLog
import okhttp3.Dns
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference
import java.net.ConnectException
import java.net.NoRouteToHostException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

enum class SessionFailure {
    REVOKED,
    INVALID_PROTOCOL,
    BOX_UNREACHABLE,
    CONNECTION_LOST,
}

internal fun classifySessionFailure(t: Throwable, response: Response?): SessionFailure {
    if (response?.code == 403) return SessionFailure.REVOKED
    var cause: Throwable? = t
    while (cause != null) {
        if (cause is UnknownHostException || cause is ConnectException || cause is NoRouteToHostException || cause is SocketTimeoutException) {
            return SessionFailure.BOX_UNREACHABLE
        }
        cause = cause.cause
    }
    return SessionFailure.CONNECTION_LOST
}

interface SessionObserver {
    fun onReady(connection: SessionConnection, attachmentKey: ByteArray)

    fun onMessage(message: ProtocolMessage)

    fun onFailure(reason: SessionFailure)

    fun onClosed()
}

interface SessionConnection {
    fun sendText(encoded: String): Boolean

    fun sendBinary(frame: ByteArray): Boolean = false

    suspend fun sendAction(
        encoded: String,
        beforeSocketWrite: suspend () -> Boolean,
    ): ActionSendResult

    fun close()
}

enum class ActionSendResult {
    NOT_SENT,
    SENT_UNKNOWN,
}

class CompanionSessionConnection internal constructor(
    private val client: OkHttpClient,
) : SessionConnection {
    private val socket = AtomicReference<WebSocket?>()
    private val ready = AtomicBoolean(false)
    private val stopped = AtomicBoolean(false)
    private val actionSendMutex = Mutex()

    internal fun attach(webSocket: WebSocket) {
        socket.set(webSocket)
    }

    internal fun markReady() {
        ready.set(true)
    }

    override fun sendText(encoded: String): Boolean {
        if (!ready.get() || stopped.get()) return false
        val message = validatedPhoneMessage(encoded) ?: return false
        return socket.get()?.send(encoded) == true
    }

    override fun sendBinary(frame: ByteArray): Boolean {
        if (!ready.get() || stopped.get() || frame.isEmpty() || frame.size > ProtocolCodec.MAX_ATTACHMENT_FRAME_BYTES) return false
        return socket.get()?.send(frame.toByteString()) == true
    }

    override suspend fun sendAction(
        encoded: String,
        beforeSocketWrite: suspend () -> Boolean,
    ): ActionSendResult =
        actionSendMutex.withLock {
            val message = validatedPhoneMessage(encoded)
            val initialSocket = socket.get()
            if (!ready.get() || stopped.get() || initialSocket == null || message?.type != MessageType.ACTION) {
                AppLog.info(
                    feature = "session-network",
                    message = "phone action rejected before send boundary",
                    fields = mapOf("decision" to "not_sent", "input_shape" to "validated_phone_action"),
                )
                return@withLock ActionSendResult.NOT_SENT
            }
            val boundaryStored =
                try {
                    beforeSocketWrite()
                } catch (error: Exception) {
                    AppLog.error(
                        feature = "session-network",
                        message = "phone action journal boundary failed",
                        error = error,
                        fields = mapOf("message_id" to message.messageId, "decision" to "not_sent"),
                    )
                    false
                }
            if (!boundaryStored) {
                AppLog.info(
                    feature = "session-network",
                    message = "phone action blocked at send boundary",
                    fields = mapOf("message_id" to message.messageId, "decision" to "not_sent_storage_unavailable"),
                )
                return@withLock ActionSendResult.NOT_SENT
            }
            val activeSocket = socket.get()
            val accepted =
                ready.get() && !stopped.get() && activeSocket === initialSocket &&
                    activeSocket.send(encoded)
            AppLog.info(
                feature = "session-network",
                message = "phone action crossed durable send boundary",
                fields = mapOf(
                    "message_id" to message.messageId,
                    "socket_accepted" to accepted,
                    "output_shape" to "sent_unknown_until_companion_result",
                ),
            )
            ActionSendResult.SENT_UNKNOWN
        }

    private fun validatedPhoneMessage(encoded: String): ProtocolMessage? {
        if (encoded.encodeToByteArray().size > ProtocolCodec.MAX_JSON_FRAME_BYTES) return null
        val message = runCatching { ProtocolCodec.decodeText(encoded) }.getOrNull() ?: return null
        return message.takeIf { it.sender == Sender.PHONE }
    }

    override fun close() {
        if (stopped.compareAndSet(false, true)) {
            ready.set(false)
            socket.getAndSet(null)?.close(1000, "client closed")
            client.dispatcher.executorService.shutdown()
            client.connectionPool.evictAll()
        }
    }

    internal fun stopAfterSocketEnd() {
        if (stopped.compareAndSet(false, true)) {
            ready.set(false)
            socket.set(null)
            client.dispatcher.executorService.shutdown()
            client.connectionPool.evictAll()
        }
    }
}

class CompanionSessionClient private constructor(
    private val signer: DevicePairingSigner,
    private val endpoint: (PairedComputer, String) -> String,
    private val tlsClients: PinnedTlsClientFactory,
    private val loadResumeCursor: (String) -> Long?,
) {
    constructor(
        signer: DevicePairingSigner,
        loadResumeCursor: (String) -> Long? = { null },
    ) : this(signer, ::productionEndpoint, PinnedTlsClientFactory(), loadResumeCursor)

    // Test-only: targets a local MockWebServer URL, not an untrusted paired-computer
    // host, so it uses the system resolver instead of the public-address DNS filter.
    internal constructor(
        signer: DevicePairingSigner,
        testEndpoint: String,
        loadResumeCursor: (String) -> Long? = { null },
    ) : this(signer, { _, _ -> testEndpoint }, PinnedTlsClientFactory(dns = Dns.SYSTEM), loadResumeCursor)

    fun connect(
        paired: PairedComputer,
        sessionId: String,
        observer: SessionObserver,
    ): SessionConnection {
        val resumeThroughSequence = loadResumeCursor(paired.pairingGeneration)
        val handshake = SessionHandshake(paired, sessionId, signer, resumeThroughSequence = resumeThroughSequence)
        val client =
            tlsClients.builder(paired.tlsIdentityPin())
                .connectTimeout(10, TimeUnit.SECONDS)
                .readTimeout(0, TimeUnit.SECONDS)
                .writeTimeout(10, TimeUnit.SECONDS)
                .pingInterval(20, TimeUnit.SECONDS)
                .build()
        val connection = CompanionSessionConnection(client)
        val failureReported = AtomicBoolean(false)
        var protocolSession: ProtocolSession? = null
        var pendingAttachmentKey: ByteArray? = null
        var welcomeReceived = false
        val listener =
            object : WebSocketListener() {
                override fun onOpen(webSocket: WebSocket, response: Response) {
                    AppLog.info(
                        feature = "session-network",
                        message = "pinned WebSocket opened",
                        fields = mapOf("device_id" to paired.deviceId, "session_id" to sessionId, "input_shape" to "tls13_websocket"),
                    )
                }

                override fun onMessage(webSocket: WebSocket, text: String) {
                    try {
                        val activeProtocol = protocolSession
                        if (activeProtocol == null) {
                            when (val output = handshake.receive(text)) {
                                is SessionHandshake.Output.Proof -> {
                                    if (!webSocket.send(output.json)) throw SessionHandshakeException("Session proof could not be sent")
                                }
                                is SessionHandshake.Output.Ready -> {
                                    val nextProtocol =
                                        ProtocolSession(
                                            attachmentKey = output.attachmentKey,
                                            expectedSessionId = sessionId,
                                            deviceId = paired.deviceId,
                                            initialSequence = resumeThroughSequence,
                                        )
                                    if (!webSocket.send(output.helloJson)) throw SessionHandshakeException("Session hello could not be sent")
                                    protocolSession = nextProtocol
                                    pendingAttachmentKey = output.attachmentKey.copyOf()
                                }
                            }
                            return
                        }
                        val message = activeProtocol.acceptText(text)
                        if (message.sender != Sender.COMPANION || (!welcomeReceived && message.type != MessageType.WELCOME) || (welcomeReceived && message.type == MessageType.WELCOME)) {
                            throw SessionHandshakeException("Companion message order is invalid")
                        }
                        if (message.type == MessageType.WELCOME) {
                            welcomeReceived = true
                            connection.markReady()
                            observer.onReady(connection, requireNotNull(pendingAttachmentKey).copyOf())
                            pendingAttachmentKey = null
                        }
                        observer.onMessage(message)
                    } catch (error: Exception) {
                        AppLog.error(
                            feature = "session-network",
                            message = "WebSocket frame rejected",
                            error = error,
                            fields = mapOf("device_id" to paired.deviceId, "session_id" to sessionId, "decision" to "close_policy_violation"),
                        )
                        reportFailure(failureReported, observer, SessionFailure.INVALID_PROTOCOL)
                        webSocket.close(1008, "protocol error")
                    }
                }

                override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                    val activeProtocol = protocolSession
                    if (activeProtocol == null || !welcomeReceived) {
                        reportFailure(failureReported, observer, SessionFailure.INVALID_PROTOCOL)
                        webSocket.close(1008, "protocol error")
                        return
                    }
                    try {
                        activeProtocol.acceptAttachmentFrame(bytes.toByteArray())
                    } catch (error: Exception) {
                        AppLog.error(
                            feature = "session-network",
                            message = "WebSocket attachment rejected",
                            error = error,
                            fields = mapOf("device_id" to paired.deviceId, "session_id" to sessionId, "decision" to "close_policy_violation"),
                        )
                        reportFailure(failureReported, observer, SessionFailure.INVALID_PROTOCOL)
                        webSocket.close(1008, "protocol error")
                    }
                }

                override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                    connection.stopAfterSocketEnd()
                    if (!failureReported.get()) observer.onClosed()
                }

                override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                    AppLog.error(
                        feature = "session-network",
                        message = "pinned WebSocket failed",
                        error = t,
                        fields = mapOf("device_id" to paired.deviceId, "session_id" to sessionId, "http_status" to (response?.code ?: 0)),
                    )
                    val reason = classifySessionFailure(t, response)
                    reportFailure(failureReported, observer, reason)
                    connection.stopAfterSocketEnd()
                }
            }
        AppLog.info(
            feature = "session-network",
            message = "opening pinned WebSocket",
            fields = mapOf(
                "device_id" to paired.deviceId,
                "session_id" to sessionId,
                "resume_mode" to if (resumeThroughSequence != null) "warm" else "no_local_state",
                "input_shape" to "paired_computer,resume_cursor",
            ),
        )
        val webSocket = client.newWebSocket(Request.Builder().url(endpoint(paired, sessionId)).build(), listener)
        connection.attach(webSocket)
        return connection
    }

    private fun PairedComputer.tlsIdentityPin() =
        app.codexlauncher.connection.security.TlsIdentityPin.parse(tlsIdentity)

    private companion object {
        fun reportFailure(
            alreadyReported: AtomicBoolean,
            observer: SessionObserver,
            reason: SessionFailure,
        ) {
            if (alreadyReported.compareAndSet(false, true)) observer.onFailure(reason)
        }

        fun productionEndpoint(paired: PairedComputer, sessionId: String): String {
            val host = if (paired.host.contains(':')) "[${paired.host}]" else paired.host
            return "wss://$host:${paired.port}/v1/session?deviceId=${paired.deviceId}&sessionId=$sessionId"
        }
    }
}
