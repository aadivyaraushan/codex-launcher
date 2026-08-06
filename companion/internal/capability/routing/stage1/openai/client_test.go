package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
)

func TestModelUsesResponsesStructuredOutputAndReturnsAParsedRoute(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer paid-secret" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"write\",\"app_class\":\"tasks\",\"app_named\":\"todoist\",\"subject\":\"Buy oat milk\",\"body\":\"Before tomorrow\",\"confidence\":0.98}"}]}],"usage":{"input_tokens":30,"output_tokens":20,"total_tokens":50}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	router := stage1.New(client.Model)
	route, err := router.Route(context.Background(), "add buy oat milk to todoist before tomorrow")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if route.Verb != manifest.Write || route.AppNamed != "todoist" || route.Subject != "Buy oat milk" {
		t.Fatalf("route=%+v", route)
	}

	if request["model"] != Model || request["store"] != false {
		t.Fatalf("model/store request=%v", request)
	}
	reasoning, _ := request["reasoning"].(map[string]any)
	if reasoning["effort"] != "none" {
		t.Fatalf("reasoning=%v", reasoning)
	}
	text, _ := request["text"].(map[string]any)
	format, _ := text["format"].(map[string]any)
	if format["type"] != "json_schema" || format["strict"] != true || format["name"] != "operator_route" {
		t.Fatalf("text.format=%v", format)
	}
	schema, _ := format["schema"].(map[string]any)
	if schema["additionalProperties"] != false {
		t.Fatalf("route schema permits extra fields: %v", schema)
	}
}

func TestStage1InstructionsCoachConnectedSpotifyAndItsHandoffFallbackAsMedia(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"play\",\"app_class\":\"media\",\"app_named\":\"spotify\",\"subject\":\"lofi\",\"body\":\"calm focus mix\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "play something calm on Spotify"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"spotify", "media", "play", "connected", "playback", "hand-off"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if strings.Contains(lower, "spotify token") {
		t.Fatalf("instructions must not expose token details: %s", instructions)
	}
}

func TestStage1InstructionsNameEveryPersistedOAuthProductionClass(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"calendar\",\"app_named\":\"gcalendar\",\"subject\":\"today\",\"body\":\"\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()
	client, err := New(Config{APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(t.Context(), "show today's calendar"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"app_class calendar", "app_named gcalendar", "app_class drive", "app_named gdrive",
		"app_class slack", "app_named slack", "app_class email", "app_named outlook",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
}

func TestStage1InstructionsSeparateBeeperSendsFromDraftsAndNotificationReplies(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"send\",\"app_class\":\"beeper_messaging\",\"app_named\":\"discord\",\"subject\":\"Raina\",\"body\":\"hello\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()
	client, err := New(Config{APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(t.Context(), "send Raina hello on Discord"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"beeper_messaging", "discord", "instagram", "google messages", "verb send", "subject", "body", "notification_reply", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("instructions do not explain %q: %s", want, instructions)
		}
	}
}

func TestStage1InstructionsCoachAppleMusicPrepareAndOpenAsMedia(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"play\",\"app_class\":\"media\",\"app_named\":\"applemusic\",\"subject\":\"lofi\",\"body\":\"calm focus mix\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "play something calm on Apple Music"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"apple music", "applemusic", "media", "play", "write", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if strings.Contains(lower, "musickit") || strings.Contains(lower, "apple music api") || strings.Contains(lower, "apple music token") {
		t.Fatalf("instructions must not coach API/token control: %s", instructions)
	}
}

func TestStage1InstructionsCoachGooglePhotosPrepareAndOpenAsMedia(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"media\",\"app_named\":\"googlephotos\",\"subject\":\"beach\",\"body\":\"find beach trip photos\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "find beach trip photos in Google Photos"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"google photos", "googlephotos", "media", "read", "write", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if strings.Contains(lower, "photoslibrary") || strings.Contains(lower, "photos library api") || strings.Contains(lower, "picker api") {
		t.Fatalf("instructions must not coach Library/Picker API control: %s", instructions)
	}
}

func TestStage1InstructionsCoachTeamsPrepareAndOpenAsMessaging(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"compose\",\"app_class\":\"messaging\",\"app_named\":\"teams\",\"subject\":\"Maya\",\"body\":\"Running late\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "draft a Teams message to Maya"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"teams", "messaging", "compose", "prepare-and-open", "personal"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if strings.Contains(lower, "bot token") || strings.Contains(lower, "use graph to send") {
		t.Fatalf("instructions must not coach Graph/bot send: %s", instructions)
	}
}

func TestStage1InstructionsCoachBookingPrepareAndOpenAsTravel(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"travel\",\"app_named\":\"booking\",\"subject\":\"Lisbon\",\"body\":\"hotels near Alfama\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "search hotels in Lisbon on Booking.com"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"booking", "travel", "read", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim a hotel was booked") {
		t.Fatalf("instructions must refuse booking completion claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachTravelConnectorsPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"travel\",\"app_named\":\"tripadvisor\",\"subject\":\"Rome\",\"body\":\"best gelato near Trastevere\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "find gelato near Trastevere on Tripadvisor"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"tripadvisor", "viator", "stubhub", "alltrails", "googlemaps",
		"travel", "read", "prepare-and-open",
		"app_named tripadvisor", "app_named viator", "app_named stubhub", "app_named alltrails",
		"app_named googlemaps",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "booked") {
		t.Fatalf("instructions must refuse booked completion claims: %s", instructions)
	}
	if !strings.Contains(lower, "directions") || !strings.Contains(lower, "saved") {
		t.Fatalf("instructions must coach Maps directions/saved-place and refuse saved claims: %s", instructions)
	}
	if !strings.Contains(lower, "navigated") {
		t.Fatalf("instructions must refuse navigated completion claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachAirlinesAndCitymapperPrepareAndOpenAsTravel(t *testing.T) {
	// Callers: go test on stage1/openai; mirrors CoachTravelConnectors / CoachBooking.
	// User ask: stage1 coaching + tests (travel/read; never claim booked).
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"travel\",\"app_named\":\"united\",\"subject\":\"UA123\",\"body\":\"check flight status for UA123\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "check United flight status for UA123"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"united", "delta", "southwest", "american", "citymapper",
		"travel", "read", "prepare-and-open",
		"app_named united", "app_named delta", "app_named southwest",
		"app_named american", "app_named citymapper",
		"flight status", "manage", "directions",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	if !strings.Contains(lower, "booked") || !strings.Contains(lower, "checked-in") || !strings.Contains(lower, "boarded") {
		t.Fatalf("instructions must refuse booked/checked-in/boarded claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachServicesComposePrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"compose\",\"app_class\":\"services\",\"app_named\":\"taskrabbit\",\"subject\":\"shelf\",\"body\":\"assemble IKEA bookshelf\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "prepare a Taskrabbit request to assemble a bookshelf"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"taskrabbit", "thumbtack", "services", "compose", "prepare-and-open",
		"app_named taskrabbit", "app_named thumbtack",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "booked") {
		t.Fatalf("instructions must refuse booked completion claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachFinanceReadPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"finance\",\"app_named\":\"creditkarma\",\"subject\":\"score\",\"body\":\"check my credit score\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "open Credit Karma to check my score"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"creditkarma", "turbotax", "finance", "read", "prepare-and-open",
		"app_named creditkarma", "app_named turbotax",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "filed") || !strings.Contains(lower, "score retrieved") {
		t.Fatalf("instructions must refuse filed/score-retrieved claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachAudiblePrepareAndOpenAsMedia(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"play\",\"app_class\":\"media\",\"app_named\":\"audible\",\"subject\":\"Project Hail Mary\",\"body\":\"continue from where I left off\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "continue Project Hail Mary on Audible"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"audible", "media", "play", "read", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if strings.Contains(lower, "audible api") || strings.Contains(lower, "audible token") {
		t.Fatalf("instructions must not coach API/token control: %s", instructions)
	}
}

func TestStage1InstructionsCoachPodcastsPlainRSSAsMedia(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"play\",\"app_class\":\"media\",\"app_named\":\"podcasts\",\"subject\":\"Hello Feed\",\"body\":\"\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "play the latest episode from my podcast feed"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"podcasts", "media", "play", "read", "plain rss", "enclosure"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "no partner api") || !strings.Contains(lower, "no oauth") {
		t.Fatalf("instructions must say Podcasts needs no partner API/OAuth: %s", instructions)
	}
}

func TestStage1InstructionsCoachMessagesPrepareAndOpenAsMessaging(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"compose\",\"app_class\":\"messaging\",\"app_named\":\"messages\",\"subject\":\"Maya\",\"body\":\"Running ten minutes late\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a text to Maya that I'm running late"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"messages", "messaging", "compose", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if strings.Contains(lower, "messages api") || strings.Contains(lower, "messages token") || strings.Contains(lower, "imessage send") {
		t.Fatalf("instructions must not coach API/token or macOS send: %s", instructions)
	}
}

func TestStage1InstructionsCoachDiscordPrepareAndOpenAsMessaging(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"compose\",\"app_class\":\"messaging\",\"app_named\":\"discord\",\"subject\":\"Maya\",\"body\":\"Running ten minutes late\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a Discord message to Maya that I'm running late"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"discord", "messaging", "compose", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if strings.Contains(lower, "discord bot") || strings.Contains(lower, "discord oauth") || strings.Contains(lower, "discord token") || strings.Contains(lower, "self-bot") {
		t.Fatalf("instructions must not coach bot/oauth/token routes: %s", instructions)
	}
}

func TestStage1InstructionsCoachNetflixPrepareAndOpenAsMedia(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"play\",\"app_class\":\"media\",\"app_named\":\"netflix\",\"subject\":\"Stranger Things\",\"body\":\"open Stranger Things on Netflix\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Open Stranger Things on Netflix"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"netflix", "media", "play", "write", "prepare-and-open", "my list"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "played") {
		t.Fatalf("instructions must refuse played completion claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachConnectedYouTubeAndItsHandoffFallbackAsMedia(t *testing.T) {
	// Callers: go test ./companion/internal/capability/routing/stage1/openai/
	// Covers stage1 coaching for Wave-1 YouTube prepare-and-open (media play|read).
	// User: "HandOffActions + stage1 + proof media log + tests."
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"play\",\"app_class\":\"media\",\"app_named\":\"youtube\",\"subject\":\"lofi beats\",\"body\":\"open lofi beats on YouTube\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Open lofi beats on YouTube"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"youtube", "media", "play", "read", "data api", "connected", "hand-off"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
}

func TestStage1InstructionsCoachAirbnbPrepareAndOpenAsTravel(t *testing.T) {
	// Callers: go test ./companion/internal/capability/routing/stage1/openai/
	// Wave 3 book demoted → travel/read; never claim booked.
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"travel\",\"app_named\":\"airbnb\",\"subject\":\"Lisbon\",\"body\":\"apartments near Alfama\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Search apartments in Lisbon on Airbnb"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"airbnb", "travel", "read", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "booked") {
		t.Fatalf("instructions must refuse booked completion claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachOpenTablePrepareAndOpenAsFood(t *testing.T) {
	// Callers: go test ./companion/internal/capability/routing/stage1/openai/
	// Wave 3 book demoted → food/read; never claim reservation booked.
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"food\",\"app_named\":\"opentable\",\"subject\":\"Friday 7pm\",\"body\":\"table for 2\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Check OpenTable for Friday Italian"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"opentable", "food", "read", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "reservation") || !strings.Contains(lower, "booked") {
		t.Fatalf("instructions must refuse reservation booked claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachGrubhubPrepareAndOpenAsFood(t *testing.T) {
	// Callers: go test ./companion/internal/capability/routing/stage1/openai/
	// Food read|order; never claim checkout completed.
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"order\",\"app_class\":\"food\",\"app_named\":\"grubhub\",\"subject\":\"chipotle bowl\",\"body\":\"chipotle bowl no rice\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Order a Chipotle bowl on Grubhub"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"grubhub", "food", "read", "order", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "checkout completed") {
		t.Fatalf("instructions must refuse checkout completed claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachFacebookPrepareAndOpenAsMessaging(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"compose\",\"app_class\":\"messaging\",\"app_named\":\"facebook\",\"subject\":\"friends\",\"body\":\"draft a post that I'm heading out\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a Facebook post that I'm heading out"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"facebook", "messaging", "compose", "prepare-and-open", "personal"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "posted") {
		t.Fatalf("instructions must refuse posted completion claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachThreadsPrepareAndOpenAsMessaging(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"compose\",\"app_class\":\"messaging\",\"app_named\":\"threads\",\"subject\":\"friends\",\"body\":\"draft a Threads post that I'm heading out\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a Threads post that I'm heading out"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"threads", "messaging", "compose", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "posted") {
		t.Fatalf("instructions must refuse posted completion claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachTikTokPrepareAndOpenAsMessaging(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"compose\",\"app_class\":\"messaging\",\"app_named\":\"tiktok\",\"subject\":\"friends\",\"body\":\"draft a TikTok caption that I'm heading out\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a TikTok caption that I'm heading out"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"tiktok", "messaging", "compose", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "posted") {
		t.Fatalf("instructions must refuse posted completion claims: %s", instructions)
	}
}

func TestStage1InstructionsCoachExpediaPrepareAndOpenAsTravel(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"travel\",\"app_named\":\"expedia\",\"subject\":\"Lisbon\",\"body\":\"hotels near Alfama under $150\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Search hotels in Lisbon on Expedia"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"expedia", "travel", "read", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "booked") {
		t.Fatalf("instructions must refuse booked completion claims: %s", instructions)
	}
}

// Shopping prepare-and-open (Target/Walmart/Nike/Sephora/Wayfair): browse/open only; no UCP cart API.
func TestStage1InstructionsCoachShoppingPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"shopping\",\"app_named\":\"target\",\"subject\":\"paper towels\",\"body\":\"search paper towels\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Open Target to browse paper towels"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	// Callers: go test ./companion/internal/capability/routing/stage1/openai/ (this file).
	// User ask: shopping +ebay in stage1 coaching (Wave1Specs 51 → 54).
	for _, want := range []string{
		"shopping", "target", "walmart", "nike", "sephora", "wayfair", "ebay", "read", "prepare-and-open",
		"app_named target", "app_named walmart", "app_named nike", "app_named sephora", "app_named wayfair", "app_named ebay",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "or shopping") {
		t.Fatalf("stable app class list must include shopping: %s", instructions)
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	// Require ALL refuse tokens (no soft OR short-circuit). Coaching already
	// has cart built / checkout completed / ordered / bid — keep AND asserts.
	for _, refuse := range []string{"cart", "checkout", "ordered", "bid"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for shopping: %s", refuse, instructions)
		}
	}
}

// Kayak prepare-and-open: travel read; Wave 3 book demoted → read; never claim booked.
func TestStage1InstructionsCoachKayakPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"travel\",\"app_named\":\"kayak\",\"subject\":\"Lisbon\",\"body\":\"flights to Lisbon next weekend\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Search flights to Lisbon on Kayak"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"kayak", "travel", "read", "prepare-and-open", "app_named kayak"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "booked") {
		t.Fatalf("instructions must refuse booked completion claims: %s", instructions)
	}
}

// Priceline prepare-and-open: travel read; Wave 3 book demoted → read; never claim booked.
func TestStage1InstructionsCoachPricelinePrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"travel\",\"app_named\":\"priceline\",\"subject\":\"Miami\",\"body\":\"hotels in Miami next weekend\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Search hotels in Miami on Priceline"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"priceline", "travel", "read", "prepare-and-open", "app_named priceline"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "booked") {
		t.Fatalf("instructions must refuse booked completion claims: %s", instructions)
	}
}

// LinkedIn prepare-and-open: messaging compose (Facebook/Threads peer); never claim posted/commented.
func TestStage1InstructionsCoachLinkedInPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"compose\",\"app_class\":\"messaging\",\"app_named\":\"linkedin\",\"subject\":\"network\",\"body\":\"draft a LinkedIn post\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a LinkedIn post that I'm open to work"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"linkedin", "messaging", "compose", "prepare-and-open", "app_named linkedin"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	// Require BOTH refuse tokens (no soft OR). LinkedIn coaching already has both.
	for _, refuse := range []string{"posted", "commented"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for LinkedIn: %s", refuse, instructions)
		}
	}
}

// Pinterest prepare-and-open: messaging compose (Facebook/Threads peer); never claim pinned/posted/saved.
func TestStage1InstructionsCoachPinterestPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"compose\",\"app_class\":\"messaging\",\"app_named\":\"pinterest\",\"subject\":\"friends\",\"body\":\"draft a Pinterest pin that I'm heading out\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a Pinterest pin that I'm heading out"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"pinterest", "messaging", "compose", "prepare-and-open", "app_named pinterest"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"pinned", "posted", "saved"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for Pinterest: %s", refuse, instructions)
		}
	}
}

// Callers: Wave1Specs → stage1 coaching, HandOffActions, deeplink_proof, this test.
// User ask: Wave1Specs 55 → 58 — Duolingo/Fitbit services read; never claim lesson completed / workout logged / synced / saved.
func TestStage1InstructionsCoachDuolingoFitbitPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"services\",\"app_named\":\"duolingo\",\"subject\":\"Spanish\",\"body\":\"open Spanish lesson A1 greetings\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Open Duolingo for Spanish A1 greetings"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"duolingo", "fitbit", "services", "read", "prepare-and-open",
		"app_named duolingo", "app_named fitbit",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"lesson completed", "workout logged", "synced", "saved"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for Duolingo/Fitbit: %s", refuse, instructions)
		}
	}
}

// Callers: Wave1Specs → stage1 coaching, HandOffActions, deeplink_proof, this test.
// User ask: Wave1Specs 55 → 58 — Shazam media read; never claim identified/played/saved.
func TestStage1InstructionsCoachShazamPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"media\",\"app_named\":\"shazam\",\"subject\":\"song\",\"body\":\"identify this song nearby\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Open Shazam to identify this song"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"shazam", "media", "read", "prepare-and-open", "app_named shazam"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"identified", "played", "saved"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for Shazam: %s", refuse, instructions)
		}
	}
}

// Callers: Wave1Specs → stage1 coaching, HandOffActions, deeplink_proof, this test.
// User ask: Wave1Specs 58 → 61 — Chromecast media play; never claim cast started/playing/connected.
func TestStage1InstructionsCoachChromecastPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"play\",\"app_class\":\"media\",\"app_named\":\"chromecast\",\"subject\":\"living room\",\"body\":\"open Chromecast to cast living room TV\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Open Chromecast to cast living room TV"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"chromecast", "media", "play", "prepare-and-open", "app_named chromecast"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"cast started", "playing", "connected"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for Chromecast: %s", refuse, instructions)
		}
	}
}

// Callers: Wave1Specs → stage1 coaching, HandOffActions, deeplink_proof, this test.
// User ask: Wave1Specs 58 → 61 — YouTube Music / SoundCloud media play|read; never claim played/playlist/library.
func TestStage1InstructionsCoachYouTubeMusicAndSoundCloudPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"play\",\"app_class\":\"media\",\"app_named\":\"youtubemusic\",\"subject\":\"lofi\",\"body\":\"open lofi beats on YouTube Music\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Open lofi beats on YouTube Music"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"youtubemusic", "youtube music", "soundcloud", "media", "play", "read", "prepare-and-open",
		"app_named youtubemusic", "app_named soundcloud",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"played", "added to playlist", "library changed"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for YouTube Music/SoundCloud: %s", refuse, instructions)
		}
	}
}

// Callers: Wave1Specs → stage1 coaching, HandOffActions, deeplink_proof, this test.
// User ask: Wave1Specs 61 → 64 — Pandora media play|read; never claim played/station changed.
func TestStage1InstructionsCoachPandoraPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"play\",\"app_class\":\"media\",\"app_named\":\"pandora\",\"subject\":\"lofi\",\"body\":\"open lofi beats on Pandora\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Open lofi beats on Pandora"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"pandora", "media", "play", "read", "prepare-and-open", "app_named pandora"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"played", "added to playlist", "station changed"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for Pandora: %s", refuse, instructions)
		}
	}
}

// Callers: Wave1Specs → stage1 coaching, HandOffActions, deeplink_proof, this test.
// User ask: Wave1Specs 61 → 64 — Asana/Trello tasks write; never claim task created/card moved/assigned/completed.
// Not Todoist RT-2 — prepare-and-open only.
func TestStage1InstructionsCoachAsanaAndTrelloPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"write\",\"app_class\":\"tasks\",\"app_named\":\"asana\",\"subject\":\"groceries\",\"body\":\"draft an Asana task to buy oat milk\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft an Asana task to buy oat milk"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"asana", "trello", "tasks", "write", "prepare-and-open",
		"app_named asana", "app_named trello",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"task created", "card moved", "assigned", "completed"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for Asana/Trello: %s", refuse, instructions)
		}
	}
}

// Callers: Wave1Specs → stage1 coaching, HandOffActions, deeplink_proof, this test.
// User ask: Wave1Specs 64 → 67 — Microsoft To Do tasks write; never claim task created/assigned/completed.
// Not Todoist RT-2 — prepare-and-open only.
func TestStage1InstructionsCoachMSToDoPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"write\",\"app_class\":\"tasks\",\"app_named\":\"mstodo\",\"subject\":\"groceries\",\"body\":\"draft a Microsoft To Do task to buy oat milk\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a Microsoft To Do task to buy oat milk"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"mstodo", "microsoft to do", "tasks", "write", "prepare-and-open",
		"app_named mstodo",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"task created", "assigned", "completed"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for Microsoft To Do: %s", refuse, instructions)
		}
	}
}

// Callers: Wave1Specs → stage1 coaching, HandOffActions, deeplink_proof, this test.
// User ask: Wave1Specs 64 → 67 — Google Docs notes write + Dropbox notes read; never claim doc/file completion.
func TestStage1InstructionsCoachGoogleDocsAndDropboxPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"write\",\"app_class\":\"notes\",\"app_named\":\"googledocs\",\"subject\":\"meeting\",\"body\":\"draft a Google Doc for meeting notes\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a Google Doc for meeting notes"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"googledocs", "google docs", "dropbox", "notes", "write", "read", "prepare-and-open",
		"app_named googledocs", "app_named dropbox",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"created", "saved", "shared", "uploaded", "downloaded", "synced"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for Google Docs/Dropbox: %s", refuse, instructions)
		}
	}
}

// Callers: Wave1Specs → stage1 coaching, HandOffActions, deeplink_proof, this test.
// User ask: Wave1Specs 67 → 70 — Google Sheets / Evernote / Google Slides notes write; never claim created/saved/shared/synced.
// Separate from Google Drive OAuth and googledocs Spec.
func TestStage1InstructionsCoachSheetsEvernoteSlidesPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"write\",\"app_class\":\"notes\",\"app_named\":\"googlesheets\",\"subject\":\"budget\",\"body\":\"draft a Google Sheet for Q3 budget\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a Google Sheet for Q3 budget"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"googlesheets", "google sheets", "evernote", "googleslides", "google slides",
		"notes", "write", "prepare-and-open",
		"app_named googlesheets", "app_named evernote", "app_named googleslides",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"sheet created", "slide created", "notebook created", "created", "saved", "shared", "synced"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for Sheets/Evernote/Slides: %s", refuse, instructions)
		}
	}
}

// Callers: Wave1Specs → stage1 coaching, HandOffActions, deeplink_proof, this test.
// User ask: Wave1Specs 70 → 73 — Pocket Casts media play|read + Goodreads notes read + Kindle media read.
// Pack goal: prepare-and-open; never claim played/downloaded/subscribed, shelved/rated, purchased/downloaded/read completed.
// Separate from Podcasts RT-2; Kindle is reader app not Amazon shopping.
func TestStage1InstructionsCoachPocketCastsGoodreadsKindlePrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"play\",\"app_class\":\"media\",\"app_named\":\"pocketcasts\",\"subject\":\"This American Life\",\"body\":\"open This American Life on Pocket Casts\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Open This American Life on Pocket Casts"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"pocketcasts", "pocket casts", "goodreads", "kindle",
		"media", "notes", "play", "read", "prepare-and-open",
		"app_named pocketcasts", "app_named goodreads", "app_named kindle",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"played", "downloaded", "subscribed", "shelved", "rated", "purchased", "read completed"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for Pocket Casts/Goodreads/Kindle: %s", refuse, instructions)
		}
	}
}

// Callers: Wave1Specs → stage1 coaching, HandOffActions, deeplink_proof, this test.
// User ask: Wave1Specs 73 → 76 — Claude/ChatGPT/Grok messaging compose prepare-and-open.
// Pack goal: draft prompt / open official apps only; never claim replied/sent/answered/completed chat. Operator does not call their APIs.
func TestStage1InstructionsCoachClaudeChatGPTGrokPrepareAndOpen(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"compose\",\"app_class\":\"messaging\",\"app_named\":\"claude\",\"subject\":\"prompt\",\"body\":\"draft a Claude prompt about weekend plans\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a Claude prompt about weekend plans"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"claude", "chatgpt", "chat gpt", "grok",
		"messaging", "compose", "prepare-and-open",
		"app_named claude", "app_named chatgpt", "app_named grok",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if !strings.Contains(lower, "never claim") {
		t.Fatalf("instructions must refuse completion claims: %s", instructions)
	}
	for _, refuse := range []string{"replied", "sent", "answered", "completed chat"} {
		if !strings.Contains(lower, refuse) {
			t.Fatalf("instructions must refuse %q for Claude/ChatGPT/Grok: %s", refuse, instructions)
		}
	}
}

// Callers: go test ./companion/internal/capability/routing/stage1/openai/
// Covers stage1 coaching for Wave-1 WhatsApp/Messenger/Signal compose hand-offs.
// Notification reply is a separate ReplyCapability path — never claim sent here.
func TestStage1InstructionsCoachMessagingExtrasPrepareAndOpen(t *testing.T) {
	cases := []struct {
		named, utterance string
		wants            []string
	}{
		{"whatsapp", "Draft a WhatsApp message to Maya that I'm running late", []string{"whatsapp", "messaging", "compose", "prepare-and-open"}},
		{"messenger", "Draft a Messenger message to Maya that I'm running late", []string{"messenger", "messaging", "compose", "prepare-and-open"}},
		{"signal", "Draft a Signal message to Maya that I'm running late", []string{"signal", "messaging", "compose", "prepare-and-open"}},
	}
	for _, tc := range cases {
		t.Run(tc.named, func(t *testing.T) {
			var request map[string]any
			payload := `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"compose\",\"app_class\":\"messaging\",\"app_named\":\"` + tc.named + `\",\"subject\":\"Maya\",\"body\":\"Running ten minutes late\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				_, _ = io.WriteString(w, payload)
			}))
			defer server.Close()

			client, err := New(Config{
				APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if _, err := client.Model(context.Background(), tc.utterance); err != nil {
				t.Fatalf("Model: %v", err)
			}
			instructions, _ := request["instructions"].(string)
			lower := strings.ToLower(instructions)
			for _, want := range tc.wants {
				if !strings.Contains(lower, want) {
					t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
				}
			}
			if !strings.Contains(lower, "never claim") || !strings.Contains(lower, "sent") {
				t.Fatalf("instructions must refuse send-completion claims: %s", instructions)
			}
		})
	}
}

// Callers: go test ./companion/internal/capability/routing/stage1/openai/
// Covers stage1 coaching for Wave-1 Uber estimates prepare-and-open (rides/read).
func TestStage1InstructionsCoachUberEstimatesPrepareAndOpenAsRides(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"rides\",\"app_named\":\"uber\",\"subject\":\"airport\",\"body\":\"fare estimate to SFO\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "What's an Uber estimate to SFO?"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"uber", "rides", "read", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if strings.Contains(lower, "uber oauth") || strings.Contains(lower, "riders api") || strings.Contains(lower, "uber api key") {
		t.Fatalf("instructions must not coach Uber API/oauth: %s", instructions)
	}
}

// Callers: go test ./companion/internal/capability/routing/stage1/openai/
// Covers stage1 coaching for Wave-1 Lyft estimates prepare-and-open (rides/read).
func TestStage1InstructionsCoachLyftEstimatesPrepareAndOpenAsRides(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"rides\",\"app_named\":\"lyft\",\"subject\":\"airport\",\"body\":\"fare estimate to SFO\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "What's a Lyft estimate to SFO?"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"lyft", "rides", "read", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if strings.Contains(lower, "lyft oauth") || strings.Contains(lower, "lyft api") {
		t.Fatalf("instructions must not coach Lyft API/oauth: %s", instructions)
	}
}

// Callers: go test ./companion/internal/capability/routing/stage1/openai/
// Covers stage1 coaching for Wave-1 Google Keep prepare-and-open (notes/write).
func TestStage1InstructionsCoachGoogleKeepPrepareAndOpenAsNotes(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"write\",\"app_class\":\"notes\",\"app_named\":\"googlekeep\",\"subject\":\"groceries\",\"body\":\"milk eggs bread\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Draft a Keep note for groceries"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"googlekeep", "google keep", "notes", "write", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if strings.Contains(lower, "keep api") || strings.Contains(lower, "notes api") || strings.Contains(lower, "keep oauth") {
		t.Fatalf("instructions must not coach Keep API/oauth: %s", instructions)
	}
}

// Callers: go test ./companion/internal/capability/routing/stage1/openai/
// Covers stage1 coaching for Uber Eats / Resy / DoorDash food prepare-and-open.
func TestStage1InstructionsCoachUberEatsResyDoorDashPrepareAndOpenAsFood(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"food\",\"app_named\":\"ubereats\",\"subject\":\"Thai\",\"body\":\"nearby Thai\",\"confidence\":0.97}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), "Find Thai on Uber Eats"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{"ubereats", "uber eats", "resy", "doordash", "food", "read", "prepare-and-open"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("stage1 instructions missing %q: %s", want, instructions)
		}
	}
	if strings.Contains(lower, "doordash oauth") || strings.Contains(lower, "uber eats api") || strings.Contains(lower, "resy api") {
		t.Fatalf("instructions must not coach partner API/oauth: %s", instructions)
	}
}

func TestModelErrorsAndLogsNeverLeakAPIKeyUtteranceOrVendorBody(t *testing.T) {
	const key = "paid-super-secret"
	const utterance = "private medical appointment in todoist"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "vendor says private account detail", http.StatusBadGateway)
	}))
	defer server.Close()

	var logs bytes.Buffer
	client, err := New(Config{APIKey: key, BaseURL: server.URL, HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = client.Model(context.Background(), utterance)
	if err == nil {
		t.Fatal("failed response returned no error")
	}
	combined := err.Error() + logs.String()
	for _, secret := range []string{key, utterance, "private account detail"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("output leaked %q: %s", secret, combined)
		}
	}
}

func TestModelRejectsResponsesWithoutOneOutputTextBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"reasoning","summary":[]}]}`)
	}))
	defer server.Close()
	client, _ := New(Config{APIKey: "secret", BaseURL: server.URL, HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if _, err := client.Model(context.Background(), "do something"); err == nil {
		t.Fatal("response with no output_text was accepted")
	}
}

func TestMissingAPIKeyIsRefusedBeforeAnyRequest(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("client accepted an empty API key")
	}
}

func TestLiveRouteAgainstApprovedAccount(t *testing.T) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	var logs bytes.Buffer
	client, err := New(Config{APIKey: key, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	route, err := stage1.New(client.Model).Route(context.Background(), "Add buy oat milk to Todoist before tomorrow")
	if err != nil {
		t.Fatalf("live Route: %v", err)
	}
	if route.Verb != manifest.Write || !strings.EqualFold(route.AppNamed, "todoist") {
		t.Fatalf("live route=%+v", route)
	}
	t.Log(logs.String())
}

// A directions question carries two places, so the router has to be able to
// hand back named slots. Strict mode forbids a free-form map: every object
// must close its property list and mark every property required. So the
// slots are a fixed, named set, and an unused one comes back null.
func TestRouteFormatLetsTheModelReturnNamedSlots(t *testing.T) {
	format := routeFormat()
	schema, ok := format["schema"].(map[string]any)
	if !ok {
		t.Fatalf("format has no schema object: %+v", format)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no properties object: %+v", schema)
	}
	fields, ok := properties["fields"].(map[string]any)
	if !ok {
		t.Fatalf("schema cannot express named slots, so the model can never fill one: %+v", properties)
	}
	if !contains(schema["required"], "fields") {
		t.Errorf("fields is not required, which strict mode rejects: %v", schema["required"])
	}

	slots, ok := fields["properties"].(map[string]any)
	if !ok {
		t.Fatalf("fields has no properties object: %+v", fields)
	}
	for _, want := range []string{"origin", "destination"} {
		if _, ok := slots[want]; !ok {
			t.Errorf("slot %q missing, so a directions question cannot be routed", want)
		}
	}
	if fields["additionalProperties"] != false {
		t.Errorf("fields allows extra keys, which strict mode rejects: %v", fields["additionalProperties"])
	}
	for name := range slots {
		if !contains(fields["required"], name) {
			t.Errorf("slot %q is not in required, which strict mode rejects", name)
		}
	}
	// A slot the user did not name has to be expressible as absent. Strict
	// mode's only way to say that is null alongside string.
	for name, raw := range slots {
		slot, ok := raw.(map[string]any)
		if !ok {
			t.Errorf("slot %q is not an object: %v", name, raw)
			continue
		}
		types, ok := slot["type"].([]string)
		if !ok || len(types) != 2 || types[0] != "string" || types[1] != "null" {
			t.Errorf("slot %q type=%v, want [string null] so an unused slot can come back empty", name, slot["type"])
		}
	}
	// No slot name may look like a resolved handle; that is stage 2's job.
	for name := range slots {
		for _, bad := range []string{"handle", "phone", "number", "email", "address", "thread", "contact", "recipient"} {
			if strings.Contains(strings.ToLower(name), bad) {
				t.Errorf("slot %q names something stage 1 may never return", name)
			}
		}
	}
}

// B3: the model must be able to name a Beeper manage op in fields.operation.
func TestRouteFormatIncludesOperationNamedSlot(t *testing.T) {
	format := routeFormat()
	schema, _ := format["schema"].(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	fields, _ := properties["fields"].(map[string]any)
	slots, ok := fields["properties"].(map[string]any)
	if !ok {
		t.Fatalf("fields has no properties: %+v", fields)
	}
	if _, ok := slots["operation"]; !ok {
		t.Fatalf("namedSlots missing operation; Beeper modify/cancel routes cannot name the op: slots=%v", slotsKeys(slots))
	}
	if !contains(fields["required"], "operation") {
		t.Fatalf("operation must be required under strict mode: required=%v", fields["required"])
	}
}

// B3: coach reads + manage ops onto beeper_messaging with the closed operation list.
func TestStage1InstructionsCoachBeeperReadsAndManageOperations(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"beeper_messaging\",\"app_named\":\"instagram\",\"subject\":\"\",\"body\":\"\",\"fields\":{\"origin\":null,\"destination\":null,\"navigate\":null,\"list\":null,\"folder\":null,\"operation\":null},\"confidence\":0.9}"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	client, err := New(Config{APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(t.Context(), "what's my unread Instagram message"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"beeper_messaging", "verb read", "app_named instagram",
		"operation", "reply", "edit", "delete", "react", "unreact",
		"mark_read", "mark_unread", "archive", "unarchive", "pin", "mute",
		"set_reminder", "clear_reminder",
		"verb modify", "verb cancel",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("Beeper read/manage coaching missing %q: %s", want, instructions)
		}
	}
}

// B3 routing fixtures: documented expected routes for the motivating Beeper asks.
// The mock returns the coached shape; we assert ParseRoute keeps verb/class/named/operation.
func TestBeeperRoutingFixturesPreserveVerbClassNamedAndOperation(t *testing.T) {
	fixtures := []struct {
		name, utterance, responseText string
		wantVerb                      manifest.Verb
		wantClass, wantNamed, wantOp  string
	}{
		{
			name: "unread Instagram", utterance: "what's my most recent unread Instagram message",
			responseText: `{"verb":"read","app_class":"beeper_messaging","app_named":"instagram","subject":"","body":"","fields":{"origin":null,"destination":null,"navigate":null,"list":null,"folder":null,"operation":null},"confidence":0.95}`,
			wantVerb: manifest.Read, wantClass: "beeper_messaging", wantNamed: "instagram", wantOp: "",
		},
		{
			name: "reply Discord", utterance: "reply to Maya on Discord that I'll be late",
			responseText: `{"verb":"send","app_class":"beeper_messaging","app_named":"discord","subject":"Maya","body":"I'll be late","fields":{"origin":null,"destination":null,"navigate":null,"list":null,"folder":null,"operation":"reply"},"confidence":0.95}`,
			wantVerb: manifest.Send, wantClass: "beeper_messaging", wantNamed: "discord", wantOp: "reply",
		},
		{
			name: "delete last message", utterance: "delete my last message on Discord",
			responseText: `{"verb":"cancel","app_class":"beeper_messaging","app_named":"discord","subject":"","body":"","fields":{"origin":null,"destination":null,"navigate":null,"list":null,"folder":null,"operation":"delete"},"confidence":0.95}`,
			wantVerb: manifest.Cancel, wantClass: "beeper_messaging", wantNamed: "discord", wantOp: "delete",
		},
		{
			name: "archive group chat", utterance: "archive that group chat on Discord",
			responseText: `{"verb":"modify","app_class":"beeper_messaging","app_named":"discord","subject":"group chat","body":"","fields":{"origin":null,"destination":null,"navigate":null,"list":null,"folder":null,"operation":"archive"},"confidence":0.95}`,
			wantVerb: manifest.Modify, wantClass: "beeper_messaging", wantNamed: "discord", wantOp: "archive",
		},
	}
	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]any{
				"id": "resp_1",
				"output": []map[string]any{{
					"type": "message", "role": "assistant",
					"content": []map[string]any{{"type": "output_text", "text": tc.responseText}},
				}},
				"usage": map[string]int{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(payload)
			}))
			defer server.Close()
			client, err := New(Config{APIKey: "k", BaseURL: server.URL, HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			route, err := stage1.New(client.Model).Route(t.Context(), tc.utterance)
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			if route.Verb != tc.wantVerb || route.AppClass != tc.wantClass || route.AppNamed != tc.wantNamed {
				t.Fatalf("route=%+v, want verb=%s class=%s named=%s", route, tc.wantVerb, tc.wantClass, tc.wantNamed)
			}
			gotOp := route.Fields["operation"]
			if gotOp != tc.wantOp {
				t.Fatalf("operation=%q, want %q (fields=%v)", gotOp, tc.wantOp, route.Fields)
			}
		})
	}
}

func slotsKeys(slots map[string]any) []string {
	keys := make([]string, 0, len(slots))
	for k := range slots {
		keys = append(keys, k)
	}
	return keys
}

// contains reports whether a schema list value holds want, accepting either
// []string or []any so the assertion does not depend on how the map was built.
func contains(list any, want string) bool {
	switch typed := list.(type) {
	case []string:
		for _, item := range typed {
			if item == want {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}
