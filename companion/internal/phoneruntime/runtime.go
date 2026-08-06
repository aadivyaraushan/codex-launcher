package phoneruntime

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
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
	deeplinkadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/deeplink"
	stage1explicit "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/explicit"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
	"github.com/codex-launcher/codex-launcher/companion/internal/durablestore"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/transport"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/localtrust"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

// ListenAddress is the fixed loopback port for phone-runtime. A process already
// holding it is an error; never scan for another port.
const ListenAddress = "127.0.0.1:9443"

var (
	ErrInvalidConfig      = errors.New("phone runtime configuration is invalid")
	ErrMissingDependency  = errors.New("phone runtime dependency is missing")
	ErrListenAddressTaken = errors.New("phone runtime listen address already in use")
)

type Config struct {
	Root          string
	DisplayName   string
	ListenAddress string
}

type Dependencies struct {
	Random io.Reader
	Logger *slog.Logger
	Now    func() time.Time
	Flow   mobilesession.CapabilityFlow
	// BeeperAccounts probes Beeper /v1/accounts for health beeper= (tests inject; prod uses openBeeperAccounts).
	BeeperAccounts func(context.Context) ([]BeeperAccountStatus, error)
}

type Health struct {
	Mode          string            `json:"mode"`
	Process       string            `json:"process"`
	Router        string            `json:"router"`
	Beeper        string            `json:"beeper"`
	Credentials   map[string]string `json:"credentials"`
	Adapters      []string          `json:"adapters"`
	ListenAddress string            `json:"listenAddress"`
	TaskCapable   bool              `json:"taskCapable"`
	LocalPair     string            `json:"localPair"`
}

type Runtime struct {
	config       Config
	logger       *slog.Logger
	now          func() time.Time
	store        io.Closer
	pairing      *pairing.Service
	mobile       *transport.Server
	flow         mobilesession.CapabilityFlow
	inventory    capabilityruntime.Inventory
	router       string
	mu           sync.Mutex
	process      string
	boundAddr    string
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
	handler, err := mobilesession.NewWithLogger(ctx, config.DisplayName, projectService, journal, logger, now)
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	flow := dependencies.Flow
	inventory := capabilityruntime.Inventory{}
	routerSource := "explicit_app"
	beeperAccounts := dependencies.BeeperAccounts
	if beeperAccounts == nil {
		beeperAccounts = openBeeperAccounts(logger)
	}
	if flow == nil {
		specs := deeplinkadapter.Wave1Specs()
		rules := make([]stage1explicit.Rule, 0, len(specs))
		for _, spec := range specs {
			rules = append(rules, stage1explicit.Rule{ID: spec.ID, Name: spec.AppName, AppClass: spec.AppClass, Verbs: spec.Verbs})
		}
		model := stage1explicit.New(rules, logger)
		// Fact-force: callers=New/Serve phone-runtime; API=ProductionConfig.MapsBrokerBaseURL;
		// user: "Maps Go→Android Places/Routes RPC"
		prod := capabilityruntime.ProductionConfig{
			Model:             model.Route,
			Logger:            logger,
			MapsBrokerBaseURL: phoneMapsBrokerBaseURL(),
		}
		if api := beeperAPIFromEnv(logger); api != nil {
			prod.BeeperAPI = api
		}
		service, inv, buildErr := capabilityruntime.NewProduction(prod)
		if buildErr != nil {
			_ = store.Close()
			return nil, buildErr
		}
		flow = service
		inventory = inv
	}
	handler.EnableCapabilities(flow)

	mobileServer, err := transport.NewServer(pairingService, handler.Handle, logger)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	certificate, err := pairingService.TLSCertificate(now())
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	rt := &Runtime{
		config:      config,
		logger:      logger,
		now:         now,
		store:       store,
		pairing:     pairingService,
		mobile:      mobileServer,
		flow:        flow,
		inventory:   inventory,
		router:      routerSource,
		process:     "ready",
		certificate: certificate,
		handler:     handler,
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
	logger.Info("[phone-runtime] opened", "mode", "standalone_phone", "root", config.Root, "listen", config.ListenAddress, "registered_count", len(inventory.Registered), "task_capable", false, "local_pair_acked", rt.localPairAcked, "broker_ready_count", len(rt.brokerReady), "beeper_probe", beeperAccounts != nil)
	return rt, nil
}

func (runtime *Runtime) Close() error {
	if runtime == nil || runtime.store == nil {
		return nil
	}
	return runtime.store.Close()
}

func (runtime *Runtime) TaskCapable() bool { return false }

func (runtime *Runtime) CapabilityCapable() bool {
	return runtime != nil && runtime.flow != nil
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
	OfferID        string `json:"offerId"`
	Secret         string `json:"secret"`
	SessionSecret  string `json:"sessionSecret"`
	HostPublicKey  string `json:"hostPublicKey"`
	TLSPublicKey   string `json:"tlsPublicKey"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Protocol       int    `json:"protocol"`
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
		Router:        runtime.router,
		Beeper:        beeper,
		Credentials:   credentials,
		Adapters:      adapters,
		ListenAddress: listen,
		TaskCapable:   false,
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
