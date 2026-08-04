package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
)

type recordingBeeperAPI struct {
	sends int
}

type recordingDisconnectStore struct {
	deleted []string
}

func (s *recordingDisconnectStore) Delete(_ context.Context, name string) error {
	s.deleted = append(s.deleted, name)
	return nil
}

func (a *recordingBeeperAPI) SearchChats(context.Context, string) ([]beeper.Chat, error) {
	return []beeper.Chat{{ID: "chat-1", Network: "Discord", Title: "Aadivya"}}, nil
}

func (a *recordingBeeperAPI) Accounts(context.Context) ([]beeper.Account, error) {
	return []beeper.Account{{ID: "google-account-live", Network: "Google Messages", Status: "connected"}}, nil
}

func (a *recordingBeeperAPI) StartChat(context.Context, string, string) (beeper.Chat, error) {
	return beeper.Chat{ID: "chat-1", Network: "Google Messages", Title: "wife"}, nil
}

func (a *recordingBeeperAPI) Send(context.Context, string, string) (beeper.Sent, error) {
	a.sends++
	return beeper.Sent{ChatID: "chat-1", PendingMessageID: "pending-1"}, nil
}

func TestBeeperProofRefusesToSendWithoutExplicitApproval(t *testing.T) {
	api := &recordingBeeperAPI{}
	err := proveBeeper(context.Background(), api, beeperProofConfig{
		Network: "Discord", Recipient: "Aadivya", Message: "Operator verification",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if !errors.Is(err, errBeeperProofApprovalRequired) {
		t.Fatalf("proveBeeper error = %v, want approval-required error", err)
	}
	if api.sends != 0 {
		t.Fatalf("send count = %d, want 0", api.sends)
	}
}

func TestBeeperProofSendsExactlyOnceAfterApprovalAndReportsPendingDelivery(t *testing.T) {
	api := &recordingBeeperAPI{}
	err := proveBeeper(context.Background(), api, beeperProofConfig{
		Network: "Discord", Recipient: "Aadivya", Message: "Operator verification", Approved: true,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var unknown *adapter.OutcomeUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("proveBeeper error = %v, want pending-delivery OutcomeUnknownError", err)
	}
	if api.sends != 1 {
		t.Fatalf("send count = %d, want 1", api.sends)
	}
}

func TestBeeperReconnectClearsOnlyTheSelectedNetworkMarker(t *testing.T) {
	store := &recordingDisconnectStore{}
	if err := clearBeeperDisconnect(t.Context(), store, "Discord"); err != nil {
		t.Fatalf("clearBeeperDisconnect: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "adapter_disconnected_discord" {
		t.Fatalf("deleted markers = %v", store.deleted)
	}
}
