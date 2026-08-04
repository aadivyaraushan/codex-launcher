package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/beepermessage"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

var errBeeperProofApprovalRequired = errors.New("beeper proof: preview shown; pass --approved only after the owner approves this exact recipient and text")

type beeperProofConfig struct {
	Network   string
	Recipient string
	Message   string
	Approved  bool
}

type beeperDisconnectStore interface {
	Delete(context.Context, string) error
}

func runBeeperReconnect(args []string) error {
	fs := flag.NewFlagSet("beeper-reconnect", flag.ContinueOnError)
	network := fs.String("network", "", "Beeper network to reconnect: Discord, Instagram, or Google Messages")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := clearBeeperDisconnect(context.Background(), credentialstore.NewKeychain(slog.Default()), *network); err != nil {
		return err
	}
	verdict("%s is allowed to reconnect on the next companion start", strings.TrimSpace(*network))
	return nil
}

func clearBeeperDisconnect(ctx context.Context, store beeperDisconnectStore, network string) error {
	spec, err := beeperSpecForNetwork(network)
	if err != nil {
		return err
	}
	if err := store.Delete(ctx, credentialstore.AdapterDisconnectName(spec.ID)); err != nil {
		return fmt.Errorf("beeper reconnect: clear %s disconnect: %w", spec.Network, err)
	}
	return nil
}

func beeperSpecForNetwork(network string) (beepermessage.Spec, error) {
	for _, candidate := range beepermessage.ProductionSpecs() {
		if strings.EqualFold(candidate.Network, strings.TrimSpace(network)) {
			return candidate, nil
		}
	}
	return beepermessage.Spec{}, fmt.Errorf("beeper proof: unsupported network %q; want Discord, Instagram, or Google Messages", network)
}

func runBeeper(args []string) error {
	fs := flag.NewFlagSet("beeper", flag.ContinueOnError)
	network := fs.String("network", "", "connected Beeper network: Discord, Instagram, or Google Messages")
	recipient := fs.String("recipient", "", "exact conversation title or an unambiguous part of it")
	message := fs.String("message", "", "exact message text")
	approved := fs.Bool("approved", false, "confirm the owner approved the previewed recipient and exact text")
	if err := fs.Parse(args); err != nil {
		return err
	}

	token := strings.TrimSpace(os.Getenv("BEEPER_ACCESS_TOKEN"))
	if token == "" {
		return errors.New("beeper proof: BEEPER_ACCESS_TOKEN is not set")
	}
	logger := slog.Default()
	logger.Info("[beeper-proof] starting", "network", *network, "recipient_length", len(*recipient), "message_length", len(*message), "approved", *approved)
	client := beeper.NewClient(os.Getenv("BEEPER_DESKTOP_BASE_URL"), beeper.StaticToken(token), nil, logger)
	return proveBeeper(context.Background(), client, beeperProofConfig{
		Network: *network, Recipient: *recipient, Message: *message, Approved: *approved,
	}, logger)
}

func proveBeeper(ctx context.Context, api beepermessage.API, config beeperProofConfig, logger *slog.Logger) error {
	spec, err := beeperSpecForNetwork(config.Network)
	if err != nil {
		return err
	}

	a := beepermessage.New(spec, api, logger)
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		return fmt.Errorf("beeper proof: register %s adapter: %w", spec.ID, err)
	}
	runner := execution.New(reg)

	step(1, "Resolve the approved conversation through Beeper")
	plan, err := runner.Resolve(ctx, adapter.Intent{
		AdapterID: spec.ID, Verb: manifest.Send,
		Subject: config.Recipient, Body: config.Message,
	})
	if err != nil {
		return fmt.Errorf("beeper proof: resolve conversation: %w", err)
	}

	step(2, "Preview the exact network, conversation, and text — nothing has sent yet")
	preview, err := runner.Preview(ctx, plan)
	if err != nil {
		return fmt.Errorf("beeper proof: preview: %w", err)
	}
	printPreview(preview.Preview)
	if !config.Approved {
		logger.Info("[beeper-proof] stopped before send", "network", spec.Network, "reason", "approval_missing")
		return errBeeperProofApprovalRequired
	}

	step(3, "Execute the preview-bound confirmation through Beeper")
	outcome, err := runner.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		return fmt.Errorf("beeper proof: execute: %w", err)
	}
	printOutcome(outcome)
	verdict("%s message accepted by Beeper after the exact preview was confirmed", spec.Network)
	return nil
}
