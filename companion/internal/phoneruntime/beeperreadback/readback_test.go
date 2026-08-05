package beeperreadback_test

import (
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/beeperreadback"
)

func TestAwaitingWhenNoEvent(t *testing.T) {
	d, err := beeperreadback.Decide("p1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != beeperreadback.StatusPending {
		t.Fatalf("%+v", d)
	}
}

func TestSuccessRequiresSenderSuccessAndMatch(t *testing.T) {
	d, err := beeperreadback.Decide("p1", &beeperreadback.Event{
		PendingMessageID: "p1", ChatID: "c1", Text: "hi", IsSender: true, SendStatus: "SUCCESS",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != beeperreadback.StatusSuccess {
		t.Fatalf("%+v", d)
	}
}

func TestMismatchStaysPending(t *testing.T) {
	d, err := beeperreadback.Decide("p1", &beeperreadback.Event{
		PendingMessageID: "other", ChatID: "c1", Text: "hi", IsSender: true, SendStatus: "SUCCESS",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != beeperreadback.StatusPending || d.Reason != "pending_id_mismatch" {
		t.Fatalf("%+v", d)
	}
}
