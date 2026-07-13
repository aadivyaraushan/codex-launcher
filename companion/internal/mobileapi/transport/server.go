package transport

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
)

const (
	maxPairRequestBytes = 16 * 1024
	maxConnections      = 8
	maxPreauthRequests  = 8
	handshakeTimeout    = 10 * time.Second
	pairReadTimeout     = 10 * time.Second
)

var ErrMissingDependency = errors.New("mobile transport dependency is missing")

type MessageHandler func(context.Context, string, contract.Message) error

type Server struct {
	pairing         *pairing.Service
	handle          MessageHandler
	logger          *slog.Logger
	now             func() time.Time
	quota           *contract.AttachmentQuota
	pairReadTimeout time.Duration
	preauthSlots    chan struct{}
}

func NewServer(pairingService *pairing.Service, handler MessageHandler, logger *slog.Logger) (*Server, error) {
	if pairingService == nil || handler == nil {
		return nil, ErrMissingDependency
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		pairing:         pairingService,
		handle:          handler,
		logger:          logger,
		now:             time.Now,
		quota:           contract.NewAttachmentQuota(contract.DefaultAttachmentLimits()),
		pairReadTimeout: pairReadTimeout,
		preauthSlots:    make(chan struct{}, maxPreauthRequests),
	}, nil
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pair", server.handlePair)
	mux.HandleFunc("GET /v1/session", server.handleSession)
	return mux
}

func (server *Server) Serve(ctx context.Context, listener net.Listener, certificate tls.Certificate) error {
	if listener == nil || len(certificate.Certificate) == 0 || certificate.PrivateKey == nil {
		return ErrMissingDependency
	}
	httpServer := &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 * 1024,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{certificate},
			NextProtos:   []string{"http/1.1"},
		},
	}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownContext)
	}()
	server.logger.Info("[mobile-transport] TLS server starting", "input_shape", "validated_listener,tls_certificate")
	limitedListener := newConnectionLimitedListener(listener, maxConnections)
	err := httpServer.Serve(tls.NewListener(limitedListener, httpServer.TLSConfig))
	if errors.Is(err, http.ErrServerClosed) {
		<-shutdownDone
		server.logger.Info("[mobile-transport] TLS server stopped", "branch_reason", "context_cancelled")
		return nil
	}
	if err != nil {
		server.logger.Error("[mobile-transport] TLS server failed", "error_class", fmt.Sprintf("%T", err))
	}
	return err
}

type connectionLimitedListener struct {
	net.Listener
	slots     chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func newConnectionLimitedListener(listener net.Listener, limit int) *connectionLimitedListener {
	return &connectionLimitedListener{
		Listener: listener,
		slots:    make(chan struct{}, limit),
		done:     make(chan struct{}),
	}
}

func (listener *connectionLimitedListener) Accept() (net.Conn, error) {
	select {
	case listener.slots <- struct{}{}:
	case <-listener.done:
		return nil, net.ErrClosed
	}
	connection, err := listener.Listener.Accept()
	if err != nil {
		<-listener.slots
		return nil, err
	}
	return &connectionLimitedConnection{
		Conn: connection,
		release: func() {
			<-listener.slots
		},
	}, nil
}

func (listener *connectionLimitedListener) Close() error {
	var closeError error
	listener.closeOnce.Do(func() {
		close(listener.done)
		closeError = listener.Listener.Close()
	})
	return closeError
}

type connectionLimitedConnection struct {
	net.Conn
	release func()
	once    sync.Once
}

func (connection *connectionLimitedConnection) Close() error {
	var closeError error
	connection.once.Do(func() {
		closeError = connection.Conn.Close()
		connection.release()
	})
	return closeError
}

func (server *Server) handlePair(response http.ResponseWriter, request *http.Request) {
	if !server.beginPreauthentication(response) {
		return
	}
	defer server.endPreauthentication()
	server.logger.Info("[mobile-transport] pairing request received", "input_shape", "json_body")
	response.Header().Set("Connection", "close")
	controller := http.NewResponseController(response)
	if err := controller.SetReadDeadline(time.Now().Add(server.pairReadTimeout)); err != nil {
		server.logger.Error("[mobile-transport] pairing request rejected", "branch_reason", "read_deadline_unavailable", "error_class", fmt.Sprintf("%T", err))
		http.Error(response, "Pairing temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
	request.Body = http.MaxBytesReader(response, request.Body, maxPairRequestBytes)
	var body pairRequestBody
	if err := decodeOneJSON(request.Body, &body); err != nil {
		server.logger.Warn("[mobile-transport] pairing request rejected", "branch_reason", "invalid_body", "error_class", fmt.Sprintf("%T", err))
		http.Error(response, "Invalid pairing request", http.StatusBadRequest)
		return
	}
	pairRequest, err := body.request()
	if err != nil {
		server.logger.Warn("[mobile-transport] pairing request rejected", "branch_reason", "invalid_encoding")
		http.Error(response, "Invalid pairing request", http.StatusBadRequest)
		return
	}
	record, err := server.pairing.Pair(request.Context(), pairRequest, server.now())
	if err != nil {
		server.logger.Warn("[mobile-transport] pairing request rejected", "branch_reason", "pairing_policy", "error_class", fmt.Sprintf("%T", err))
		http.Error(response, "Pairing failed", http.StatusForbidden)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(response).Encode(pairResponseBody{DeviceID: record.ID, PairingGeneration: record.PairingGeneration})
	server.logger.Info("[mobile-transport] phone paired", "device_id", record.ID, "output_shape", "device_id,pairing_generation")
}

func (server *Server) handleSession(response http.ResponseWriter, request *http.Request) {
	if !server.beginPreauthentication(response) {
		return
	}
	preauthenticationHeld := true
	defer func() {
		if preauthenticationHeld {
			server.endPreauthentication()
		}
	}()
	deviceID := request.URL.Query().Get("deviceId")
	sessionID := request.URL.Query().Get("sessionId")
	server.logger.Info("[mobile-transport] session requested", "input_shape", "websocket_upgrade")
	challenge, err := server.pairing.BeginSession(deviceID, sessionID, server.now())
	if err != nil {
		server.logger.Warn("[mobile-transport] session rejected", "branch_reason", "challenge_unavailable")
		http.Error(response, "Session rejected", http.StatusForbidden)
		return
	}
	connection, err := websocket.Accept(response, request, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		server.logger.Warn("[mobile-transport] websocket upgrade failed", "device_id", deviceID, "error_class", fmt.Sprintf("%T", err))
		return
	}
	defer connection.CloseNow()
	connection.SetReadLimit(maxPairRequestBytes)
	handshakeContext, cancel := context.WithTimeout(request.Context(), handshakeTimeout)
	defer cancel()
	if err := wsjson.Write(handshakeContext, connection, sessionChallengeBodyFrom(challenge)); err != nil {
		server.closeForPolicy(connection, "challenge_write")
		return
	}
	var proofBody sessionProofBody
	if err := wsjson.Read(handshakeContext, connection, &proofBody); err != nil {
		server.closeForPolicy(connection, "proof_read")
		return
	}
	proof, err := proofBody.proof()
	if err != nil {
		server.closeForPolicy(connection, "proof_encoding")
		return
	}
	if !proofMatchesChallenge(proof, challenge) {
		server.logger.Warn("[mobile-transport] session authentication failed", "device_id", deviceID, "session_id", sessionID, "branch_reason", "challenge_substitution")
		server.closeForPolicy(connection, "challenge_substitution")
		return
	}
	authenticatedSession, err := server.pairing.Authenticate(request.Context(), proof, server.now())
	if err != nil {
		server.logger.Warn("[mobile-transport] session authentication failed", "device_id", deviceID, "session_id", sessionID, "branch_reason", "invalid_proof")
		server.closeForPolicy(connection, "invalid_proof")
		return
	}
	server.endPreauthentication()
	preauthenticationHeld = false
	defer authenticatedSession.Close()
	attachmentKeyInput := append(append([]byte(nil), proof.HostSignature...), proof.Signature...)
	attachmentKey := sha256.Sum256(attachmentKeyInput)
	protocolSession := contract.NewSessionWithAttachments(attachmentKey[:], server.quota, sessionID, deviceID)
	connection.SetReadLimit(int64(contract.MaxAttachmentFrameBytes))
	if err := wsjson.Write(request.Context(), connection, authenticatedBody{Type: "authenticated", DeviceID: deviceID, SessionID: sessionID}); err != nil {
		return
	}
	revoked := make(chan struct{})
	go func() {
		select {
		case <-authenticatedSession.Done():
			_ = connection.Close(websocket.StatusPolicyViolation, "pairing revoked")
		case <-request.Context().Done():
		}
		close(revoked)
	}()
	server.logger.Info("[mobile-transport] session authenticated", "device_id", deviceID, "session_id", sessionID, "output_shape", "authenticated_websocket")
	server.readFrames(request.Context(), connection, protocolSession, deviceID, sessionID)
	authenticatedSession.Close()
	<-revoked
}

func (server *Server) beginPreauthentication(response http.ResponseWriter) bool {
	select {
	case server.preauthSlots <- struct{}{}:
		return true
	default:
		server.logger.Warn("[mobile-transport] request rejected", "branch_reason", "preauthentication_capacity")
		http.Error(response, "Companion busy", http.StatusServiceUnavailable)
		return false
	}
}

func (server *Server) endPreauthentication() {
	<-server.preauthSlots
}

func (server *Server) readFrames(ctx context.Context, connection *websocket.Conn, session *contract.Session, deviceID, sessionID string) {
	initialized := false
	for {
		messageType, frame, err := connection.Read(ctx)
		if err != nil {
			server.logger.Info("[mobile-transport] session ended", "device_id", deviceID, "session_id", sessionID, "branch_reason", "socket_closed")
			return
		}
		switch messageType {
		case websocket.MessageText:
			message, acceptErr := session.AcceptText(frame)
			branchReason := ""
			if acceptErr != nil {
				branchReason = "invalid_text_frame"
			} else if message.Sender != "phone" {
				branchReason = "invalid_direction"
			} else if (!initialized && message.Type != "hello") || (initialized && message.Type == "hello") {
				branchReason = "protocol_order"
			}
			if branchReason != "" {
				server.logger.Warn("[mobile-transport] frame rejected", "device_id", deviceID, "session_id", sessionID, "branch_reason", branchReason)
				server.closeForPolicy(connection, branchReason)
				return
			}
			if message.Type == "hello" {
				initialized = true
			}
			if handleErr := server.handle(ctx, deviceID, message); handleErr != nil {
				server.logger.Error("[mobile-transport] message handler failed", "device_id", deviceID, "session_id", sessionID, "error_class", fmt.Sprintf("%T", handleErr))
				_ = connection.Close(websocket.StatusInternalError, "handler failed")
				return
			}
		case websocket.MessageBinary:
			if !initialized {
				server.logger.Warn("[mobile-transport] frame rejected", "device_id", deviceID, "session_id", sessionID, "branch_reason", "protocol_order")
				server.closeForPolicy(connection, "protocol_order")
				return
			}
			if err := session.AcceptAttachmentFrame(frame); err != nil {
				server.logger.Warn("[mobile-transport] frame rejected", "device_id", deviceID, "session_id", sessionID, "branch_reason", "invalid_attachment_frame")
				server.closeForPolicy(connection, "invalid_attachment_frame")
				return
			}
		default:
			server.closeForPolicy(connection, "unsupported_frame")
			return
		}
	}
}

func (server *Server) closeForPolicy(connection *websocket.Conn, reason string) {
	_ = connection.Close(websocket.StatusPolicyViolation, reason)
}

func decodeOneJSON(reader io.Reader, output any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON value")
	}
	return nil
}
