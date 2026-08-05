package recipient_test

import (
	"errors"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/recipient"
)

func TestAmbiguousRefusedBeforePreview(t *testing.T) {
	_, err := recipient.ExactOne("maya", []string{"maya", "maya"})
	if !errors.Is(err, recipient.ErrAmbiguous) {
		t.Fatalf("%v", err)
	}
}

func TestExactOneOk(t *testing.T) {
	got, err := recipient.ExactOne("raina", []string{"wife", "raina", "sinchana"})
	if err != nil || got != "raina" {
		t.Fatalf("%q %v", got, err)
	}
}
