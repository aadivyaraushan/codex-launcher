package stage1

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// ---- the split that keeps the contact graph out of the cloud ------------

func TestARouteHasNoFieldThatCouldHoldAResolvedHandle(t *testing.T) {
	// Stage 1 runs in the cloud. If the type it returns cannot carry a phone
	// number, an email or a thread id, then no prompt change and no model
	// mistake can put one there. This is the split, enforced structurally.
	forbidden := []string{"handle", "phone", "number", "email", "address", "thread", "contact", "recipient"}

	rt := reflect.TypeOf(Route{})
	for i := range rt.NumField() {
		name := strings.ToLower(rt.Field(i).Name)
		for _, bad := range forbidden {
			if strings.Contains(name, bad) {
				t.Errorf("Route has a field %q; stage 1 must never carry a resolved handle", rt.Field(i).Name)
			}
		}
	}
}

func TestAReplyCarryingAHandleIsRejectedRatherThanIgnored(t *testing.T) {
	// A model that helpfully resolves the person is a privacy failure, not a
	// convenience. Dropping the field quietly would hide it; erroring makes
	// it visible the first time it happens.
	replies := []string{
		`{"verb":"send","app_class":"messaging","subject":"Maya","body":"hi","confidence":0.9,"handle":"+15550000001"}`,
		`{"verb":"send","app_class":"messaging","subject":"Maya","body":"hi","confidence":0.9,"phone":"+15550000001"}`,
		`{"verb":"send","app_class":"messaging","subject":"Maya","body":"hi","confidence":0.9,"thread_id":"t_123"}`,
		`{"verb":"send","app_class":"messaging","subject":"Maya","body":"hi","confidence":0.9,"email":"maya@example.com"}`,
	}
	for _, reply := range replies {
		if _, err := ParseRoute([]byte(reply)); !errors.Is(err, ErrHandleInRoute) {
			t.Errorf("ParseRoute(%s) returned %v, want ErrHandleInRoute", reply, err)
		}
	}
}

func TestAnOrdinaryReplyParsesIntoARoute(t *testing.T) {
	raw := []byte(`{"verb":"send","app_class":"messaging","app_named":"whatsapp",
	  "subject":"Maya","body":"running ten minutes late","confidence":0.94}`)

	got, err := ParseRoute(raw)
	if err != nil {
		t.Fatalf("ParseRoute failed: %v", err)
	}
	if got.Verb != manifest.Send {
		t.Errorf("verb = %s, want send", got.Verb)
	}
	if got.AppClass != "messaging" || got.AppNamed != "whatsapp" {
		t.Errorf("app class/name = %q/%q", got.AppClass, got.AppNamed)
	}
	if got.Subject != "Maya" {
		t.Errorf("subject = %q, want the name exactly as the user said it", got.Subject)
	}
}

// ---- the verb set is closed here too ------------------------------------

func TestAReplyInventingAVerbIsRejected(t *testing.T) {
	raw := []byte(`{"verb":"pay","app_class":"payments","subject":"Maya","confidence":0.99}`)
	if _, err := ParseRoute(raw); err == nil {
		t.Fatal("stage 1 accepted the verb \"pay\"; money movement is deep-link only")
	}
}

func TestAReplyWithNoVerbIsRejected(t *testing.T) {
	raw := []byte(`{"app_class":"messaging","subject":"Maya","confidence":0.99}`)
	if _, err := ParseRoute(raw); err == nil {
		t.Fatal("stage 1 accepted a reply with no verb")
	}
}

// ---- a low-confidence route asks; it never executes ---------------------

func TestALowConfidenceRouteMustAsk(t *testing.T) {
	// Zero cases where a low-confidence route executes instead of asking is
	// a shipping target, so it is a property of the type rather than of any
	// one caller remembering to check.
	low := Route{Verb: manifest.Send, AppClass: "messaging", Subject: "Maya", Confidence: ConfidenceFloor - 0.01}
	if !low.MustAsk() {
		t.Fatalf("a route at confidence %.2f did not ask", low.Confidence)
	}

	high := Route{Verb: manifest.Send, AppClass: "messaging", Subject: "Maya", Confidence: ConfidenceFloor}
	if high.MustAsk() {
		t.Fatalf("a route at the floor (%.2f) asked anyway", high.Confidence)
	}
}

func TestARouteThatNamesNoAppClassMustAsk(t *testing.T) {
	r := Route{Verb: manifest.Send, Subject: "Maya", Confidence: 0.99}
	if !r.MustAsk() {
		t.Fatal("a route with no app class went ahead")
	}
}

// ---- the loop is cheap because the model call is injected ---------------

func TestRoutingRunsAgainstARecordedReplyWithNoNetwork(t *testing.T) {
	// Every routing test in this repo runs offline against recorded replies.
	// Changing the prompt is the only thing that costs a live call.
	called := ""
	r := New(func(_ context.Context, utterance string) ([]byte, error) {
		called = utterance
		return []byte(`{"verb":"send","app_class":"messaging","subject":"Maya","body":"hi","confidence":0.95}`), nil
	})

	got, err := r.Route(context.Background(), "text Maya hi")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if called != "text Maya hi" {
		t.Errorf("the utterance handed to the model was %q", called)
	}
	if got.Subject != "Maya" {
		t.Errorf("subject = %q", got.Subject)
	}
}

func TestAModelErrorIsARoutingErrorNotASilentDefault(t *testing.T) {
	boom := errors.New("model unreachable")
	r := New(func(context.Context, string) ([]byte, error) { return nil, boom })

	if _, err := r.Route(context.Background(), "text Maya hi"); !errors.Is(err, boom) {
		t.Fatalf("Route swallowed the model error: %v", err)
	}
}

func TestUnparseableModelOutputIsAnErrorNotAGuess(t *testing.T) {
	r := New(func(context.Context, string) ([]byte, error) { return []byte("I think you want to text Maya!"), nil })

	if _, err := r.Route(context.Background(), "text Maya hi"); err == nil {
		t.Fatal("Route accepted prose as a route")
	}
}

// ---- named slots ---------------------------------------------------------

// Some requests carry more than one thing: "directions from home to the
// airport" has two places, and Subject can only hold one. Fields is how the
// cloud half hands named slots to the on-device half.
func TestFieldsCarryNamedSlotsToTheOnDeviceHalf(t *testing.T) {
	route, err := ParseRoute([]byte(`{"verb":"read","app_class":"travel","app_named":"maps",
	  "subject":"directions to the airport","body":"",
	  "fields":{"origin":"Blue Bottle Coffee Oakland","destination":"SFO"},
	  "confidence":0.95}`))
	if err != nil {
		t.Fatalf("ParseRoute: %v", err)
	}
	if route.Fields["origin"] != "Blue Bottle Coffee Oakland" || route.Fields["destination"] != "SFO" {
		t.Fatalf("fields=%v, want both endpoints", route.Fields)
	}
}

func TestAFieldNamingAContactHandleIsRefusedLikeAnyOtherField(t *testing.T) {
	// The whole point of the two stages is that the cloud half never sees a
	// resolved handle. A free-form map would be the obvious way around that
	// rule, so the same screen has to apply inside it.
	for _, key := range []string{"recipient_phone", "email", "thread_id", "contact"} {
		raw := []byte(`{"verb":"send","app_class":"messaging","app_named":"Signal",
		  "subject":"hi","body":"","fields":{"` + key + `":"whatever"},"confidence":0.99}`)
		if _, err := ParseRoute(raw); !errors.Is(err, ErrHandleInRoute) {
			t.Errorf("field %q was accepted (err=%v), want ErrHandleInRoute", key, err)
		}
	}
}

func TestAReplyWithNoFieldsStillParses(t *testing.T) {
	route, err := ParseRoute([]byte(`{"verb":"read","app_class":"travel","app_named":"maps",
	  "subject":"Blue Bottle","body":"","confidence":0.95}`))
	if err != nil {
		t.Fatalf("ParseRoute: %v", err)
	}
	if len(route.Fields) != 0 {
		t.Fatalf("fields=%v, want none", route.Fields)
	}
}

// Strict mode makes the model send every slot every time, filling the unused
// ones with null. An adapter that asks "was this slot set?" would read a
// present-but-empty slot as set, so the empty ones have to be dropped here
// rather than in each adapter.
func TestParseRouteDropsSlotsTheModelLeftEmpty(t *testing.T) {
	reply := []byte(`{"verb":"read","app_class":"travel","app_named":"maps",
	  "subject":"directions","body":"",
	  "fields":{"origin":"Blue Bottle Coffee Oakland","destination":"SFO",
	            "list":null,"folder":"","navigate":"   "},
	  "confidence":0.9}`)

	route, err := ParseRoute(reply)
	if err != nil {
		t.Fatalf("ParseRoute: %v", err)
	}
	want := map[string]string{"origin": "Blue Bottle Coffee Oakland", "destination": "SFO"}
	if len(route.Fields) != len(want) {
		t.Fatalf("Fields=%v, want only the slots the model actually filled: %v", route.Fields, want)
	}
	for key, value := range want {
		if route.Fields[key] != value {
			t.Errorf("Fields[%q]=%q, want %q", key, route.Fields[key], value)
		}
	}
}
