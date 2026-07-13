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
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference

enum class SessionFailure {
    REVOKED,
    INVALID_PROTOCOL,
    CONNECTION_LOST,
}

interface SessionObserver {
    fun onReady(connection: SessionConnection, attachmentKey: ByteArray)

    fun onMessage(message: ProtocolMessage)

    fun onFailure(reason: SessionFailure)

    fun onClosed()
}

interface SessionConnection {
    fun sendText(encoded: String): Boolean

    fun close()
}

class CompanionSessionConnection internal constructor(
    private val client: OkHttpClient,
) : SessionConnection {
    private val socket = AtomicReference<WebSocket?>()
    private val ready = AtomicBoolean(false)
    private val stopped = AtomicBoolean(false)

    internal fun attach(webSocket: WebSocket) {
        socket.set(webSocket)
    }

    internal fun markReady() {
        ready.set(true)
    }

    override fun sendText(encoded: String): Boolean {
        if (!ready.get() || stopped.get() || encoded.encodeToByteArray().size > ProtocolCodec.MAX_JSON_FRAME_BYTES) return false
        val message = runCatching { ProtocolCodec.decodeText(encoded) }.getOrNull() ?: return false
        if (message.sender != Sender.PHONE) return false
        return socket.get()?.send(encoded) == true
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
) {
    constructor(signer: DevicePairingSigner) : this(signer, ::productionEndpoint, PinnedTlsClientFactory())

    internal constructor(signer: DevicePairingSigner, testEndpoint: String) :
        this(signer, { _, _ -> testEndpoint }, PinnedTlsClientFactory())

    fun connect(
        paired: PairedComputer,
        sessionId: String,
        observer: SessionObserver,
    ): SessionConnection {
        val handshake = SessionHandshake(paired, sessionId, signer)
        val client =
            tlsClients.builder(paired.hostIdentityPin())
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
                    val reason = if (response?.code == 403) SessionFailure.REVOKED else SessionFailure.CONNECTION_LOST
                    reportFailure(failureReported, observer, reason)
                    connection.stopAfterSocketEnd()
                }
            }
        AppLog.info(
            feature = "session-network",
            message = "opening pinned WebSocket",
            fields = mapOf("device_id" to paired.deviceId, "session_id" to sessionId, "input_shape" to "paired_computer"),
        )
        val webSocket = client.newWebSocket(Request.Builder().url(endpoint(paired, sessionId)).build(), listener)
        connection.attach(webSocket)
        return connection
    }

    private fun PairedComputer.hostIdentityPin() =
        app.codexlauncher.connection.security.HostIdentityPin.parse(hostIdentity)

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
