package phoneruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
)

func TestDeferredTurnSourceAdvertisesTranscripts(t *testing.T) {
	var source mobilesession.TaskSource = newDeferredTurnSource()
	if _, ok := source.(mobilesession.TaskTranscriptSource); !ok {
		t.Fatal("deferredTurnSource must implement TaskTranscriptSource so welcome advertises task_transcripts")
	}
}

func TestDeferredTurnSourceReadTranscriptForwardsOnceConnected(t *testing.T) {
	deferred := newDeferredTurnSource()
	if _, err := deferred.ReadTranscript(context.Background(), "phone-agent", tasktranscript.PageOptions{TaskID: "phone-agent", Limit: 32}); !errors.Is(err, errTurnSourceNotConnected) {
		t.Fatalf("unconnected ReadTranscript = %v, want errTurnSourceNotConnected", err)
	}

	stub := newStubTurnSource()
	deferred.set(stub)
	page, err := deferred.ReadTranscript(context.Background(), "phone-agent", tasktranscript.PageOptions{TaskID: "phone-agent", Limit: 32})
	if err != nil {
		t.Fatalf("connected ReadTranscript: %v", err)
	}
	if page.TaskID != "phone-agent" || page.Entries == nil {
		t.Fatalf("forwarded page = %+v", page)
	}
}
