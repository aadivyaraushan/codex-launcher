package discovery

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// The Instagram canary is still broken after Fix 1, but for a different, more
// honest reason. Fix 1 removed the orphaning: the phone's keyword table no
// longer holds a stale messaging label. What is left is the keyword matcher
// grabbing the trailing word "messages" and routing to the Messages app
// instead of Instagram — a genuine misroute that only the LLM router
// (Workstream A) can fix, not a stage2 dead end. The canary must stay
// non-PASS, but as an executed MISROUTE, not an orphan_or_unnamed ask.
func TestInstagramCanaryIsAKeywordMisrouteAgainstTheRealPhoneRouter(t *testing.T) {
	sweep, err := NewPhoneSweep(discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	cases := []Case{{
		Utterance:     "what's my most recent unread Instagram messages",
		Beeper:        "on",
		ExpectedApp:   "instagram",
		ExpectedClass: "beeper_messaging",
		ExpectedVerb:  "read",
		Note:          "canary: keyword matcher grabs \"messages\"",
	}}
	results, err := sweep.Run(context.Background(), cases)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Verdict != MISROUTE {
		t.Fatalf("canary verdict = %s, want MISROUTE: %+v", results[0].Verdict, results[0])
	}
	if results[0].RouteOK {
		t.Fatalf("canary RouteOK = true, want false (stage1 aimed at messages, not instagram): %+v", results[0])
	}
	if results[0].ActualApp != "messages" {
		t.Fatalf("canary ActualApp = %q, want %q (the grabbed keyword): %+v", results[0].ActualApp, "messages", results[0])
	}
}

// The Beeper-orphaning fix: with Beeper on, "send a message on Discord" must
// route into beeper_messaging/send and execute, not dead-end at
// "I don't have the app you named connected for this." Before Fix 1 the
// phone's rule still said messaging/compose and the resolver orphaned it.
func TestBeeperMovedAppRoutesToBeeperMessagingUnderBeeperOn(t *testing.T) {
	sweep, err := NewPhoneSweep(discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	cases := []Case{{
		Utterance:     "send a message on Discord",
		Beeper:        "on",
		ExpectedApp:   "discord",
		ExpectedClass: "beeper_messaging",
		ExpectedVerb:  "send",
	}}
	results, err := sweep.Run(context.Background(), cases)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Verdict != PASS {
		t.Fatalf("result = %+v, want PASS", results)
	}
	if !results[0].RouteOK {
		t.Fatalf("RouteOK = false, want true: %+v", results[0])
	}
}

// A person-named Beeper message reaches send too: beeper_messaging is
// resolved by the adapter searching its own chats, not against the phone's
// empty contact graph, so naming "Sarah" does not dead-end the way a
// deep-link messaging app would.
func TestPersonNamedBeeperMessageReachesSendUnderBeeperOn(t *testing.T) {
	sweep, err := NewPhoneSweep(discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	cases := []Case{{
		Utterance:     "message Sarah on Discord",
		Beeper:        "on",
		ExpectedApp:   "discord",
		ExpectedClass: "beeper_messaging",
		ExpectedVerb:  "send",
	}}
	results, err := sweep.Run(context.Background(), cases)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Verdict != PASS {
		t.Fatalf("result = %+v, want PASS", results)
	}
}

func TestKnownGoodYouTubeRouteIsPass(t *testing.T) {
	sweep, err := NewPhoneSweep(discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	cases := []Case{{
		Utterance:     "Play Never Gonna Give You Up official video YouTube",
		Beeper:        "off",
		ExpectedApp:   "youtube",
		ExpectedClass: "media",
		ExpectedVerb:  "play",
	}}
	results, err := sweep.Run(context.Background(), cases)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Verdict != PASS {
		t.Fatalf("result = %+v, want PASS", results)
	}
	if !results[0].RouteOK {
		t.Fatalf("RouteOK = false, want true: %+v", results[0])
	}
	if results[0].FailKind != "" {
		t.Fatalf("FailKind = %q, want empty on a PASS: %+v", results[0].FailKind, results[0])
	}
}

// Fix 2: a deep-link messaging app tops out at HandsOff — it can only open
// the app, never send from a resolved handle. So "text my mom on WhatsApp"
// with an empty contact graph opens WhatsApp with the subject as a hint
// instead of dead-ending on "I don't know how to reach my mom." The old
// empty_contacts dead-end for a hand-off surface is gone; open-the-app is the
// tier's honest ceiling and counts as a PASS.
func TestAHandsOffMessagingAppOpensInsteadOfDeadEndingOnEmptyContacts(t *testing.T) {
	sweep, err := NewPhoneSweep(discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	cases := []Case{{
		Utterance:     "text my mom on WhatsApp",
		Beeper:        "off",
		ExpectedApp:   "whatsapp",
		ExpectedClass: "messaging",
		ExpectedVerb:  "compose",
	}}
	results, err := sweep.Run(context.Background(), cases)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Verdict != PASS {
		t.Fatalf("verdict = %s, want PASS (hand-off opens the app): %+v", results[0].Verdict, results[0])
	}
	if !results[0].RouteOK {
		t.Fatalf("RouteOK = false, want true (stage1 aimed at whatsapp/messaging/compose): %+v", results[0])
	}
	if results[0].FailKind != "" {
		t.Fatalf("FailKind = %q, want empty on a hand-off PASS: %+v", results[0].FailKind, results[0])
	}
}

func TestKnownOmittedAppIsPassWhenNoRouteIsExpected(t *testing.T) {
	sweep, err := NewPhoneSweep(discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	cases := []Case{{
		Utterance: "Find the meeting notes in Slack",
		Beeper:    "both",
	}}
	results, err := sweep.Run(context.Background(), cases)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v", results)
	}
	for _, r := range results {
		if r.Verdict != PASS {
			t.Errorf("beeper=%s verdict = %s, want PASS (Slack has no rule in the phone's table): %+v", r.Beeper, r.Verdict, r)
		}
	}
}

func TestBothExpandsToOneResultPerBeeperState(t *testing.T) {
	router := func(_ context.Context, _ string) ([]byte, error) {
		return []byte(`{"verb":"play","app_class":"media","app_named":"youtube","subject":"x","body":"x","fields":{},"confidence":0.99}`), nil
	}
	sweep := Sweep{
		BeeperOn: Environment{
			Router: router,
			Resolve: func(_ context.Context, route stage1.Route) (stage2.Decision, error) {
				return stage2.Decision{AdapterID: "on-adapter", Verb: route.Verb}, nil
			},
			Manifest: func(string) (manifest.Manifest, error) { return manifest.Manifest{}, nil },
		},
		BeeperOff: Environment{
			Router: router,
			Resolve: func(_ context.Context, route stage1.Route) (stage2.Decision, error) {
				return stage2.Decision{AdapterID: "off-adapter", Verb: route.Verb}, nil
			},
			Manifest: func(string) (manifest.Manifest, error) { return manifest.Manifest{}, nil },
		},
	}
	results, err := sweep.Run(context.Background(), []Case{{Utterance: "x", Beeper: "both"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v", results)
	}
	got := map[string]string{}
	for _, r := range results {
		got[r.Beeper] = r.ActualApp
	}
	if got["on"] != "on-adapter" || got["off"] != "off-adapter" {
		t.Fatalf("got = %+v, want on-adapter/off-adapter", got)
	}
}

func TestInvalidBeeperValueIsRejected(t *testing.T) {
	sweep := Sweep{BeeperOff: Environment{Router: func(context.Context, string) ([]byte, error) { return nil, nil }}}
	_, err := sweep.Run(context.Background(), []Case{{Utterance: "x", Beeper: "sideways"}})
	if err == nil {
		t.Fatal("want an error for an unknown beeper value")
	}
}

func TestClassifyExecutedDetectsCapabilityGapWhenAdapterManifestLacksTheExpectedVerb(t *testing.T) {
	lookup := func(id string) (manifest.Manifest, error) {
		return manifest.Manifest{ID: id, Verbs: []manifest.Verb{manifest.Send}}, nil
	}
	c := Case{ExpectedApp: "instagram", ExpectedClass: "beeper_messaging", ExpectedVerb: "read"}
	verdict, reason := classifyExecuted(lookup, c, "instagram", "beeper_messaging", "send", "")
	if verdict != CAPABILITY_GAP {
		t.Fatalf("verdict = %s (%s), want CAPABILITY_GAP", verdict, reason)
	}
}

func TestClassifyExecutedIsMisrouteWhenTheAdapterSupportsTheExpectedVerbButDidNotGetIt(t *testing.T) {
	lookup := func(id string) (manifest.Manifest, error) {
		return manifest.Manifest{ID: id, Verbs: []manifest.Verb{manifest.Read, manifest.Send}}, nil
	}
	c := Case{ExpectedApp: "instagram", ExpectedClass: "beeper_messaging", ExpectedVerb: "read"}
	verdict, _ := classifyExecuted(lookup, c, "instagram", "beeper_messaging", "send", "")
	if verdict != MISROUTE {
		t.Fatalf("verdict = %s, want MISROUTE", verdict)
	}
}
