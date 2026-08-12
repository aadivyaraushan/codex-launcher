package phoneruntime

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/disconnect"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
	"github.com/codex-launcher/codex-launcher/companion/internal/durablestore"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/transport"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge/gates"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agenttrigger"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/beeperwatch"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/localtrust"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/turnproxy"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

// ListenAddress is the fixed loopback port for phone-runtime. A process already
// holding it is an error; never scan for another port.
const ListenAddress = "127.0.0.1:9443"

// turnProxySessionKey is the OpenClaw Gateway's default single-agent session
// (see saved-results/openclaw-gateway-protocol.md). The phone agent only
// ever speaks for that one session, so there is nothing to pick per task.
const turnProxySessionKey = "agent:main:main"

// turnProxyRetryDelay is how long the connect-retry loop waits between a
// failed gateway dial and the next attempt. A package var so tests can
// shrink it instead of waiting out a real 5 seconds.
var turnProxyRetryDelay = 5 * time.Second

// turnProxyPublishAttempts and turnProxyPublishRefreshDelay mirror the
// desktop event pump's retry semantics (internal/app/eventpump.go
// maxTaskEventPublishAttempts / taskCatalogRefreshDelay): a task event for a
// task the handler's snapshot doesn't know about yet gets one snapshot
// refresh and a short wait before being retried.
const (
	turnProxyPublishAttempts     = 3
	turnProxyPublishRefreshDelay = 50 * time.Millisecond
)

var (
	ErrInvalidConfig      = errors.New("phone runtime configuration is invalid")
	ErrMissingDependency  = errors.New("phone runtime dependency is missing")
	ErrListenAddressTaken = errors.New("phone runtime listen address already in use")
)

type Config struct {
	Root          string
	DisplayName   string
	ListenAddress string
	// GatewayURL and GatewayTokenPath configure the OpenClaw Gateway
	// connection that makes the runtime task capable. GatewayTokenPath is
	// a path to the bearer token, never the token itself — it must never
	// live in this config or in a log line. Leaving both empty is valid
	// (Mac dev, and most existing tests); setting only one is not.
	GatewayURL       string
	GatewayTokenPath string
	// BeeperBaseURL is the Beeper Client API base URL the runtime watches
	// for inbound messages to trigger the agent on. Empty means no
	// watcher — the default. The watcher's bearer token is never carried
	// in this config; it comes from BEEPER_ACCESS_TOKEN or the local
	// Beeper account database (loadBeeperAccessToken, beeper_health.go).
	BeeperBaseURL string
}

type Dependencies struct {
	Random io.Reader
	Logger *slog.Logger
	Now    func() time.Time
	// BeeperAccounts probes Beeper /v1/accounts for health beeper= (tests inject; prod uses openBeeperAccounts).
	BeeperAccounts func(context.Context) ([]BeeperAccountStatus, error)
	// TurnProxyConnect dials the OpenClaw Gateway. Nil means real: Open
	// wires up an adapter around turnproxy.Connect. Tests inject a stub to
	// avoid a live websocket.
	TurnProxyConnect func(context.Context, turnproxy.Config) (TurnSource, error)
	// BeeperWatch runs the Beeper watcher. Nil means real: beeperwatch.Run.
	// Tests inject a stub to avoid a live websocket.
	BeeperWatch func(context.Context, beeperwatch.Config)
}

type Health struct {
	Mode          string            `json:"mode"`
	Process       string            `json:"process"`
	Beeper        string            `json:"beeper"`
	Credentials   map[string]string `json:"credentials"`
	Adapters      []string          `json:"adapters"`
	ListenAddress string            `json:"listenAddress"`
	TaskCapable   bool              `json:"taskCapable"`
	LocalPair     string            `json:"localPair"`
}

type Runtime struct {
	config    Config
	logger    *slog.Logger
	now       func() time.Time
	store     io.Closer
	pairing   *pairing.Service
	mobile    *transport.Server
	inventory capabilityruntime.Inventory
	mu        sync.Mutex
	process   string
	boundAddr string
	certificate  tls.Certificate
	handler      *mobilesession.Handler
	// Callers: CreateLocalPairOffer / ReleasePendingViaAttestation; Android LocalPairHandshake.
	// Affected API: pendingSessionOffer for /v1/pair enrollment after attest.
	// Attest JSON adds sessionSecret,hostPublicKey,tlsPublicKey,host,port,protocol.
	// User: "open a real session/transport to phone-runtime on loopback" via existing pairing.
	pendingOffer        *localtrust.Offer
	pendingSessionOffer *pairing.PairingOffer
	localPairAcked      bool
	operatorPin         localtrust.ExpectedOperator
	// brokerReady: Android CredentialBroker grant presence only (no secrets).
	brokerReady map[string]string
	// beeperAccounts probes local/remote Beeper for health beeper= (no tokens stored).
	beeperAccounts func(context.Context) ([]BeeperAccountStatus, error)
	// bridge serves the on-phone OpenClaw agent's tool calls over
	// /v1/agent-tools/*. Open builds it unconditionally from the production
	// inventory's runner, so it is set on every successful Open.
	bridge *agentbridge.Bridge
	// bridgeToken is the bearer token bridge checks on every request.
	bridgeToken string
	// bridgeTokenPath is where bridgeToken is persisted, so
	// AgentBridgeTokenPath can hand it to Phase 3 without exposing the
	// token itself.
	bridgeTokenPath string
	// bridgeCertPath is where the bridge's TLS certificate (public material
	// only, no key) is exported in PEM form, so AgentBridgeCertPath can hand
	// it to Phase 3 for certificate pinning.
	bridgeCertPath string
	// turnSource is the deferred TurnSource handed to the mobile handler
	// when a gateway is configured; nil when running without one.
	turnSource *deferredTurnSource
	// turnProxyCancel stops the connect-retry goroutine; Close calls it.
	// Nil when no gateway is configured, so there is nothing to stop.
	turnProxyCancel context.CancelFunc
	// turnProxyDone closes when the connect-retry goroutine returns, so
	// Close can wait for it to fully exit before closing the store out
	// from under it. Nil alongside turnProxyCancel when no gateway is
	// configured.
	turnProxyDone chan struct{}
	// beeperWatchDone closes when the Beeper watcher goroutine returns, so
	// Close can wait for it too. Nil when no watcher was started (no
	// BeeperBaseURL configured, or no access token was available).
	beeperWatchDone chan struct{}
}

func (config Config) validate() error {
	if filepath.Clean(config.Root) == "." || config.Root == "" {
		return ErrInvalidConfig
	}
	if config.DisplayName == "" {
		return ErrInvalidConfig
	}
	if config.ListenAddress == "" {
		return ErrInvalidConfig
	}
	if (config.GatewayURL == "") != (config.GatewayTokenPath == "") {
		return ErrInvalidConfig
	}
	if config.BeeperBaseURL != "" && config.GatewayURL == "" {
		// A watcher with no gateway has no agent session to deliver to.
		return ErrInvalidConfig
	}
	return nil
}

func Open(ctx context.Context, config Config, dependencies Dependencies) (*Runtime, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	if dependencies.Random == nil {
		return nil, ErrMissingDependency
	}
	logger := dependencies.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := dependencies.Now
	if now == nil {
		now = time.Now
	}
	if err := os.MkdirAll(config.Root, 0o700); err != nil {
		return nil, fmt.Errorf("create phone runtime root: %w", err)
	}
	statePath := filepath.Join(config.Root, "state.sqlite3")
	store, err := durablestore.Open(ctx, statePath, eventjournal.Limits{MaxEvents: 2048, MaxBytes: 8 * 1024 * 1024})
	if err != nil {
		return nil, err
	}
	pairingService, err := pairing.NewServiceWithLogger(ctx, store, dependencies.Random, logger)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	projectService, err := projects.NewWithLogger(nil, logger)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	journal := eventjournal.New(store, logger)
	var turnSource *deferredTurnSource
	var handler *mobilesession.Handler
	if config.GatewayURL != "" {
		// The handler is built once, right here; the gateway websocket is
		// dialed afterward (below, once the rest of Open has wired up gate
		// approvals) and may take several retries to connect. turnSource
		// stands in until then so the handler never waits on the network.
		turnSource = newDeferredTurnSource()
		handler, err = mobilesession.NewWithTaskSourceAndQueue(ctx, config.DisplayName, projectService, journal, turnSource, promptqueue.New(store, logger), logger, now)
	} else {
		handler, err = mobilesession.NewWithLogger(ctx, config.DisplayName, projectService, journal, logger, now)
	}
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	bridgeTokenPath := filepath.Join(config.Root, "agentbridge-token")
	beeperAccounts := dependencies.BeeperAccounts
	if beeperAccounts == nil {
		beeperAccounts = openBeeperAccounts(logger)
	}

	prod := capabilityruntime.ProductionConfig{
		Logger:            logger,
		MapsBrokerBaseURL: phoneMapsBrokerBaseURL(),
	}
	// Fact-force (edit): callers=Open phone-runtime NewProduction;
	// API=BeeperAPI+BeeperReadOnly; user: Slice 4 B4+B5.
	if api, readOnly := beeperAPIFromEnv(logger); api != nil {
		prod.BeeperAPI = api
		prod.BeeperReadOnly = readOnly
	}
	inventory, buildErr := capabilityruntime.NewProduction(prod)
	if buildErr != nil {
		_ = store.Close()
		return nil, buildErr
	}

	// The agent bridge needs a runner to hand the OpenClaw agent; it is
	// built unconditionally from the inventory NewProduction just returned
	// — there is no longer a routed flow that could stand in for it.
	token, tokenErr := loadOrMintBridgeToken(bridgeTokenPath, dependencies.Random)
	if tokenErr != nil {
		_ = store.Close()
		return nil, tokenErr
	}
	bridgeToken := token
	gateStore := newDurableGateStore(ctx, store)

	// approvals and router form a cycle with bridge: approvals needs the
	// router to register decisions on, the router needs approvals to
	// resolve them, and approvals needs the bridge to actually release a
	// gate — but the bridge itself needs approvals (as its Notifier)
	// before it exists. Build approvals and the router first with sink
	// and releaser left nil, wire the router in, then backfill releaser
	// once bridge is built below.
	approvals := newGateApprovals(logger, now, nil, handler, nil)
	router := decisions.NewRouter(approvals, logger)
	approvals.sink = router
	handler.EnableDecisions(router)

	disconnector := disconnect.New(inventory.Runner(), capabilityruntime.ConsentGate(), logger)
	bridge := agentbridge.New(inventory, inventory.Runner(), token, logger, agentbridge.GateDeps{
		Policy:       gates.New(gateStore, newGateIDFunc(logger)),
		Store:        gateStore,
		Notifier:     approvals,
		Disconnector: disconnector,
		// Read fresh on every call, deliberately: an owner edit to
		// agent-rules.md must apply to the very next send, not wait for
		// a restart.
		AllowListed: func(recipient string) bool {
			rules, err := agenttrigger.EnsureRules(config.Root)
			if err != nil {
				return false
			}
			return agenttrigger.Allowed(rules, recipient)
		},
	})
	approvals.releaser = bridge

	var turnProxyCancel context.CancelFunc
	var turnProxyDone chan struct{}
	var beeperWatchDone chan struct{}
	if turnSource != nil {
		connect := dependencies.TurnProxyConnect
		if connect == nil {
			connect = connectTurnProxy
		}
		var connectCtx context.Context
		connectCtx, turnProxyCancel = context.WithCancel(context.Background())
		turnProxyDone = make(chan struct{})
		go func() {
			defer close(turnProxyDone)
			runTurnProxyConnect(connectCtx, connect, config.GatewayURL, config.GatewayTokenPath, turnSource, handler, logger)
		}()

		if config.BeeperBaseURL != "" {
			// The watcher shares the turn proxy's cancel so both stop
			// together on teardown; its own done channel is nil (and stays
			// nil) if there is no access token to start it with.
			if token := loadBeeperAccessToken(); token == "" {
				logger.Warn("[phone-runtime] Beeper watcher not started", "reason", "no_access_token")
			} else {
				watch := dependencies.BeeperWatch
				if watch == nil {
					watch = beeperwatch.Run
				}
				delivery := &triggerDelivery{root: config.Root, turnSource: turnSource, logger: logger}
				beeperWatchDone = make(chan struct{})
				go func() {
					defer close(beeperWatchDone)
					watch(connectCtx, beeperwatch.Config{
						BaseURL: config.BeeperBaseURL,
						Token:   token,
						Notify:  delivery.deliver,
						Logger:  logger,
					})
				}()
			}
		}
	}

	mobileServer, err := transport.NewServer(pairingService, handler.Handle, logger)
	if err != nil {
		stopTurnProxyConnect(turnProxyCancel, turnProxyDone, beeperWatchDone, turnSource, logger)
		_ = store.Close()
		return nil, err
	}
	certificate, err := pairingService.TLSCertificate(now())
	if err != nil {
		stopTurnProxyConnect(turnProxyCancel, turnProxyDone, beeperWatchDone, turnSource, logger)
		_ = store.Close()
		return nil, err
	}
	// The certificate is re-minted on every Open (pairingService.TLSCertificate
	// above), so it is re-exported every time too — the OpenClaw plugin
	// (Phase 3) reads this file fresh on each request and must see whatever
	// the runtime is currently serving, not a stale cert from a prior run.
	bridgeCertPath := filepath.Join(config.Root, "agentbridge-cert.pem")
	if err := writeAgentBridgeCert(bridgeCertPath, certificate); err != nil {
		stopTurnProxyConnect(turnProxyCancel, turnProxyDone, beeperWatchDone, turnSource, logger)
		_ = store.Close()
		return nil, err
	}

	rt := &Runtime{
		config:          config,
		logger:          logger,
		now:             now,
		store:           store,
		pairing:         pairingService,
		mobile:          mobileServer,
		inventory:       inventory,
		process:         "ready",
		certificate:     certificate,
		handler:         handler,
		bridge:          bridge,
		bridgeToken:     bridgeToken,
		bridgeTokenPath: bridgeTokenPath,
		bridgeCertPath:  bridgeCertPath,
		turnSource:      turnSource,
		turnProxyCancel: turnProxyCancel,
		turnProxyDone:   turnProxyDone,
		beeperWatchDone: beeperWatchDone,
		operatorPin: localtrust.ExpectedOperator{
			PackageName: "app.codexlauncher",
			// Release (frozen owner) APK signer. Debug builds use c613e660… — accept both below.
			SigningCertSHA256: "35639edad0224145765b0d0f2b21cd9c8cd96be6592bdfbb4b8f39d4a9f3f3e3",
		},
	}
	if _, err := os.Stat(filepath.Join(config.Root, "android-auth.json")); err == nil {
		rt.localPairAcked = true
	}
	rt.brokerReady = loadBrokerReady(config.Root)
	rt.beeperAccounts = beeperAccounts
	logger.Info("[phone-runtime] opened", "mode", "standalone_phone", "root", config.Root, "listen", config.ListenAddress, "registered_count", len(inventory.Registered), "task_capable", false, "local_pair_acked", rt.localPairAcked, "broker_ready_count", len(rt.brokerReady), "beeper_probe", beeperAccounts != nil, "gateway_configured", turnSource != nil)
	return rt, nil
}

// connectTurnProxy is the production TurnProxyConnect: it dials the real
// gateway and returns its *turnproxy.Source, which already satisfies
// TurnSource.
func connectTurnProxy(ctx context.Context, cfg turnproxy.Config) (TurnSource, error) {
	return turnproxy.Connect(ctx, cfg)
}

// gatewayTaskEventHandler is the subset of *mobilesession.Handler that
// gatewayPublisher and runTurnProxyConnect need. It exists so tests could
// substitute a stub, though production always hands in the real handler.
type gatewayTaskEventHandler interface {
	PublishTaskEvent(context.Context, taskstate.MobileEvent) error
	RefreshTaskSnapshot(context.Context) error
}

// gatewayPublisher adapts the mobile session handler to turnproxy's
// EventPublisher, mirroring the desktop event pump's retry semantics
// (internal/app/eventpump.go publishTaskEvent): the handler's task snapshot
// can lag the gateway's turn-proxy task by a beat (freshly connected, or
// mid reconnect), so ErrUnknownTaskEvent gets one snapshot refresh and a
// short wait before the event is retried, rather than being dropped.
type gatewayPublisher struct {
	handler gatewayTaskEventHandler
	logger  *slog.Logger
}

func (publisher *gatewayPublisher) PublishTaskEvent(ctx context.Context, event taskstate.MobileEvent) error {
	var lastErr error
	for attempt := 1; attempt <= turnProxyPublishAttempts; attempt++ {
		lastErr = publisher.handler.PublishTaskEvent(ctx, event)
		if lastErr == nil {
			return nil
		}
		if errors.Is(lastErr, mobilesession.ErrTaskEventAuthorizationRevoked) {
			return lastErr
		}
		if errors.Is(lastErr, mobilesession.ErrUnknownTaskEvent) && attempt < turnProxyPublishAttempts {
			if refreshErr := publisher.handler.RefreshTaskSnapshot(ctx); refreshErr != nil {
				return fmt.Errorf("phone runtime: refresh task snapshot: %w", refreshErr)
			}
			publisher.logger.Info("[phone-runtime] turn proxy task catalog refreshed", "thread_id", event.TaskID, "attempt", attempt)
			if !waitOrCanceled(ctx, turnProxyPublishRefreshDelay) {
				return ctx.Err()
			}
			continue
		}
		return lastErr
	}
	return lastErr
}

// triggerDelivery turns one inbound Beeper message into a triggered turn on
// the phone agent's task; deliver is beeperwatch.Config's Notify.
type triggerDelivery struct {
	root       string
	turnSource *deferredTurnSource
	logger     *slog.Logger
}

// deliver loads the owner's rules, builds the trigger prompt and preview,
// and starts a triggered turn. Failures are logged with the message id
// only — never the message text or any token, matching beeperwatch's own
// logging discipline — and otherwise just drop the message rather than
// retrying, the same at-most-once delivery beeperwatch itself promises.
func (delivery *triggerDelivery) deliver(ctx context.Context, msg beeperwatch.Message) {
	rules, err := agenttrigger.EnsureRules(delivery.root)
	if err != nil {
		if rules == "" {
			delivery.logger.Error("[phone-runtime] agent rules unavailable, dropping trigger", "id", msg.ID, "error", err.Error())
			return
		}
		delivery.logger.Error("[phone-runtime] agent rules read failed, using the rules it returned anyway", "id", msg.ID, "error", err.Error())
	}
	prompt := agenttrigger.Prompt(msg, rules)
	preview := agenttrigger.Preview(msg)
	if _, err := delivery.turnSource.StartTriggeredTurn(ctx, agentGateThreadID, prompt, preview); err != nil {
		delivery.logger.Error("[phone-runtime] triggered turn failed", "id", msg.ID, "error", err.Error())
	}
}

// runTurnProxyConnect owns the gateway connection for the runtime's
// lifetime: it dials with retry, hands the connected source to deferred,
// and — since a gateway connection can drop at any time (the process
// restarting, the network dropping) — redials for as long as ctx stays
// alive rather than returning after the first successful connect.
func runTurnProxyConnect(ctx context.Context, connect func(context.Context, turnproxy.Config) (TurnSource, error), gatewayURL, tokenPath string, deferred *deferredTurnSource, handler gatewayTaskEventHandler, logger *slog.Logger) {
	publisher := &gatewayPublisher{handler: handler, logger: logger}
	var carried taskstate.LastMessage
	for {
		source, ok := dialTurnProxyWithRetry(ctx, connect, gatewayURL, tokenPath, publisher, logger, carried)
		if !ok {
			return
		}
		deferred.set(source)
		logger.Info("[phone-runtime] turn proxy connected")
		if err := handler.RefreshTaskSnapshot(ctx); err != nil {
			logger.Error("[phone-runtime] turn proxy post-connect snapshot refresh failed", "error", err.Error())
		}

		select {
		case <-ctx.Done():
			return
		case <-source.Done():
			// The dropped source is the only place that still knows what it
			// last said — read it before closing so the redial's Config can
			// seed the next Source and the Home preview doesn't go blank.
			if task, err := source.CurrentTask(ctx, agentGateThreadID); err != nil {
				logger.Error("[phone-runtime] turn proxy last-message read before close failed", "error", err.Error())
			} else {
				carried = task.LastMessage
			}
			// Clear and close before looping back to redial — the runtime
			// must never report TaskCapable on a socket that is already
			// dead, and the old connection must be fully released before a
			// new one takes its place.
			if old := deferred.clear(); old != nil {
				if err := old.Close(); err != nil {
					logger.Error("[phone-runtime] turn proxy close after drop failed", "error", err.Error())
				}
			}
			logger.Info("[phone-runtime] turn proxy connection dropped, reconnecting")
		}
	}
}

// dialTurnProxyWithRetry dials the gateway and retries until it succeeds or
// ctx is canceled, returning (nil, false) in the latter case. The token is
// read fresh from disk on every attempt rather than once up front, so
// rotating the file without restarting the runtime still works the next
// time it is read — and a token file that is briefly missing or unreadable
// is just another retryable failure, not a fatal one.
func dialTurnProxyWithRetry(ctx context.Context, connect func(context.Context, turnproxy.Config) (TurnSource, error), gatewayURL, tokenPath string, publisher turnproxy.EventPublisher, logger *slog.Logger, initialLastMessage taskstate.LastMessage) (TurnSource, bool) {
	for {
		if ctx.Err() != nil {
			return nil, false
		}
		raw, err := os.ReadFile(tokenPath)
		if err != nil {
			logger.Error("[phone-runtime] turn proxy token unreadable", "error", err.Error())
			if !waitOrCanceled(ctx, turnProxyRetryDelay) {
				return nil, false
			}
			continue
		}
		cfg := turnproxy.Config{
			URL:                gatewayURL,
			Token:              strings.TrimSpace(string(raw)),
			TaskID:             agentGateThreadID,
			SessionKey:         turnProxySessionKey,
			Publisher:          publisher,
			Logger:             logger,
			InitialLastMessage: initialLastMessage,
		}
		source, err := connect(ctx, cfg)
		if err != nil {
			logger.Error("[phone-runtime] turn proxy connect failed", "error", err.Error())
			if !waitOrCanceled(ctx, turnProxyRetryDelay) {
				return nil, false
			}
			continue
		}
		if ctx.Err() != nil {
			// The dial can resolve successfully after Close already
			// canceled ctx — the source must not be handed back for
			// installation into a runtime that is tearing down.
			if closeErr := source.Close(); closeErr != nil {
				logger.Error("[phone-runtime] turn proxy close of late connect failed", "error", closeErr.Error())
			}
			return nil, false
		}
		return source, true
	}
}

// stopTurnProxyConnect cancels the connect-retry goroutine (and, sharing the
// same cancel, the Beeper watcher goroutine if one was started), waits for
// both to actually exit, so a caller closing the store next never races
// either still using it, and then closes whatever source the connect
// goroutine installed — so every teardown path (Close and Open's error
// paths) releases the socket identically. A nil cancel means no gateway was
// configured — there is nothing to stop, and this must not block or panic
// in that case. A nil beeperDone means the watcher never started, so there
// is nothing to wait for there either.
func stopTurnProxyConnect(cancel context.CancelFunc, done, beeperDone chan struct{}, source *deferredTurnSource, logger *slog.Logger) {
	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
	if beeperDone != nil {
		<-beeperDone
	}
	if source != nil {
		if err := source.Close(); err != nil {
			logger.Error("[phone-runtime] turn proxy close failed", "error", err.Error())
		}
	}
}

// waitOrCanceled sleeps for delay, returning false early (without sleeping
// out the rest of delay) if ctx is canceled first.
func waitOrCanceled(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// loadOrMintBridgeToken returns the agent-bridge bearer token at path,
// minting one on first run. A file that exists with non-empty content wins
// over minting a new one, so the token survives a restart — the OpenClaw
// plugin (Phase 3) reads it once and expects it to keep working.
func loadOrMintBridgeToken(path string, random io.Reader) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			return token, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", fmt.Errorf("mint agent-bridge token: %w", err)
	}
	token := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return "", fmt.Errorf("persist agent-bridge token: %w", err)
	}
	return token, nil
}

// AgentBridgeTokenPath returns where the agent-bridge bearer token is
// persisted, so Phase 3 can read it without this package handing out the
// token itself.
func (runtime *Runtime) AgentBridgeTokenPath() string {
	return runtime.bridgeTokenPath
}

// writeAgentBridgeCert exports the certificate's leaf DER (certificate.
// Certificate[0]) as a single PEM CERTIFICATE block at path. It is public
// material only — no private key — so it is written world-readable
// (0o644) rather than the 0o600 used for the bearer token.
func writeAgentBridgeCert(path string, certificate tls.Certificate) error {
	if len(certificate.Certificate) == 0 {
		return fmt.Errorf("phone runtime: TLS certificate has no leaf to export")
	}
	block := &pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o644); err != nil {
		return fmt.Errorf("export agent-bridge certificate: %w", err)
	}
	return nil
}

// AgentBridgeCertPath returns where the agent-bridge's TLS certificate is
// exported in PEM form, so Phase 3 can pin it without a private trust store.
func (runtime *Runtime) AgentBridgeCertPath() string {
	return runtime.bridgeCertPath
}

func (runtime *Runtime) Close() error {
	if runtime == nil || runtime.store == nil {
		return nil
	}
	stopTurnProxyConnect(runtime.turnProxyCancel, runtime.turnProxyDone, runtime.beeperWatchDone, runtime.turnSource, runtime.logger)
	return runtime.store.Close()
}

// TaskCapable reports whether the runtime has a live gateway connection to
// drive turns on. False both when no gateway is configured (Mac dev) and
// when one is configured but the connect-retry loop hasn't succeeded yet.
func (runtime *Runtime) TaskCapable() bool {
	return runtime != nil && runtime.turnSource != nil && runtime.turnSource.connected()
}

func (runtime *Runtime) HasPendingLocalPairOffer() bool {
	if runtime == nil {
		return false
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.pendingOffer != nil && runtime.pendingOffer.Secret != ""
}

// CreateLocalPairOffer builds a public offer pinned to this process TLS leaf and
// keeps the pairing secret only in memory for ReleaseSecret.
func (runtime *Runtime) CreateLocalPairOffer() (localtrust.PublicOffer, error) {
	if runtime == nil {
		return localtrust.PublicOffer{}, ErrMissingDependency
	}
	offer, err := localtrust.NewOffer(localtrust.OfferParams{
		Port:      9443,
		ExpiresIn: 10 * time.Minute,
		Now:       runtime.now().UTC(),
	})
	if err != nil {
		return localtrust.PublicOffer{}, err
	}
	leaf := runtime.certificate.Leaf
	if leaf == nil && len(runtime.certificate.Certificate) > 0 {
		leaf, err = x509.ParseCertificate(runtime.certificate.Certificate[0])
		if err != nil {
			return localtrust.PublicOffer{}, err
		}
	}
	if leaf == nil {
		return localtrust.PublicOffer{}, fmt.Errorf("phone runtime: missing TLS leaf for local-pair pin")
	}
	spki := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	offer.Public.RuntimeIdentity = base64.RawURLEncoding.EncodeToString(leaf.RawSubjectPublicKeyInfo)
	offer.Public.TLSSPKI = base64.RawURLEncoding.EncodeToString(spki[:])
	sessionOffer, err := runtime.pairing.BeginPairing(pairing.PairingTarget{
		Host:     "127.0.0.1",
		Port:     9443,
		Protocol: pairing.ProtocolMajor,
	}, runtime.now())
	if err != nil {
		return localtrust.PublicOffer{}, fmt.Errorf("phone runtime: begin local session pairing: %w", err)
	}
	runtime.mu.Lock()
	runtime.pendingOffer = offer
	runtime.pendingSessionOffer = &sessionOffer
	runtime.mu.Unlock()
	runtime.logger.Info("[phone-runtime] local-pair offer created", "offer_id", offer.Public.OfferID, "port", offer.Public.Port)
	return offer.Public, nil
}

type localPairAttestRequest struct {
	Nonce                string   `json:"nonce"`
	ChainDERBase64       []string `json:"chainDerBase64"`
	AndroidAuthPublicKey string   `json:"androidAuthPublicKey"`
}

type localPairAttestResponse struct {
	OfferID       string `json:"offerId"`
	Secret        string `json:"secret"`
	SessionSecret string `json:"sessionSecret"`
	HostPublicKey string `json:"hostPublicKey"`
	TLSPublicKey  string `json:"tlsPublicKey"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Protocol      int    `json:"protocol"`
}

type localPairAckRequest struct {
	OfferID              string `json:"offerId"`
	AndroidAuthPublicKey string `json:"androidAuthPublicKey"`
}

func (runtime *Runtime) loopbackOnly(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

func (runtime *Runtime) ReleasePendingViaAttestation(req localPairAttestRequest) (localPairAttestResponse, error) {
	var empty localPairAttestResponse
	if runtime == nil {
		return empty, ErrMissingDependency
	}
	runtime.mu.Lock()
	offer := runtime.pendingOffer
	runtime.mu.Unlock()
	if offer == nil {
		return empty, fmt.Errorf("local-pair: no pending offer")
	}
	if req.Nonce == "" || len(req.ChainDERBase64) == 0 {
		return empty, fmt.Errorf("local-pair: nonce and chain required")
	}
	chain := make([][]byte, 0, len(req.ChainDERBase64))
	for i, b64 := range req.ChainDERBase64 {
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			raw, err = base64.RawURLEncoding.DecodeString(b64)
		}
		if err != nil {
			return empty, fmt.Errorf("local-pair: chain[%d] decode: %w", i, err)
		}
		chain = append(chain, raw)
	}
	roots, rootFPs, err := localtrust.LoadGoogleAttestationRoots()
	if err != nil {
		return empty, err
	}
	leaf, rootFP, err := localtrust.VerifyAndroidKeyAttestationChain(chain, roots)
	if err != nil {
		runtime.logger.Info("[phone-runtime] local-pair attest rejected", "reason", "chain", "error", err.Error())
		return empty, err
	}
	if _, ok := rootFPs[rootFP]; !ok {
		return empty, fmt.Errorf("%w: root not pinned", localtrust.ErrAttestUntrustedRoot)
	}
	parsed, err := localtrust.ParseAndroidKeyAttestation(leaf)
	if err != nil {
		runtime.logger.Info("[phone-runtime] local-pair attest rejected", "reason", "parse", "error", err.Error())
		return empty, err
	}
	parsed.RootFingerprint = rootFP
	wantChallenge := localtrust.ChallengeBytes(
		offer.Public.OfferID,
		offer.Public.RuntimeIdentity,
		offer.Public.EphemeralPub,
		offer.Public.TLSSPKI,
		req.Nonce,
	)
	pkg := ""
	for _, p := range parsed.PackageNames {
		if p == runtime.operatorPin.PackageName {
			pkg = p
			break
		}
	}
	signer := ""
	allowedSigners := map[string]struct{}{
		strings.ToLower(strings.ReplaceAll(runtime.operatorPin.SigningCertSHA256, ":", "")): {},
		// debug keystore (local instrumentation / older overnight installs)
		"c613e6607c404042e6913517278638946b29c18dc1cd81e92c9d3699a580c25d": {},
	}
	for _, d := range parsed.SignatureDigests {
		if _, ok := allowedSigners[d]; ok {
			signer = d
			break
		}
	}
	wantSigner := signer
	if wantSigner == "" {
		wantSigner = strings.ToLower(strings.ReplaceAll(runtime.operatorPin.SigningCertSHA256, ":", ""))
	}
	obs := localtrust.AttestObserved{
		PackageName:       pkg,
		SigningCertSHA256: signer,
		Challenge:         parsed.Challenge,
		Level:             parsed.Level,
		RootFingerprint:   rootFP,
		Revoked:           false,
	}
	exp := localtrust.AttestExpected{
		PackageName:            runtime.operatorPin.PackageName,
		SigningCertSHA256:      wantSigner,
		OfferID:                offer.Public.OfferID,
		RuntimeIdentity:        offer.Public.RuntimeIdentity,
		EphemeralPublicKey:     offer.Public.EphemeralPub,
		TLSSPKI:                offer.Public.TLSSPKI,
		TranscriptNonce:        req.Nonce,
		PinnedRootFingerprints: map[string]struct{}{rootFP: {}},
	}
	if !bytesEqual(parsed.Challenge, wantChallenge) {
		obs.Challenge = parsed.Challenge
	}
	secret, err := offer.ReleaseSecret(exp, obs)
	if err != nil {
		runtime.logger.Info("[phone-runtime] local-pair attest rejected", "reason", err.Error(), "level", string(parsed.Level), "packages", strings.Join(parsed.PackageNames, ","))
		return empty, err
	}
	runtime.mu.Lock()
	sessionOffer := runtime.pendingSessionOffer
	runtime.pendingOffer = nil
	runtime.pendingSessionOffer = nil
	runtime.mu.Unlock()
	runtime.logger.Info("[phone-runtime] local-pair secret released", "offer_id", offer.Public.OfferID, "android_key_present", req.AndroidAuthPublicKey != "", "session_enroll", sessionOffer != nil)
	resp := localPairAttestResponse{OfferID: offer.Public.OfferID, Secret: secret}
	if sessionOffer != nil {
		resp.SessionSecret = sessionOffer.Secret
		resp.HostPublicKey = sessionOffer.HostPublicKey
		resp.TLSPublicKey = sessionOffer.TLSPublicKey
		resp.Host = sessionOffer.Target.Host
		resp.Port = sessionOffer.Target.Port
		resp.Protocol = sessionOffer.Target.Protocol
	}
	return resp, nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (runtime *Runtime) AckLocalPair(req localPairAckRequest) error {
	if runtime == nil {
		return ErrMissingDependency
	}
	if req.OfferID == "" || req.AndroidAuthPublicKey == "" {
		return fmt.Errorf("local-pair: ack requires offerId and androidAuthPublicKey")
	}
	payload := map[string]any{
		"offerId":              req.OfferID,
		"androidAuthPublicKey": req.AndroidAuthPublicKey,
		"epoch":                1,
		"storedAt":             runtime.now().UTC().Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(runtime.config.Root, "android-auth.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	runtime.mu.Lock()
	runtime.localPairAcked = true
	runtime.mu.Unlock()
	runtime.logger.Info("[phone-runtime] local-pair ack durable", "offer_id", req.OfferID, "path", path)
	return nil
}

func (runtime *Runtime) BoundAddress() string {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.boundAddr
}

func (runtime *Runtime) Health() Health {
	runtime.mu.Lock()
	process := runtime.process
	listen := runtime.config.ListenAddress
	if runtime.boundAddr != "" {
		listen = runtime.boundAddr
	}
	runtime.mu.Unlock()

	adapters := append([]string(nil), runtime.inventory.Registered...)
	credentials := map[string]string{}
	for id, reason := range runtime.inventory.Skipped {
		lower := strings.ToLower(reason)
		if strings.Contains(lower, "keychain") || strings.Contains(lower, "macos") {
			credentials[id] = "unavailable:android_broker_pending"
			continue
		}
		credentials[id] = "unavailable:" + reason
	}
	runtime.mu.Lock()
	for id, status := range runtime.brokerReady {
		if status == "ready" {
			credentials[id] = "ready"
		}
	}
	runtime.mu.Unlock()
	beeper := classifyBeeperHealth(runtime.beeperAccounts)
	runtime.mu.Lock()
	acked := runtime.localPairAcked
	pending := runtime.pendingOffer != nil
	runtime.mu.Unlock()
	localPair := "unpaired"
	if acked {
		localPair = "acked"
	} else if pending {
		localPair = "offer_pending"
	}
	return Health{
		Mode:          "standalone_phone",
		Process:       process,
		Beeper:        beeper,
		Credentials:   credentials,
		Adapters:      adapters,
		ListenAddress: listen,
		TaskCapable:   runtime.TaskCapable(),
		LocalPair:     localPair,
	}
}

func (runtime *Runtime) Serve(ctx context.Context) error {
	if runtime == nil || runtime.mobile == nil {
		return ErrMissingDependency
	}
	listener, err := net.Listen("tcp", runtime.config.ListenAddress)
	if err != nil {
		runtime.setProcess("failed")
		if runtime.config.ListenAddress == ListenAddress {
			return fmt.Errorf("%w: %s: %v", ErrListenAddressTaken, ListenAddress, err)
		}
		return err
	}
	runtime.mu.Lock()
	runtime.boundAddr = listener.Addr().String()
	runtime.process = "serving"
	runtime.mu.Unlock()
	runtime.logger.Info("[phone-runtime] listening", "address", runtime.boundAddr, "mode", "standalone_phone")

	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Path == "/v1/health" {
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(runtime.Health())
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == "/v1/local-pair/offer" {
			if !runtime.loopbackOnly(request.RemoteAddr) {
				http.Error(writer, "loopback only", http.StatusForbidden)
				runtime.logger.Info("[phone-runtime] local-pair offer rejected", "reason", "non_loopback", "remote", request.RemoteAddr)
				return
			}
			pub, err := runtime.CreateLocalPairOffer()
			if err != nil {
				runtime.logger.Info("[phone-runtime] local-pair offer failed", "error", err.Error())
				http.Error(writer, "offer create failed", http.StatusInternalServerError)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(writer).Encode(pub)
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == "/v1/local-pair/attest" {
			if !runtime.loopbackOnly(request.RemoteAddr) {
				http.Error(writer, "loopback only", http.StatusForbidden)
				return
			}
			var req localPairAttestRequest
			if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
				http.Error(writer, "bad json", http.StatusBadRequest)
				return
			}
			resp, err := runtime.ReleasePendingViaAttestation(req)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusForbidden)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(writer).Encode(resp)
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == "/v1/local-pair/ack" {
			if !runtime.loopbackOnly(request.RemoteAddr) {
				http.Error(writer, "loopback only", http.StatusForbidden)
				return
			}
			var req localPairAckRequest
			if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
				http.Error(writer, "bad json", http.StatusBadRequest)
				return
			}
			if err := runtime.AckLocalPair(req); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]string{"status": "acked"})
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == "/v1/credentials/broker-status" {
			if !runtime.loopbackOnly(request.RemoteAddr) {
				http.Error(writer, "loopback only", http.StatusForbidden)
				return
			}
			var raw map[string]json.RawMessage
			if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
				http.Error(writer, "bad json", http.StatusBadRequest)
				return
			}
			if err := runtime.applyBrokerStatusRequest(raw); err != nil {
				status := http.StatusBadRequest
				if errors.Is(err, ErrBrokerStatusSecret) {
					status = http.StatusForbidden
				}
				http.Error(writer, err.Error(), status)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
			return
		}
		if strings.HasPrefix(request.URL.Path, "/v1/agent-tools/") {
			if !runtime.loopbackOnly(request.RemoteAddr) {
				http.Error(writer, "loopback only", http.StatusForbidden)
				return
			}
			if runtime.bridge == nil {
				http.Error(writer, "agent bridge unavailable", http.StatusServiceUnavailable)
				return
			}
			runtime.bridge.Handler().ServeHTTP(writer, request)
			return
		}
		runtime.mobile.Handler().ServeHTTP(writer, request)
	})

	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 * 1024,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{runtime.certificate},
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
	err = httpServer.Serve(tls.NewListener(listener, httpServer.TLSConfig))
	runtime.setProcess("stopped")
	if errors.Is(err, http.ErrServerClosed) {
		<-shutdownDone
		return nil
	}
	runtime.setProcess("failed")
	return err
}

func (runtime *Runtime) setProcess(state string) {
	runtime.mu.Lock()
	runtime.process = state
	runtime.mu.Unlock()
}

// TestHTTPClient returns a client that pins this runtime's TLS leaf. Pairing
// certificates are identity pins, not hostname certs, so the client skips the
// hostname SAN check and requires the exact leaf bytes instead.
func (runtime *Runtime) TestHTTPClient() *http.Client {
	want := runtime.certificate.Leaf
	if want == nil && len(runtime.certificate.Certificate) > 0 {
		if cert, err := x509.ParseCertificate(runtime.certificate.Certificate[0]); err == nil {
			want = cert
		}
	}
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS13,
				InsecureSkipVerify: true,
				VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
					if want == nil || len(rawCerts) == 0 {
						return errors.New("phone runtime TLS leaf missing")
					}
					got, err := x509.ParseCertificate(rawCerts[0])
					if err != nil {
						return err
					}
					if !got.Equal(want) {
						return errors.New("phone runtime TLS leaf mismatch")
					}
					return nil
				},
			},
		},
	}
}
