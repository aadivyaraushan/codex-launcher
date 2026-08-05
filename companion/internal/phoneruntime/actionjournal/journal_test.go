package actionjournal_test

import (
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/actionjournal"
)

func TestSameActionIdSameHashReturnsStoredState(t *testing.T) {
	j := actionjournal.NewMemory()
	rec, err := j.Confirm("act-1", "hash-a", "acct", "chat")
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != actionjournal.StateConfirmed {
		t.Fatalf("state=%s", rec.State)
	}
	again, err := j.Confirm("act-1", "hash-a", "acct", "chat")
	if err != nil {
		t.Fatal(err)
	}
	if again.State != actionjournal.StateConfirmed {
		t.Fatalf("replay state=%s", again.State)
	}
}

func TestSameActionIdDifferentHashRejected(t *testing.T) {
	j := actionjournal.NewMemory()
	if _, err := j.Confirm("act-1", "hash-a", "acct", "chat"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Confirm("act-1", "hash-b", "acct", "chat"); err == nil {
		t.Fatal("expected hash mismatch rejection")
	}
}

func TestDispatchingWithoutReceiptBecomesDeliveryUnknown(t *testing.T) {
	j := actionjournal.NewMemory()
	if _, err := j.Confirm("act-1", "hash-a", "acct", "chat"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.MarkDispatching("act-1", "hash-a"); err != nil {
		t.Fatal(err)
	}
	rec, err := j.CrashRecover("act-1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != actionjournal.StateDeliveryUnknown {
		t.Fatalf("state=%s want delivery_unknown", rec.State)
	}
}

func TestSubmittedWithPendingIdResumesPollingNotResend(t *testing.T) {
	j := actionjournal.NewMemory()
	if _, err := j.Confirm("act-1", "hash-a", "acct", "chat"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.MarkDispatching("act-1", "hash-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.MarkSubmitted("act-1", "hash-a", "pending-9"); err != nil {
		t.Fatal(err)
	}
	rec, err := j.CrashRecover("act-1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != actionjournal.StateSubmitted {
		t.Fatalf("state=%s", rec.State)
	}
	if rec.PendingMessageID != "pending-9" {
		t.Fatalf("pending=%s", rec.PendingMessageID)
	}
	if rec.ShouldResend {
		t.Fatal("must not resend after submit")
	}
}

func TestMarkObservedFromSubmitted(t *testing.T) {
	j := actionjournal.NewMemory()
	if _, err := j.Confirm("act-1", "hash-a", "acct", "chat"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.MarkDispatching("act-1", "hash-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.MarkSubmitted("act-1", "hash-a", "pending-9"); err != nil {
		t.Fatal(err)
	}
	rec, err := j.MarkObserved("act-1", "pending-9")
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != actionjournal.StateObserved {
		t.Fatalf("state=%s", rec.State)
	}
}
