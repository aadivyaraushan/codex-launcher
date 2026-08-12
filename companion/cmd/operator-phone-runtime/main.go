package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/localtrust"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

type runtimeCLI struct {
	Root                string
	Listen              string
	Name                string
	GatewayURL          string
	GatewayTokenPath    string
	AllowSoftwareAttest bool
	BeeperBaseURL       string
}

func envFlagOn(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseRuntimeCLI(args []string, output io.Writer) (runtimeCLI, error) {
	fs := flag.NewFlagSet("operator-phone-runtime", flag.ContinueOnError)
	if output != nil {
		fs.SetOutput(output)
	}
	root := fs.String("root", "", "no-backup state root for phone-runtime identities and session store")
	listen := fs.String("listen", phoneruntime.ListenAddress, "fixed loopback listen address")
	name := fs.String("name", "Operator phone", "display name shown in the mobile session")
	gatewayURL := fs.String("gateway-url", "", "OpenClaw gateway websocket URL; requires -gateway-token-path (path only, never the token value)")
	gatewayTokenPath := fs.String("gateway-token-path", "", "path to the OpenClaw gateway bearer token file (never the token itself)")
	allowSoftware := fs.Bool("allow-software-attest", false, "AVD-only: accept software Keystore attestation and emulator cert chains; never enable on release Pixel builds")
	beeperBaseURL := fs.String("beeper-base-url", "", "Beeper Client API base URL to watch for inbound messages (e.g. http://127.0.0.1:23373); empty disables the watcher. Token is never a flag — it comes from BEEPER_ACCESS_TOKEN or the local Beeper account database")
	if err := fs.Parse(args); err != nil {
		return runtimeCLI{}, err
	}
	cli := runtimeCLI{
		Root:                *root,
		Listen:              *listen,
		Name:                *name,
		GatewayURL:          strings.TrimSpace(*gatewayURL),
		GatewayTokenPath:    strings.TrimSpace(*gatewayTokenPath),
		AllowSoftwareAttest: *allowSoftware,
		BeeperBaseURL:       strings.TrimSpace(*beeperBaseURL),
	}
	if envFlagOn(os.Getenv("OPERATOR_ALLOW_SOFTWARE_ATTEST")) {
		cli.AllowSoftwareAttest = true
	}
	return cli, nil
}

func run(args []string) int {
	if len(args) > 0 && args[0] == "pair-android" {
		return runPairAndroid(args[1:])
	}
	cli, err := parseRuntimeCLI(args, os.Stderr)
	if err != nil {
		return 2
	}
	if cli.Root == "" {
		fmt.Fprintln(os.Stderr, "operator-phone-runtime: -root is required")
		return 2
	}
	absRoot, err := filepath.Abs(cli.Root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "operator-phone-runtime: resolve root: %v\n", err)
		return 1
	}
	tokenPath := cli.GatewayTokenPath
	if tokenPath != "" {
		tokenPath, err = filepath.Abs(tokenPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "operator-phone-runtime: resolve gateway token path: %v\n", err)
			return 1
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	runtime, err := phoneruntime.Open(ctx, phoneruntime.Config{
		Root:                absRoot,
		DisplayName:         cli.Name,
		ListenAddress:       cli.Listen,
		GatewayURL:          cli.GatewayURL,
		GatewayTokenPath:    tokenPath,
		AllowSoftwareAttest: cli.AllowSoftwareAttest,
		BeeperBaseURL:       cli.BeeperBaseURL,
	}, phoneruntime.Dependencies{Random: rand.Reader, Logger: logger})
	if err != nil {
		fmt.Fprintf(os.Stderr, "operator-phone-runtime: open: %v\n", err)
		return 1
	}
	defer runtime.Close()

	logger.Info("[phone-runtime] serve starting", "mode", runtime.Health().Mode, "process", runtime.Health().Process, "listen", runtime.Health().ListenAddress)
	if err := runtime.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "operator-phone-runtime: serve: %v\n", err)
		return 1
	}
	return 0
}

func runPairAndroid(args []string) int {
	fs := flag.NewFlagSet("pair-android", flag.ContinueOnError)
	out := fs.String("out", "", "path for the public offer JSON (0600); secret never written")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *out == "" {
		fmt.Fprintln(os.Stderr, "pair-android: -out is required")
		return 2
	}
	offer, err := localtrust.NewOffer(localtrust.OfferParams{Port: 9443, ExpiresIn: 10 * time.Minute, Now: time.Now().UTC()})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pair-android: create offer: %v\n", err)
		return 1
	}
	raw, err := json.MarshalIndent(offer.Public, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pair-android: encode: %v\n", err)
		return 1
	}
	if err := os.WriteFile(*out, raw, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "pair-android: write: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "pair-android: wrote public offer id=%s mime=%s (secret retained in memory only for this process)\n", offer.Public.OfferID, localtrust.MimeType)
	_ = offer.Secret
	return 0
}
