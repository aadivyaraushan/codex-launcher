package deeplink

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	deeplinkadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/deeplink"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

func TestDeepLinkFlowRegistersWave1MoneyAndFoodAdapters(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"compose","app_class":"money","app_named":"venmo","subject":"Maya","body":"$20 for dinner","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-1", "Draft a Venmo payment to Maya for dinner")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "venmo" || preview.Verb != manifest.Compose || preview.Fingerprint == "" {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "$20 for dinner") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-1", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Venmo" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"sent", "paid", "ordered"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
}

func TestDeepLinkFlowRoutesFoodOrderToStarbucks(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"order","app_class":"food","app_named":"starbucks","subject":"usual","body":"grande oat latte","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-2", "Order my usual at Starbucks")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "starbucks" || preview.Verb != manifest.Order {
		t.Fatalf("preview=%+v", preview)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-2", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Starbucks" {
		t.Fatalf("outcome=%+v", outcome)
	}
	if strings.Contains(strings.ToLower(outcome.Detail), "ordered") {
		t.Fatalf("outcome claims ordered: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRegistersEveryWave1Spec(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"compose","app_class":"money","app_named":"cashapp","subject":"Devansh","body":"$15 coffee","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
	// User ask: Wave1Specs 73 → 76 (claude/chatgpt/grok messaging compose).
	if got := len(deeplinkadapter.Wave1Specs()); got != 76 {
		t.Fatalf("Wave1Specs count = %d, want 76", got)
	}
	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-3", "Cash App draft")
	if err != nil {
		t.Fatalf("Prepare cashapp: %v", err)
	}
	if preview.AdapterID != "cashapp" {
		t.Fatalf("cashapp not registered/routed: %+v", preview)
	}
}

func TestDeepLinkFlowRoutesMediaPlayToSpotify(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"play","app_class":"media","app_named":"spotify","subject":"lofi beats","body":"play something calm while I work","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-4", "Play something calm on Spotify")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "spotify" || preview.Verb != manifest.Play {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "play something calm while I work") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-4", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Spotify" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"played", "playing", "started playback", "added to playlist"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesMediaPlayToAudible(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"play","app_class":"media","app_named":"audible","subject":"Project Hail Mary","body":"continue Project Hail Mary from where I left off","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-5", "Continue Project Hail Mary on Audible")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "audible" || preview.Verb != manifest.Play {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "continue Project Hail Mary from where I left off") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-5", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Audible" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"played", "playing", "started playback", "resumed"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesMediaPlayToAppleMusic(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"play","app_class":"media","app_named":"applemusic","subject":"lofi beats","body":"play something calm while I work","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-applemusic", "Play something calm on Apple Music")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "applemusic" || preview.Verb != manifest.Play {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "play something calm while I work") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-applemusic", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Apple Music" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"played", "playing", "started playback", "added to playlist"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesMessagingComposeToMessages(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"compose","app_class":"messaging","app_named":"messages","subject":"Maya","body":"Running ten minutes late","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-6", "Draft a text to Maya that I'm running late")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "messages" || preview.Verb != manifest.Compose {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "Running ten minutes late") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-6", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Messages" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"sent", "delivered", "message sent"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesMessagingComposeToDiscord(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"compose","app_class":"messaging","app_named":"discord","subject":"Maya","body":"Running ten minutes late","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-7", "Draft a Discord message to Maya that I'm running late")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "discord" || preview.Verb != manifest.Compose {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "Running ten minutes late") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-7", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Discord" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"sent", "delivered", "message sent"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesRidesReadToUber(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"rides","app_named":"uber","subject":"airport","body":"fare estimate to SFO around 6pm","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-uber", "What's an Uber to SFO around 6pm?")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "uber" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "fare estimate to SFO around 6pm") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-uber", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Uber" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"booked", "reserved", "ride requested", "ordered"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesFoodReadToUberEats(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"food","app_named":"ubereats","subject":"Thai","body":"nearby Thai restaurants under $20","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-ubereats", "Find Thai on Uber Eats")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "ubereats" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-ubereats", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Uber Eats" {
		t.Fatalf("outcome=%+v", outcome)
	}
	if strings.Contains(strings.ToLower(outcome.Detail), "ordered") {
		t.Fatalf("outcome claims ordered: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesFoodReadToResy(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"food","app_named":"resy","subject":"Friday 7pm","body":"table for 2 at a quiet Italian place","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-resy", "Check Resy for Friday Italian")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "resy" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-resy", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Resy" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"booked", "reserved", "ordered"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
}

func TestDeepLinkFlowRoutesFoodOrderToDoorDash(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"order","app_class":"food","app_named":"doordash","subject":"chipotle bowl","body":"chipotle bowl no rice for delivery","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-doordash", "Order a Chipotle bowl on DoorDash")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "doordash" || preview.Verb != manifest.Order {
		t.Fatalf("preview=%+v", preview)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-doordash", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "DoorDash" {
		t.Fatalf("outcome=%+v", outcome)
	}
	if strings.Contains(strings.ToLower(outcome.Detail), "ordered") {
		t.Fatalf("outcome claims ordered: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesMediaReadToGooglePhotos(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"media","app_named":"googlephotos","subject":"beach sunset","body":"find photos from last beach trip","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-googlephotos", "Find beach trip photos in Google Photos")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "googlephotos" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-googlephotos", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Google Photos" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"found", "uploaded", "saved", "completed"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
}

func TestDeepLinkFlowRoutesMessagingComposeToTeams(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"compose","app_class":"messaging","app_named":"teams","subject":"Maya","body":"Running ten minutes late","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-teams", "Draft a Teams message to Maya")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "teams" || preview.Verb != manifest.Compose {
		t.Fatalf("preview=%+v", preview)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-teams", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Teams" {
		t.Fatalf("outcome=%+v", outcome)
	}
	if strings.Contains(strings.ToLower(outcome.Detail), "sent") {
		t.Fatalf("outcome claims sent: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesTravelReadToBooking(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"travel","app_named":"booking","subject":"Lisbon","body":"hotels near Alfama under $150","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-booking", "Search hotels in Lisbon on Booking.com")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "booking" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-booking", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Booking.com" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"booked", "reserved", "purchased"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
}

func TestDeepLinkFlowRoutesTravelReadToWave1Connectors(t *testing.T) {
	cases := []struct {
		id, named, handed, subject, body, utterance string
	}{
		{"tripadvisor", "tripadvisor", "Tripadvisor", "Rome", "best gelato near Trastevere", "Find gelato near Trastevere on Tripadvisor"},
		{"viator", "viator", "Viator", "Paris", "Louvre skip-the-line tour tomorrow", "Find a Louvre tour on Viator"},
		{"stubhub", "stubhub", "StubHub", "Warriors", "Warriors tickets near me this weekend", "Find Warriors tickets on StubHub"},
		{"alltrails", "alltrails", "AllTrails", "Yosemite", "moderate hikes near Yosemite Valley", "Find moderate hikes on AllTrails"},
		{"googlemaps", "googlemaps", "Google Maps", "SFO", "directions to SFO Terminal 2", "Get directions to SFO on Google Maps"},
		{"united", "united", "United", "UA123", "check flight status for UA123 tomorrow", "Check United flight status for UA123"},
		{"delta", "delta", "Delta", "DL456", "open my Delta booking to manage seats", "Open Delta to manage my booking"},
		{"southwest", "southwest", "Southwest", "WN789", "check Southwest flight status for WN789", "Check Southwest flight status"},
		{"american", "american", "American Airlines", "AA100", "open American Airlines to manage booking AA100", "Open American Airlines to manage booking"},
		{"citymapper", "citymapper", "Citymapper", "home", "directions home on Citymapper", "Get directions home on Citymapper"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			payload := `{"verb":"read","app_class":"travel","app_named":"` + tc.named + `","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-"+tc.id, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.id || preview.Verb != manifest.Read {
				t.Fatalf("preview=%+v", preview)
			}

			outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-"+tc.id, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"booked", "reserved", "purchased", "navigated", "saved", "checked-in", "checked in", "boarded"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
		})
	}
}

func TestDeepLinkFlowRoutesGoogleMapsWriteToPrepareAndOpen(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"write","app_class":"travel","app_named":"googlemaps","subject":"home","body":"save home as a place in Maps","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-maps-write", "Save home in Google Maps")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "googlemaps" || preview.Verb != manifest.Write {
		t.Fatalf("preview=%+v", preview)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-maps-write", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Google Maps" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"navigated", "saved", "completed"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
}

func TestDeepLinkFlowRoutesServicesComposeToWave1HandOffs(t *testing.T) {
	cases := []struct {
		id, named, handed, subject, body, utterance string
	}{
		{"taskrabbit", "taskrabbit", "Taskrabbit", "shelf", "assemble IKEA bookshelf this Saturday", "Prepare a Taskrabbit request to assemble a bookshelf"},
		{"thumbtack", "thumbtack", "Thumbtack", "plumber", "find a plumber for a clogged sink", "Prepare a Thumbtack request for a plumber"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			payload := `{"verb":"compose","app_class":"services","app_named":"` + tc.named + `","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-"+tc.id, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.id || preview.Verb != manifest.Compose {
				t.Fatalf("preview=%+v", preview)
			}

			outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-"+tc.id, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"booked", "hired", "ordered", "sent"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
		})
	}
}

func TestDeepLinkFlowRoutesFinanceReadToWave1HandOffs(t *testing.T) {
	cases := []struct {
		id, named, handed, subject, body, utterance string
	}{
		{"creditkarma", "creditkarma", "Credit Karma", "score", "check my credit score and recent alerts", "Open Credit Karma to check my score"},
		{"turbotax", "turbotax", "TurboTax", "2025 return", "open my 2025 tax return draft", "Open TurboTax to continue my return"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			payload := `{"verb":"read","app_class":"finance","app_named":"` + tc.named + `","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-"+tc.id, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.id || preview.Verb != manifest.Read {
				t.Fatalf("preview=%+v", preview)
			}

			outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-"+tc.id, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"filed", "score retrieved", "retrieved", "completed"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
		})
	}
}

func TestDeepLinkFlowRoutesRidesReadToLyft(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"rides","app_named":"lyft","subject":"airport","body":"fare estimate to SFO around 6pm","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-lyft", "What's a Lyft to SFO around 6pm?")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "lyft" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "fare estimate to SFO around 6pm") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-lyft", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Lyft" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"booked", "reserved", "ride requested", "ordered"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesNotesWriteToGoogleKeep(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"write","app_class":"notes","app_named":"googlekeep","subject":"groceries","body":"milk eggs bread","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-googlekeep", "Draft a Keep note for groceries")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "googlekeep" || preview.Verb != manifest.Write {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "milk eggs bread") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-googlekeep", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Google Keep" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"saved", "created", "note saved", "wrote"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesMediaPlayAndWriteToNetflix(t *testing.T) {
	cases := []struct {
		verb                                  manifest.Verb
		subject, body, utterance, requestID string
	}{
		{manifest.Play, "Stranger Things", "open Stranger Things on Netflix", "Open Stranger Things on Netflix", "request-netflix-play"},
		{manifest.Write, "My List", "add The Crown to My List", "Add The Crown to My List on Netflix", "request-netflix-write"},
	}
	for _, tc := range cases {
		t.Run(string(tc.verb), func(t *testing.T) {
			payload := `{"verb":"` + string(tc.verb) + `","app_class":"media","app_named":"netflix","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			preview, err := service.Prepare(context.Background(), "pixel/session/1", tc.requestID, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != "netflix" || preview.Verb != tc.verb {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}

			outcome, err := service.Confirm(context.Background(), "pixel/session/1", tc.requestID, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Netflix" {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"played", "playing", "started playback", "added to my list", "added to list"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

func TestDeepLinkFlowRoutesMediaPlayAndReadToYouTube(t *testing.T) {
	// Callers: go test ./companion/internal/capability/runtime/deeplink/
	// Existing file; mirrors Netflix media flow test. No data files.
	// User: "YouTube prepare-and-open (Wave1Specs 38 → 39)" — never claim played.
	cases := []struct {
		verb                                  manifest.Verb
		subject, body, utterance, requestID string
	}{
		{manifest.Play, "lofi beats", "open lofi beats on YouTube", "Open lofi beats on YouTube", "request-youtube-play"},
		{manifest.Read, "search", "find cooking tutorials on YouTube", "Find cooking tutorials on YouTube", "request-youtube-read"},
	}
	for _, tc := range cases {
		t.Run(string(tc.verb), func(t *testing.T) {
			payload := `{"verb":"` + string(tc.verb) + `","app_class":"media","app_named":"youtube","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			preview, err := service.Prepare(context.Background(), "pixel/session/1", tc.requestID, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != "youtube" || preview.Verb != tc.verb {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}

			outcome, err := service.Confirm(context.Background(), "pixel/session/1", tc.requestID, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "YouTube" {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"played", "playing", "started playback", "watched"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

func TestDeepLinkFlowRoutesTravelReadToAirbnb(t *testing.T) {
	// Callers: go test ./companion/internal/capability/runtime/deeplink/
	// Mirrors Booking.com travel/read. Wave 3 book demoted → read; never claim booked.
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"travel","app_named":"airbnb","subject":"Lisbon","body":"apartments near Alfama under $150","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-airbnb", "Search apartments in Lisbon on Airbnb")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "airbnb" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-airbnb", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Airbnb" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"booked", "reserved", "purchased"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
}

func TestDeepLinkFlowRoutesFoodReadToOpenTable(t *testing.T) {
	// Callers: go test ./companion/internal/capability/runtime/deeplink/
	// Mirrors Resy food/read. Wave 3 book demoted → read; never claim reservation booked.
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"food","app_named":"opentable","subject":"Friday 7pm","body":"table for 2 at a quiet Italian place","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-opentable", "Check OpenTable for Friday Italian")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "opentable" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-opentable", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "OpenTable" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"booked", "reserved", "ordered"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
}

func TestDeepLinkFlowRoutesFoodReadAndOrderToGrubhub(t *testing.T) {
	// Callers: go test ./companion/internal/capability/runtime/deeplink/
	// Mirrors DoorDash food read|order. Never claim checkout completed.
	cases := []struct {
		verb                                  manifest.Verb
		subject, body, utterance, requestID string
	}{
		{manifest.Read, "burrito", "nearby burrito bowls for delivery", "Find burritos on Grubhub", "request-grubhub-read"},
		{manifest.Order, "chipotle bowl", "chipotle bowl no rice for delivery", "Order a Chipotle bowl on Grubhub", "request-grubhub-order"},
	}
	for _, tc := range cases {
		t.Run(string(tc.verb), func(t *testing.T) {
			payload := `{"verb":"` + string(tc.verb) + `","app_class":"food","app_named":"grubhub","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			preview, err := service.Prepare(context.Background(), "pixel/session/1", tc.requestID, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != "grubhub" || preview.Verb != tc.verb {
				t.Fatalf("preview=%+v", preview)
			}

			outcome, err := service.Confirm(context.Background(), "pixel/session/1", tc.requestID, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Grubhub" {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"ordered", "checkout completed", "purchased"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
		})
	}
}

func TestDeepLinkFlowRoutesMessagingComposeToFacebook(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"compose","app_class":"messaging","app_named":"facebook","subject":"friends","body":"draft a post that I'm heading out","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-facebook", "Draft a Facebook post that I'm heading out")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "facebook" || preview.Verb != manifest.Compose {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "draft a post that I'm heading out") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-facebook", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Facebook" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"posted", "published", "sent"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesMessagingComposeToThreadsAndTikTok(t *testing.T) {
	cases := []struct {
		id, named, handed, body, utterance string
	}{
		{"threads", "threads", "Threads", "draft a Threads post that I'm heading out", "Draft a Threads post that I'm heading out"},
		{"tiktok", "tiktok", "TikTok", "draft a TikTok caption that I'm heading out", "Draft a TikTok caption that I'm heading out"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			payload := `{"verb":"compose","app_class":"messaging","app_named":"` + tc.named + `","subject":"friends","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-"+tc.id, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.id || preview.Verb != manifest.Compose {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}

			outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-"+tc.id, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"posted", "replied", "answered", "published", "sent"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

func TestDeepLinkFlowRoutesTravelReadToExpedia(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"travel","app_named":"expedia","subject":"Lisbon","body":"hotels near Alfama under $150","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-expedia", "Search hotels in Lisbon on Expedia")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "expedia" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "hotels near Alfama under $150") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-expedia", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Expedia" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"booked", "reserved", "purchased"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

// Shopping prepare-and-open (Wave1Specs 45 → 50 shopping; +kayak → 51):
// Target/Walmart/Nike/Sephora/Wayfair browse/open. Verb read only — never claim
// cart built, ordered, or checkout completed.
func TestDeepLinkFlowRoutesShoppingReadToWave1HandOffs(t *testing.T) {
	cases := []struct {
		id, named, handed, subject, body, utterance string
	}{
		{"target", "target", "Target", "paper towels", "search paper towels nearby", "Open Target to browse paper towels"},
		{"walmart", "walmart", "Walmart", "groceries", "search milk and eggs", "Open Walmart to browse groceries"},
		{"nike", "nike", "Nike", "running shoes", "search Pegasus running shoes", "Open Nike to browse running shoes"},
		{"sephora", "sephora", "Sephora", "lipstick", "search lipstick nearby", "Open Sephora to browse lipstick"},
		{"wayfair", "wayfair", "Wayfair", "sofa", "search mid-century sofa", "Open Wayfair to browse sofas"},
		{"ebay", "ebay", "eBay", "camera", "browse used cameras under $200", "Open eBay to browse used cameras"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			payload := `{"verb":"read","app_class":"shopping","app_named":"` + tc.named + `","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-"+tc.id, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.id || preview.Verb != manifest.Read {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}

			outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-"+tc.id, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"cart built", "ordered", "checkout completed", "purchased", "bought", "bid placed"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

func TestDeepLinkFlowReadyLogIncludesShoppingAdapters(t *testing.T) {
	var buf strings.Builder
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"shopping","app_named":"target","subject":"towels","body":"browse paper towels","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(&buf, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = service
	logged := buf.String()
	if !strings.Contains(logged, "shopping_adapters") {
		t.Fatalf("ready log missing shopping_adapters: %s", logged)
	}
	if !strings.Contains(logged, "shopping_adapters=6") && !strings.Contains(logged, `"shopping_adapters":6`) {
		// text handler emits key=value
		if !strings.Contains(logged, "shopping_adapters=6") {
			t.Fatalf("ready log shopping_adapters count want 6: %s", logged)
		}
	}
	if !strings.Contains(logged, "travel_adapters=15") && !strings.Contains(logged, `"travel_adapters":15`) {
		if !strings.Contains(logged, "travel_adapters=15") {
			t.Fatalf("ready log travel_adapters count want 15: %s", logged)
		}
	}
	if !strings.Contains(logged, "messaging_adapters=14") && !strings.Contains(logged, `"messaging_adapters":14`) {
		if !strings.Contains(logged, "messaging_adapters=14") {
			t.Fatalf("ready log messaging_adapters count want 14: %s", logged)
		}
	}
	// Callers: Wave1Specs → runtime/deeplink ready log (this test).
	// User ask: Wave1Specs 73 → 76 — messaging_adapters=14 (+claude+chatgpt+grok); services/media/tasks/notes unchanged.
	if !strings.Contains(logged, "services_adapters=4") && !strings.Contains(logged, `"services_adapters":4`) {
		if !strings.Contains(logged, "services_adapters=4") {
			t.Fatalf("ready log services_adapters count want 4: %s", logged)
		}
	}
	if !strings.Contains(logged, "media_adapters=13") && !strings.Contains(logged, `"media_adapters":13`) {
		if !strings.Contains(logged, "media_adapters=13") {
			t.Fatalf("ready log media_adapters count want 13: %s", logged)
		}
	}
	if !strings.Contains(logged, "tasks_adapters=3") && !strings.Contains(logged, `"tasks_adapters":3`) {
		if !strings.Contains(logged, "tasks_adapters=3") {
			t.Fatalf("ready log tasks_adapters count want 3: %s", logged)
		}
	}
	if !strings.Contains(logged, "notes_adapters=7") && !strings.Contains(logged, `"notes_adapters":7`) {
		if !strings.Contains(logged, "notes_adapters=7") {
			t.Fatalf("ready log notes_adapters count want 7: %s", logged)
		}
	}
}

// Kayak prepare-and-open (Wave1Specs 50 → 51): travel read; book demoted → read.
func TestDeepLinkFlowRoutesKayakReadToTravelHandOff(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"travel","app_named":"kayak","subject":"Lisbon","body":"flights to Lisbon next weekend","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-kayak", "Search flights to Lisbon on Kayak")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "kayak" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "flights to Lisbon next weekend") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-kayak", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Kayak" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"booked", "reserved", "purchased", "ticketed"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

// Priceline prepare-and-open (Wave1Specs 51 → 54): travel read; book demoted → read.
func TestDeepLinkFlowRoutesPricelineReadToTravelHandOff(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"travel","app_named":"priceline","subject":"Miami","body":"hotels in Miami next weekend","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-priceline", "Search hotels in Miami on Priceline")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "priceline" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "hotels in Miami next weekend") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-priceline", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Priceline" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"booked", "reserved", "purchased", "ticketed"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

// LinkedIn prepare-and-open: messaging compose (Facebook/Threads peer); never claim posted/commented.
func TestDeepLinkFlowRoutesLinkedInComposeToMessagingHandOff(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"compose","app_class":"messaging","app_named":"linkedin","subject":"network","body":"draft a LinkedIn post that I'm open to work","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-linkedin", "Draft a LinkedIn post that I'm open to work")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "linkedin" || preview.Verb != manifest.Compose {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "draft a LinkedIn post that I'm open to work") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-linkedin", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "LinkedIn" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"posted", "commented", "published", "sent"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
// User ask: Wave1Specs 55 → 58 — Duolingo/Fitbit services read browse/open; never claim lesson/workout.
func TestDeepLinkFlowRoutesDuolingoFitbitReadToServicesHandOff(t *testing.T) {
	cases := []struct {
		id, named, handed, subject, body, utterance string
		bans                                       []string
	}{
		{"duolingo", "duolingo", "Duolingo", "Spanish", "open Spanish lesson A1 greetings", "Open Duolingo for Spanish A1 greetings", []string{"lesson completed", "workout logged", "synced", "saved"}},
		{"fitbit", "fitbit", "Fitbit", "today", "open today's activity summary", "Open Fitbit to today's activity", []string{"workout logged", "synced", "saved", "lesson completed"}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			payload := `{"verb":"read","app_class":"services","app_named":"` + tc.named + `","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-"+tc.id, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.id || preview.Verb != manifest.Read {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}
			outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-"+tc.id, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range tc.bans {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
// User ask: Wave1Specs 55 → 58 — Shazam media read identify/search intent; never claim identified/played/saved.
func TestDeepLinkFlowRoutesShazamReadToMediaHandOff(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"media","app_named":"shazam","subject":"song","body":"identify this song nearby","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-shazam", "Open Shazam to identify this song")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "shazam" || preview.Verb != manifest.Read {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "identify this song nearby") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-shazam", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Shazam" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"identified", "played", "saved", "playing"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
// User ask: Wave1Specs 58 → 61 — Chromecast media play cast/open intent; never claim cast started/playing/connected.
func TestDeepLinkFlowRoutesChromecastPlayToMediaHandOff(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"play","app_class":"media","app_named":"chromecast","subject":"living room","body":"open Chromecast to cast living room TV","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-chromecast", "Open Chromecast to cast living room TV")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "chromecast" || preview.Verb != manifest.Play {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "open Chromecast to cast living room TV") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-chromecast", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Chromecast" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"cast started", "playing", "connected", "played"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
// User ask: Wave1Specs 58 → 61 — YouTube Music / SoundCloud media play|read; never claim played/playlist/library.
func TestDeepLinkFlowRoutesYouTubeMusicAndSoundCloudPlayReadToMediaHandOff(t *testing.T) {
	cases := []struct {
		named, handed, subject, body, utterance, requestID string
		verb                                               manifest.Verb
	}{
		{"youtubemusic", "YouTube Music", "lofi beats", "open lofi beats on YouTube Music", "Open lofi beats on YouTube Music", "request-youtubemusic-play", manifest.Play},
		{"youtubemusic", "YouTube Music", "search", "find cooking playlists on YouTube Music", "Find cooking playlists on YouTube Music", "request-youtubemusic-read", manifest.Read},
		{"soundcloud", "SoundCloud", "lofi beats", "open lofi beats on SoundCloud", "Open lofi beats on SoundCloud", "request-soundcloud-play", manifest.Play},
		{"soundcloud", "SoundCloud", "search", "find chill mixes on SoundCloud", "Find chill mixes on SoundCloud", "request-soundcloud-read", manifest.Read},
	}
	for _, tc := range cases {
		t.Run(tc.named+"/"+string(tc.verb), func(t *testing.T) {
			payload := `{"verb":"` + string(tc.verb) + `","app_class":"media","app_named":"` + tc.named + `","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			preview, err := service.Prepare(context.Background(), "pixel/session/1", tc.requestID, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.named || preview.Verb != tc.verb {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}
			outcome, err := service.Confirm(context.Background(), "pixel/session/1", tc.requestID, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"played", "playing", "added to playlist", "library changed"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}


// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
// User ask: Wave1Specs 61 → 64 — Pandora media play|read; never claim played/station changed.
func TestDeepLinkFlowRoutesPandoraPlayReadToMediaHandOff(t *testing.T) {
	cases := []struct {
		subject, body, utterance, requestID string
		verb                                manifest.Verb
	}{
		{"lofi beats", "open lofi beats on Pandora", "Open lofi beats on Pandora", "request-pandora-play", manifest.Play},
		{"search", "find chill stations on Pandora", "Find chill stations on Pandora", "request-pandora-read", manifest.Read},
	}
	for _, tc := range cases {
		t.Run(string(tc.verb), func(t *testing.T) {
			payload := `{"verb":"` + string(tc.verb) + `","app_class":"media","app_named":"pandora","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			preview, err := service.Prepare(context.Background(), "pixel/session/1", tc.requestID, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != "pandora" || preview.Verb != tc.verb {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}
			outcome, err := service.Confirm(context.Background(), "pixel/session/1", tc.requestID, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Pandora" {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"played", "playing", "added to playlist", "station changed"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
// User ask: Wave1Specs 61 → 64 — Asana/Trello tasks write; never claim task created/card moved/assigned/completed.
// Not Todoist RT-2 — prepare-and-open only.
func TestDeepLinkFlowRoutesAsanaAndTrelloWriteToTasksHandOff(t *testing.T) {
	cases := []struct {
		named, handed, subject, body, utterance, requestID string
	}{
		{"asana", "Asana", "groceries", "draft an Asana task to buy oat milk", "Draft an Asana task to buy oat milk", "request-asana-write"},
		{"trello", "Trello", "launch", "draft a Trello card for the launch checklist", "Draft a Trello card for the launch checklist", "request-trello-write"},
	}
	for _, tc := range cases {
		t.Run(tc.named, func(t *testing.T) {
			payload := `{"verb":"write","app_class":"tasks","app_named":"` + tc.named + `","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			preview, err := service.Prepare(context.Background(), "pixel/session/1", tc.requestID, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.named || preview.Verb != manifest.Write {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}
			outcome, err := service.Confirm(context.Background(), "pixel/session/1", tc.requestID, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"task created", "card moved", "assigned", "completed"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
// User ask: Wave1Specs 64 → 67 — Microsoft To Do tasks write; never claim task created/assigned/completed.
// Not Todoist RT-2 — prepare-and-open only.
func TestDeepLinkFlowRoutesMSToDoWriteToTasksHandOff(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"write","app_class":"tasks","app_named":"mstodo","subject":"groceries","body":"draft a Microsoft To Do task to buy oat milk","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-mstodo-write", "Draft a Microsoft To Do task to buy oat milk")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "mstodo" || preview.Verb != manifest.Write {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "draft a Microsoft To Do task to buy oat milk") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-mstodo-write", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Microsoft To Do" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"task created", "assigned", "completed"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
// User ask: Wave1Specs 64 → 67 — Google Docs notes write + Dropbox notes read; never claim doc/file completion.
// Separate from Google Drive OAuth adapter.
func TestDeepLinkFlowRoutesGoogleDocsAndDropboxToNotesHandOff(t *testing.T) {
	cases := []struct {
		named, handed, subject, body, utterance, requestID string
		verb                                               manifest.Verb
		bans                                               []string
	}{
		{"googledocs", "Google Docs", "meeting notes", "draft a Google Doc for meeting notes", "Draft a Google Doc for meeting notes", "request-googledocs-write", manifest.Write, []string{"created", "saved", "shared"}},
		{"dropbox", "Dropbox", "receipts", "open receipts folder in Dropbox", "Open receipts folder in Dropbox", "request-dropbox-read", manifest.Read, []string{"uploaded", "downloaded", "shared", "synced"}},
	}
	for _, tc := range cases {
		t.Run(tc.named, func(t *testing.T) {
			payload := `{"verb":"` + string(tc.verb) + `","app_class":"notes","app_named":"` + tc.named + `","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			preview, err := service.Prepare(context.Background(), "pixel/session/1", tc.requestID, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.named || preview.Verb != tc.verb {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}
			outcome, err := service.Confirm(context.Background(), "pixel/session/1", tc.requestID, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range tc.bans {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
// User ask: Wave1Specs 67 → 70 — Google Sheets / Evernote / Google Slides notes write; never claim created/saved/shared/synced.
// Separate from Google Drive OAuth adapter and from googledocs Spec.
func TestDeepLinkFlowRoutesSheetsEvernoteSlidesToNotesHandOff(t *testing.T) {
	cases := []struct {
		named, handed, subject, body, utterance, requestID string
		bans                                               []string
	}{
		{"googlesheets", "Google Sheets", "budget", "draft a Google Sheet for Q3 budget", "Draft a Google Sheet for Q3 budget", "request-googlesheets-write", []string{"sheet created", "created", "saved", "shared", "synced"}},
		{"evernote", "Evernote", "meeting notes", "draft an Evernote note for meeting notes", "Draft an Evernote note for meeting notes", "request-evernote-write", []string{"notebook created", "created", "saved", "shared", "synced"}},
		{"googleslides", "Google Slides", "pitch", "draft a Google Slides deck for the pitch", "Draft a Google Slides deck for the pitch", "request-googleslides-write", []string{"slide created", "created", "saved", "shared", "synced"}},
	}
	for _, tc := range cases {
		t.Run(tc.named, func(t *testing.T) {
			payload := `{"verb":"write","app_class":"notes","app_named":"` + tc.named + `","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			preview, err := service.Prepare(context.Background(), "pixel/session/1", tc.requestID, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.named || preview.Verb != manifest.Write {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}
			outcome, err := service.Confirm(context.Background(), "pixel/session/1", tc.requestID, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range tc.bans {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
// User ask: Wave1Specs 70 → 73 — Pocket Casts media play|read + Goodreads notes read + Kindle media read.
// Pack goal: prepare-and-open only; never claim played/downloaded/subscribed, review posted/shelved/rated, purchased/downloaded/read completed.
// Separate from Podcasts RT-2 RSS adapter; Kindle is reader app not Amazon shopping.
func TestDeepLinkFlowRoutesPocketCastsGoodreadsKindleToHandOff(t *testing.T) {
	cases := []struct {
		named, handed, subject, body, utterance, requestID, appClass string
		verb                                                         manifest.Verb
		bans                                                         []string
	}{
		{"pocketcasts", "Pocket Casts", "This American Life", "open This American Life on Pocket Casts", "Open This American Life on Pocket Casts", "request-pocketcasts-play", "media", manifest.Play, []string{"played", "playing", "downloaded", "subscribed"}},
		{"pocketcasts", "Pocket Casts", "search", "find tech podcasts on Pocket Casts", "Find tech podcasts on Pocket Casts", "request-pocketcasts-read", "media", manifest.Read, []string{"played", "downloaded", "subscribed"}},
		{"goodreads", "Goodreads", "Project Hail Mary", "open Project Hail Mary on Goodreads", "Open Project Hail Mary on Goodreads", "request-goodreads-read", "notes", manifest.Read, []string{"posted", "shelved", "rated", "review posted"}},
		{"kindle", "Kindle", "library", "open my Kindle library", "Open my Kindle library", "request-kindle-read", "media", manifest.Read, []string{"purchased", "downloaded", "read completed"}},
	}
	for _, tc := range cases {
		t.Run(tc.requestID, func(t *testing.T) {
			payload := `{"verb":"` + string(tc.verb) + `","app_class":"` + tc.appClass + `","app_named":"` + tc.named + `","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			preview, err := service.Prepare(context.Background(), "pixel/session/1", tc.requestID, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.named || preview.Verb != tc.verb {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}
			outcome, err := service.Confirm(context.Background(), "pixel/session/1", tc.requestID, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range tc.bans {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

// Callers: Wave1Specs → runtime/deeplink, HandOffActions, stage1, deeplink_proof, this test.
// User ask: Wave1Specs 73 → 76 — Claude/ChatGPT/Grok messaging compose prepare-and-open.
// Pack goal: draft prompt / open official apps only; never claim replied/sent/answered/completed chat. Operator does not call their APIs.
func TestDeepLinkFlowRoutesClaudeChatGPTGrokComposeToMessagingHandOff(t *testing.T) {
	cases := []struct {
		named, handed, subject, body, utterance, requestID string
		bans                                               []string
	}{
		{"claude", "Claude", "prompt", "draft a Claude prompt about weekend plans", "Draft a Claude prompt about weekend plans", "request-claude-compose", []string{"replied", "sent", "answered", "completed chat"}},
		{"chatgpt", "ChatGPT", "prompt", "draft a ChatGPT prompt about weekend plans", "Draft a ChatGPT prompt about weekend plans", "request-chatgpt-compose", []string{"replied", "sent", "answered", "completed chat"}},
		{"grok", "Grok", "prompt", "draft a Grok prompt about weekend plans", "Draft a Grok prompt about weekend plans", "request-grok-compose", []string{"replied", "sent", "answered", "completed chat"}},
	}
	for _, tc := range cases {
		t.Run(tc.requestID, func(t *testing.T) {
			payload := `{"verb":"compose","app_class":"messaging","app_named":"` + tc.named + `","subject":"` + tc.subject + `","body":"` + tc.body + `","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			preview, err := service.Prepare(context.Background(), "pixel/session/1", tc.requestID, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.named || preview.Verb != manifest.Compose {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, tc.body) {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}
			outcome, err := service.Confirm(context.Background(), "pixel/session/1", tc.requestID, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range tc.bans {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

// Pinterest prepare-and-open: messaging compose (Facebook/Threads peer); never claim pinned/posted/saved.
func TestDeepLinkFlowRoutesPinterestComposeToMessagingHandOff(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"compose","app_class":"messaging","app_named":"pinterest","subject":"friends","body":"draft a Pinterest pin that I'm heading out","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-pinterest", "Draft a Pinterest pin that I'm heading out")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "pinterest" || preview.Verb != manifest.Compose {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "draft a Pinterest pin that I'm heading out") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-pinterest", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Pinterest" {
		t.Fatalf("outcome=%+v", outcome)
	}
	lower := strings.ToLower(outcome.Detail)
	for _, bad := range []string{"pinned", "posted", "saved", "published", "sent"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
		}
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
	}
}

func TestDeepLinkFlowRoutesMessagingComposeToWave1Extras(t *testing.T) {
	cases := []struct {
		id, named, handed, utterance string
	}{
		{"whatsapp", "whatsapp", "WhatsApp", "Draft a WhatsApp message to Maya that I'm running late"},
		{"messenger", "messenger", "Messenger", "Draft a Messenger message to Maya that I'm running late"},
		{"signal", "signal", "Signal", "Draft a Signal message to Maya that I'm running late"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			payload := `{"verb":"compose","app_class":"messaging","app_named":"` + tc.named + `","subject":"Maya","body":"Running ten minutes late","confidence":0.99}`
			service, err := New(Config{
				Model: func(context.Context, string) ([]byte, error) {
					return []byte(payload), nil
				},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-"+tc.id, tc.utterance)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if preview.AdapterID != tc.id || preview.Verb != manifest.Compose {
				t.Fatalf("preview=%+v", preview)
			}
			shown := strings.Join(preview.Lines, " ")
			if !strings.Contains(shown, "Running ten minutes late") {
				t.Fatalf("preview lines missing draft: %v", preview.Lines)
			}

			outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-"+tc.id, preview.Fingerprint)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != tc.handed {
				t.Fatalf("outcome=%+v", outcome)
			}
			lower := strings.ToLower(outcome.Detail)
			for _, bad := range []string{"sent", "delivered", "message sent"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("outcome claims completion (%q): %q", bad, outcome.Detail)
				}
			}
			if !strings.Contains(lower, "cannot know") {
				t.Fatalf("outcome must say cannot know: %q", outcome.Detail)
			}
		})
	}
}

func TestDeepLinkFlowRejectsMissingModel(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("empty runtime config was accepted")
	}
}
