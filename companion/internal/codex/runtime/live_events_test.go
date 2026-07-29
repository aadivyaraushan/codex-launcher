package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
)

func TestProjectAppServerEventsProjectsFixedContentFreeUpdatesWithoutPrePublicationDeduplication(t *testing.T) {
	input := make(chan appserver.Notification, 4)
	input <- appserver.Notification{Method: "item/agentMessage/delta", Params: json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","delta":"first private fragment"}`)}
	input <- appserver.Notification{Method: "item/agentMessage/delta", Params: json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","delta":"second private fragment"}`)}
	input <- appserver.Notification{Method: "account/updated", Params: json.RawMessage(`{"private":"account content"}`)}
	input <- appserver.Notification{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}`)}
	close(input)
	output := make(chan taskstate.MobileEvent, 4)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	if err := projectAppServerEvents(context.Background(), input, output, logger); err != nil {
		t.Fatal(err)
	}
	var got []taskstate.MobileEvent
	for event := range output {
		got = append(got, event)
	}
	if len(got) != 3 || got[0].Summary != "Writing a reply" || got[1].Summary != "Writing a reply" || got[2].Summary != "Codex replied" {
		t.Fatalf("projected events = %#v", got)
	}
	for _, secret := range []string{"first private fragment", "second private fragment", "account content"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("logs exposed %q: %s", secret, logs.String())
		}
	}
}

func TestProjectAppServerEventsFailsClosedOnInvalidSupportedNotification(t *testing.T) {
	input := make(chan appserver.Notification, 1)
	input <- appserver.Notification{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread-1","turn":{"status":"completed"}}`)}
	close(input)
	output := make(chan taskstate.MobileEvent, 1)

	if err := projectAppServerEvents(context.Background(), input, output, nil); err == nil {
		t.Fatal("invalid supported notification was accepted")
	}
	if _, open := <-output; open {
		t.Fatal("projected event channel remained open after failure")
	}
}

func TestProjectAppServerEventsKeepsDrainingWhileMobileConsumerIsBlocked(t *testing.T) {
	input := make(chan appserver.Notification)
	output := make(chan taskstate.MobileEvent)
	result := make(chan error, 1)
	go func() { result <- projectAppServerEvents(context.Background(), input, output, nil) }()
	sentAll := make(chan struct{})
	go func() {
		for index := 0; index < 256; index++ {
			input <- appserver.Notification{Method: "item/agentMessage/delta", Params: json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","delta":"private"}`)}
		}
		close(input)
		close(sentAll)
	}()

	select {
	case <-sentAll:
	case <-time.After(time.Second):
		t.Fatal("projector stopped draining while mobile output was blocked")
	}
	var events []taskstate.MobileEvent
	for event := range output {
		events = append(events, event)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].TaskID != "thread-1" || events[0].Summary != "Writing a reply" {
		t.Fatalf("coalesced events = %#v", events)
	}
}

func TestProjectAppServerEventsBoundsBlockedOutputToRecentTaskCapacity(t *testing.T) {
	input := make(chan appserver.Notification)
	output := make(chan taskstate.MobileEvent)
	result := make(chan error, 1)
	go func() { result <- projectAppServerEvents(context.Background(), input, output, nil) }()
	sentAll := make(chan struct{})
	go func() {
		for index := 0; index < taskadapter.MaxRecentCatalogTasks+5; index++ {
			params := fmt.Sprintf(`{"threadId":"thread-%d","turn":{"id":"turn-%d","status":"inProgress","items":[]}}`, index, index)
			input <- appserver.Notification{Method: "turn/started", Params: json.RawMessage(params)}
		}
		close(input)
		close(sentAll)
	}()

	select {
	case <-sentAll:
	case <-time.After(time.Second):
		t.Fatal("projector stopped draining after its recent-task capacity")
	}
	var events []taskstate.MobileEvent
	for event := range output {
		events = append(events, event)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if len(events) != taskadapter.MaxRecentCatalogTasks || events[len(events)-1].TaskID != fmt.Sprintf("thread-%d", taskadapter.MaxRecentCatalogTasks+4) {
		t.Fatalf("bounded events = %#v", events)
	}
}
