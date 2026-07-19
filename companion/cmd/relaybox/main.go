// Command relaybox runs the meeting-point relay box described in
// planning/relay-box-build-plan.md: a small program that lets a phone reach
// a Mac companion without both sides being on the same private network. All
// the actual relay logic lives in internal/relaybox; this file is the thin
// shell that reads configuration from the environment, opens the two
// listeners, and wires them to a relaybox.Box.
//
// Environment variables:
//
//	RELAYBOX_SECRET      required. The registration secret a Mac must
//	                     present on the Mac door to become (or take back)
//	                     the control line. The box refuses to start
//	                     without one, so it can never come up wide open.
//	RELAYBOX_CERT_PATH   optional, default "/data/relaybox/box.pem" (the
//	                     Fly volume mount). Where the box's own TLS
//	                     identity is persisted across restarts — see
//	                     relaybox.LoadOrCreateCertificate. Deleting this
//	                     file rotates the box's identity and invalidates
//	                     every Mac's existing pin.
//	RELAYBOX_PHONE_ADDR  optional, default ":8443". Listen address for the
//	                     phone door. Plain TCP: the box never terminates
//	                     TLS here, so the phone's sealed connection to the
//	                     Mac passes through untouched.
//	RELAYBOX_MAC_ADDR    optional, default ":9000". Listen address for the
//	                     Mac door. TLS, terminated by the box itself using
//	                     the persisted certificate.
//	RELAYBOX_PHONE_PROXY_PROTOCOL optional, default false. Set to "true" on
//	                     Fly when its non-decrypting proxy_proto handler is
//	                     enabled, so phone rate limits use the real client IP.
//
// Fly (or any host) must forward both RELAYBOX_PHONE_ADDR and
// RELAYBOX_MAC_ADDR as raw TCP: this binary terminates TLS on neither port
// at a proxy layer, only inside relaybox.ServeMacDoor.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/relaybox"
)

const (
	defaultCertPath  = "/data/relaybox/box.pem"
	defaultPhoneAddr = ":8443"
	defaultMacAddr   = ":9000"
)

// Config is the box binary's configuration, gathered from the environment
// in main and passed to run. Keeping it a plain struct (rather than reading
// env vars inside run) is what lets run be unit-tested with arbitrary
// values and no real environment.
type Config struct {
	Secret             string
	CertPath           string
	PhoneListenAddr    string
	MacListenAddr      string
	PhoneProxyProtocol bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := configFromEnv()
	if err := validateConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Listeners are created here, after validation, so a misconfigured box
	// (missing secret) never binds a port at all.
	macListener, err := net.Listen("tcp", cfg.MacListenAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "relaybox-cmd: listen on mac door %s: %v\n", cfg.MacListenAddr, err)
		os.Exit(1)
	}
	phoneListener, err := net.Listen("tcp", cfg.PhoneListenAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "relaybox-cmd: listen on phone door %s: %v\n", cfg.PhoneListenAddr, err)
		_ = macListener.Close()
		os.Exit(1)
	}
	if cfg.PhoneProxyProtocol {
		phoneListener = relaybox.NewProxyProtocolListener(phoneListener, 2*time.Second)
	}

	if err := run(ctx, cfg, macListener, phoneListener, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// configFromEnv reads Config from the environment variables documented in
// the package comment above.
func configFromEnv() Config {
	return Config{
		Secret:             os.Getenv("RELAYBOX_SECRET"),
		CertPath:           envOrDefault("RELAYBOX_CERT_PATH", defaultCertPath),
		PhoneListenAddr:    envOrDefault("RELAYBOX_PHONE_ADDR", defaultPhoneAddr),
		MacListenAddr:      envOrDefault("RELAYBOX_MAC_ADDR", defaultMacAddr),
		PhoneProxyProtocol: os.Getenv("RELAYBOX_PHONE_PROXY_PROTOCOL") == "true",
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// validateConfig is the box's one hard rule: it must never run without a
// registration secret, because that secret is the only thing standing
// between "my Mac" and "anyone's Mac" claiming the control line. It also
// requires a certificate path, since the box has nowhere to keep (or find)
// its identity without one.
func validateConfig(cfg Config) error {
	if cfg.Secret == "" {
		return errors.New("relaybox-cmd: RELAYBOX_SECRET is required")
	}
	if cfg.CertPath == "" {
		return errors.New("relaybox-cmd: RELAYBOX_CERT_PATH is required")
	}
	return nil
}

// run wires up and serves a Box on already-created listeners. Splitting
// listener creation out of run (main does it) is what makes run testable
// with real loopback listeners bound to port 0, and what lets validation
// fail before any socket is touched.
func run(ctx context.Context, cfg Config, macListener, phoneListener net.Listener, out io.Writer) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}

	logger := slog.Default()
	logger.Info("[relaybox-cmd] starting",
		"mac_addr", cfg.MacListenAddr,
		"phone_addr", cfg.PhoneListenAddr,
		"cert_path", cfg.CertPath,
		"phone_proxy_protocol", cfg.PhoneProxyProtocol,
	)

	cert, err := relaybox.LoadOrCreateCertificate(cfg.CertPath, rand.Reader, time.Now())
	if err != nil {
		logger.Error("[relaybox-cmd] certificate unavailable", "error", err)
		return fmt.Errorf("relaybox-cmd: load certificate: %w", err)
	}
	fingerprint := relaybox.PinnedFingerprint(cert)
	pinnedKey := relaybox.PinnedPublicKeyBase64(cert)
	logger.Info("[relaybox-cmd] certificate ready", "fingerprint", fingerprint)
	printBanner(out, fingerprint, pinnedKey, cfg)

	box, err := relaybox.New(cfg.Secret, cert, relaybox.WithLogger(logger))
	if err != nil {
		logger.Error("[relaybox-cmd] box unavailable", "error", err)
		return fmt.Errorf("relaybox-cmd: create box: %w", err)
	}

	if err := serveDoors(ctx, box, macListener, phoneListener, logger); err != nil {
		return err
	}
	logger.Info("[relaybox-cmd] shutdown complete")
	return nil
}

// printBanner tells the operator, once, what to paste into the Mac's
// config. relayclient.Dial (the Mac side) pins on the raw public key bytes,
// not a hash, so pinnedKey — not fingerprint — is the value that belongs in
// the Mac's config; fingerprint is printed only as a short human-checkable
// label. Neither line may ever print the secret or any private key
// material — the whole point of pinning by public key is that both lines
// are safe to paste into a terminal, a chat message, or a screenshot.
func printBanner(out io.Writer, fingerprint, pinnedKey string, cfg Config) {
	fmt.Fprintln(out, "[relaybox-cmd] relay box starting")
	fmt.Fprintf(out, "[relaybox-cmd] PINNED KEY (put this in the Mac config): %s\n", pinnedKey)
	fmt.Fprintf(out, "[relaybox-cmd] fingerprint (for eyeball verification): %s\n", fingerprint)
	fmt.Fprintf(out, "[relaybox-cmd] mac door listening on %s\n", cfg.MacListenAddr)
	fmt.Fprintf(out, "[relaybox-cmd] phone door listening on %s\n", cfg.PhoneListenAddr)
}

// doorResult names which door a ServeXDoor call belonged to, so shutdown
// logging and error wrapping can say which half of the box stopped.
type doorResult struct {
	name string
	err  error
}

// serveDoors runs the Mac door and the phone door concurrently and returns
// the first real error either one produces. If one door fails for a real
// reason (its listener broke), the other is stopped too — a box that can
// only do half its job is not worth keeping half-alive. If ctx is
// cancelled from outside (graceful shutdown), both doors return nil and so
// does serveDoors.
func serveDoors(ctx context.Context, box *relaybox.Box, macListener, phoneListener net.Listener, logger *slog.Logger) error {
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan doorResult, 2)
	go func() { results <- doorResult{"mac", box.ServeMacDoor(serveCtx, macListener)} }()
	go func() { results <- doorResult{"phone", box.ServePhoneDoor(serveCtx, phoneListener)} }()

	var firstErr error
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			logger.Error("[relaybox-cmd] door stopped", "door", result.name, "error", result.err)
			if firstErr == nil {
				firstErr = fmt.Errorf("relaybox-cmd: %s door: %w", result.name, result.err)
				cancel()
			}
			continue
		}
		logger.Info("[relaybox-cmd] door shut down cleanly", "door", result.name)
	}
	return firstErr
}
