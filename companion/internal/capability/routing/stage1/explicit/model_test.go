package explicit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
)

func testModel(rules ...Rule) *Model {
	return New(rules, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestAnExplicitYouTubePlayRequestRoutesWithoutACloudKey(t *testing.T) {
	model := testModel(Rule{ID: "youtube", Name: "YouTube", AppClass: "media", Verbs: []manifest.Verb{manifest.Play, manifest.Read}})

	raw, err := model.Route(context.Background(), "Play Never Gonna Give You Up official video on YouTube")
	if err != nil {
		t.Fatal(err)
	}
	route, err := stage1.ParseRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if route.Verb != manifest.Play || route.AppClass != "media" || route.AppNamed != "youtube" {
		t.Fatalf("route = %+v", route)
	}
	if route.Subject != "Never Gonna Give You Up official video" || route.Confidence < 0.9 {
		t.Fatalf("subject/confidence = %q/%v", route.Subject, route.Confidence)
	}
}

func TestWordsInAYouTubeTitleDoNotBecomeExtraCommands(t *testing.T) {
	model := testModel(Rule{ID: "youtube", Name: "YouTube", AppClass: "media", Verbs: []manifest.Verb{manifest.Play, manifest.Read}})

	for _, title := range []string{"Read My Mind", "Show Me How", "Watch Me"} {
		t.Run(title, func(t *testing.T) {
			raw, err := model.Route(context.Background(), "Play "+title+" on YouTube")
			if err != nil {
				t.Fatal(err)
			}
			route, err := stage1.ParseRoute(raw)
			if err != nil {
				t.Fatal(err)
			}
			if route.Verb != manifest.Play || route.Subject != title {
				t.Fatalf("route = %+v", route)
			}
		})
	}
}

func TestSeparateConflictingCommandsAreStillRefused(t *testing.T) {
	model := testModel(Rule{ID: "youtube", Name: "YouTube", AppClass: "media", Verbs: []manifest.Verb{manifest.Play, manifest.Read}})

	if _, err := model.Route(context.Background(), "Search and then play Never Gonna Give You Up on YouTube"); !errors.Is(err, ErrVerbUnclear) {
		t.Fatalf("conflicting command error = %v", err)
	}
}

func TestTheLongestExplicitAppNameWins(t *testing.T) {
	model := testModel(
		Rule{ID: "youtube", Name: "YouTube", AppClass: "media", Verbs: []manifest.Verb{manifest.Play}},
		Rule{ID: "youtubemusic", Name: "YouTube Music", AppClass: "media", Verbs: []manifest.Verb{manifest.Play}},
	)

	raw, err := model.Route(context.Background(), "Play lofi on YouTube Music")
	if err != nil {
		t.Fatal(err)
	}
	route, err := stage1.ParseRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if route.AppNamed != "youtubemusic" || route.Subject != "lofi" {
		t.Fatalf("route = %+v", route)
	}
}

func TestTheLocalRouteRefusesToGuess(t *testing.T) {
	model := testModel(Rule{ID: "youtube", Name: "YouTube", AppClass: "media", Verbs: []manifest.Verb{manifest.Play, manifest.Read}})

	if _, err := model.Route(context.Background(), "Play some music"); !errors.Is(err, ErrNoExplicitApp) {
		t.Fatalf("missing app error = %v", err)
	}
	if _, err := model.Route(context.Background(), "Open YouTube"); !errors.Is(err, ErrVerbUnclear) {
		t.Fatalf("unclear verb error = %v", err)
	}
}

// A Beeper-taken-over app is registered under beeper_messaging with verb
// Send, but the words a person uses ("send", "message") are the same ones a
// deep-link compose app hears. The matcher must read those words as the Send
// this app actually does, not refuse them because its single alias points at
// Compose. This is the routing half of the Beeper-orphaning fix.
func TestMessagingWordsRouteToSendWhenTheAppOnlyDoesSend(t *testing.T) {
	model := testModel(Rule{ID: "discord", Name: "Discord", AppClass: "beeper_messaging", Verbs: []manifest.Verb{manifest.Send}})

	raw, err := model.Route(context.Background(), "send a message on Discord")
	if err != nil {
		t.Fatal(err)
	}
	route, err := stage1.ParseRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if route.Verb != manifest.Send || route.AppClass != "beeper_messaging" || route.AppNamed != "discord" {
		t.Fatalf("route = %+v", route)
	}
}

// The same words on a deep-link compose app still resolve to Compose. The
// concrete verb comes from what the app can do, not from the keyword alone.
func TestMessagingWordsStillRouteToComposeWhenTheAppComposes(t *testing.T) {
	model := testModel(Rule{ID: "whatsapp", Name: "WhatsApp", AppClass: "messaging", Verbs: []manifest.Verb{manifest.Compose}})

	raw, err := model.Route(context.Background(), "send a message on WhatsApp")
	if err != nil {
		t.Fatal(err)
	}
	route, err := stage1.ParseRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if route.Verb != manifest.Compose {
		t.Fatalf("route = %+v", route)
	}
}

// A title word that names an action this app cannot do stays part of the
// subject, exactly as before. Reading "send"/"post" in a song title as a new
// command would be the regression the verb-group change must not introduce.
func TestATitleWordNamingAnUnsupportedActionStaysInTheSubject(t *testing.T) {
	model := testModel(Rule{ID: "youtube", Name: "YouTube", AppClass: "media", Verbs: []manifest.Verb{manifest.Play, manifest.Read}})

	raw, err := model.Route(context.Background(), "Play Send My Love on YouTube")
	if err != nil {
		t.Fatal(err)
	}
	route, err := stage1.ParseRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if route.Verb != manifest.Play || route.Subject != "Send My Love" {
		t.Fatalf("route = %+v", route)
	}
}

func TestASingleVerbAppNeedsNoVerbGuess(t *testing.T) {
	model := testModel(Rule{ID: "discord", Name: "Discord", AppClass: "messaging", Verbs: []manifest.Verb{manifest.Compose}})

	raw, err := model.Route(context.Background(), "Discord hello team")
	if err != nil {
		t.Fatal(err)
	}
	route, err := stage1.ParseRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if route.Verb != manifest.Compose || route.Subject != "hello team" {
		t.Fatalf("route = %+v", route)
	}
}
