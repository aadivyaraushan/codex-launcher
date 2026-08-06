// Package openai supplies the production model call for stage 1 routing.
// It uses the Responses API with a strict JSON schema so the model can return
// only the fields stage1.ParseRoute accepts.
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	Model          = "gpt-5.6-luna"
	defaultBaseURL = "https://api.openai.com"
)

var ErrMissingAPIKey = errors.New("openai stage1: API key is required")

type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// Fact-force (edit): Callers=New + NewBrokered (brokered.go); Model uses
// responsesPath when set. Grep: responsesPath was referenced but field missing.
// No data files. User: "Implement OpenAI+Beeper phone-runtime plan — SLICE 1 only"
type Client struct {
	apiKey        string
	baseURL       string
	responsesPath string // empty → /v1/responses; brokered → /v1/broker/openai/responses
	http          *http.Client
	logger        *slog.Logger
}

func New(config Config) (*Client, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, ErrMissingAPIKey
	}
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	return &Client{
		apiKey: config.APIKey, baseURL: strings.TrimRight(config.BaseURL, "/"),
		http: config.HTTPClient, logger: config.Logger,
	}, nil
}

func (c *Client) Model(ctx context.Context, utterance string) ([]byte, error) {
	request := responseRequest{
		Model:     Model,
		Store:     false,
		Reasoning: reasoningConfig{Effort: "none"},
		Instructions: strings.Join([]string{
			"Route the user's request.",
			"Use write for creating or changing stored information such as a task, note, calendar event, playlist, or list item.",
			"Use order only for selecting or purchasing goods or food. Use book only for reserving a service, trip, table, or appointment.",
			"Use send for delivering a message or post, compose for drafting one without delivering it, read for retrieving information, play for media playback, cancel for cancelling an existing action, and modify for changing an existing action whose own verb is not write.",
			"Put the main named object or title in subject. Put message text, description, timing, and other supporting details in body.",
			"Use a short stable app class such as tasks, notes, calendar, messaging, media, food, money, travel, rides, services, finance, or shopping.",
			"For money draft-and-open (Venmo, Cash App, Zelle), use verb compose and app_class money — never invent a pay verb.",
			"For Spotify, use app_class media and app_named spotify with verb play and put the track or search in subject. When Spotify is connected this route searches the real catalog, previews the track and target device, and controls playback after confirmation; without a connection the same id falls back to a hand-off that only opens Spotify. Never claim playback until the returned outcome says it completed.",
			"For Audible prepare-and-open, use app_class media and app_named audible with verb play (listen/continue) or read (library or title lookup). Open only — never claim playback control or library changes.",
			"For Apple Music prepare-and-open, use app_class media and app_named applemusic with verb play (listen/search) or write (playlist or library edit intent). Open only — never claim playback control or playlist changes.",
			"For Google Photos prepare-and-open, use app_class media and app_named googlephotos with verb read (find/browse) or write (save/upload intent). Open only — never claim library search or upload completed.",
			"For Netflix prepare-and-open, use app_class media and app_named netflix with verb play (search/open title) or write (My List intent). Open only — never claim played or added to My List.",
			"For YouTube, use app_class media and app_named youtube with verb play or read and put the video search in subject. When the YouTube Data API is connected this route searches real videos and previews the resolved title before opening it; without a connection the same id falls back to a hand-off that only opens YouTube. Never claim a video opened until the returned outcome says it completed.",
			"For connected Google Calendar, use app_class calendar and app_named gcalendar with verb read or write; put the event title or search in subject and timing/details in body.",
			"For connected Google Drive, use app_class drive and app_named gdrive with verb read or write; put the file name or search in subject and new file content in body.",
			"For connected Slack channels, use app_class slack and app_named slack with verb read or send; put the channel name in subject and the exact message in body.",
			"For connected Outlook mail, use app_class email and app_named outlook with verb read, write, or send; put the message subject in subject and recipient/body details in body.",
			"For Podcasts plain RSS, use app_class media and app_named podcasts with verb read (list episodes from a feed) or play (resolve one episode enclosure URL). Completes via enclosure — no partner API and no OAuth.",
			"For a new confirmed message through a connected Beeper account on Instagram, Discord, or Google Messages, use app_class beeper_messaging with verb send. Put the person's visible conversation name in subject, the full message in body, and use app_named instagram, discord, or messages respectively. This route searches Beeper's live chat list and asks if more than one conversation matches.",
			"For reading messages through a connected Beeper account (unread scan, recent messages in a named chat, or search), use app_class beeper_messaging with verb read and app_named instagram, discord, or messages. Leave subject empty for a network-wide unread ask; put the conversation name in subject when named; put search text in body when searching. Leave fields.operation null for reads.",
			"For managing an existing Beeper conversation or message (reply, edit, delete, react, unreact, mark_read, mark_unread, archive, unarchive, pin, mute, set_reminder, clear_reminder), use app_class beeper_messaging with app_named instagram, discord, or messages. Put the chosen name from that closed list in fields.operation. Use verb send when operation is reply; verb cancel when operation is delete or clear_reminder; verb modify for the other manage operations.",
			"For a draft the user only wants opened without sending, use app_class messaging and verb compose with app_named instagram, discord, or messages. That prepare-and-open route never claims the message was sent.",
			"For personal Teams prepare-and-open, use app_class messaging and app_named teams with verb compose. Open only — never claim the message was sent (Graph chat send does not support personal accounts).",
			// The separate path the three lines below point at. Without this
			// sentence the model has no word for it, so every "reply to Maya"
			// came back as messaging/compose and opened the app with a draft —
			// the hand-off the reply route exists to replace.
			"For replying into a message thread already live on the phone (the user says reply, answer, respond, or write back to somebody who just messaged them), use app_class notification_reply with verb send, putting the person's name exactly as the user said it in subject and the reply text in body — the phone types it into the notification's own reply box and only ever learns that the app accepted the text.",
			"For WhatsApp prepare-and-open, use app_class messaging and app_named whatsapp with verb compose. Open only — never claim the message was sent (notification reply is a separate path).",
			"For Messenger prepare-and-open, use app_class messaging and app_named messenger with verb compose. Open only — never claim the message was sent (notification reply is a separate path).",
			"For Signal prepare-and-open, use app_class messaging and app_named signal with verb compose. Open only — never claim the message was sent (notification reply is a separate path).",
			"For personal Facebook prepare-and-open, use app_class messaging and app_named facebook with verb compose. Open only — never claim the post was posted.",
			"For Threads prepare-and-open, use app_class messaging and app_named threads with verb compose. Open only — never claim the post was posted or replied.",
			"For TikTok prepare-and-open, use app_class messaging and app_named tiktok with verb compose. Open only — never claim the post was posted.",
			"For Booking.com prepare-and-open, use app_class travel and app_named booking with verb read. Open only — never claim a hotel was booked.",
			"For Tripadvisor prepare-and-open, use app_class travel and app_named tripadvisor with verb read. Open only — never claim booked.",
			"For Viator prepare-and-open, use app_class travel and app_named viator with verb read. Open only — never claim booked.",
			"For StubHub prepare-and-open, use app_class travel and app_named stubhub with verb read. Open only — never claim booked.",
			"For AllTrails prepare-and-open, use app_class travel and app_named alltrails with verb read. Open only — never claim booked.",
			"For Google Maps prepare-and-open, use app_class travel and app_named googlemaps with verb read (directions look-up) or write (saved-place intent). Open only — never claim navigated or saved.",
			"For Google Maps places and directions with a real answer inside Operator, use app_class travel and app_named maps with verb read: subject the place name for a place lookup, or the origin and destination for directions. Completes via Places API and Routes API — a real place name/address or route distance and duration is shown, not just an open link. Separate from the googlemaps prepare-and-open Spec (app_named googlemaps), which only opens the app.",
			"For United prepare-and-open, use app_class travel and app_named united with verb read (flight status or manage-booking browse). Open only — never claim booked, checked-in, or boarded.",
			"For Delta prepare-and-open, use app_class travel and app_named delta with verb read (flight status or manage-booking browse). Open only — never claim booked, checked-in, or boarded.",
			"For Southwest prepare-and-open, use app_class travel and app_named southwest with verb read (flight status or manage-booking browse). Open only — never claim booked, checked-in, or boarded.",
			"For American Airlines prepare-and-open, use app_class travel and app_named american with verb read (flight status or manage-booking browse). Open only — never claim booked, checked-in, or boarded.",
			"For Citymapper prepare-and-open, use app_class travel and app_named citymapper with verb read (transit directions). Open only — never claim booked, checked-in, or boarded.",
			"For Taskrabbit prepare-and-open, use app_class services and app_named taskrabbit with verb compose. Open only — never claim booked.",
			"For Thumbtack prepare-and-open, use app_class services and app_named thumbtack with verb compose. Open only — never claim booked.",
			"For Credit Karma prepare-and-open, use app_class finance and app_named creditkarma with verb read. Open only — never claim score retrieved.",
			"For TurboTax prepare-and-open, use app_class finance and app_named turbotax with verb read. Open only — never claim filed.",
			"For Uber estimates prepare-and-open, use app_class rides and app_named uber with verb read. Open only — never claim a ride was booked.",
			"For Lyft estimates prepare-and-open, use app_class rides and app_named lyft with verb read. Open only — never claim a ride was booked.",
			"For Google Keep prepare-and-open, use app_class notes and app_named googlekeep with verb write. Open only — never claim the note was saved.",
			"For Uber Eats prepare-and-open, use app_class food and app_named ubereats with verb read. Open only — never claim an order was placed.",
			"For Resy prepare-and-open, use app_class food and app_named resy with verb read. Open only — never claim a reservation was booked.",
			"For DoorDash prepare-and-open, use app_class food and app_named doordash with verb read (browse) or order (cart intent). Open only — never claim checkout completed.",
			"For Airbnb prepare-and-open, use app_class travel and app_named airbnb with verb read. Open only — never claim booked.",
			"For Expedia prepare-and-open, use app_class travel and app_named expedia with verb read. Open only — never claim booked.",
			"For Target prepare-and-open, use app_class shopping and app_named target with verb read. Open only — never claim cart built, ordered, or checkout completed.",
			"For Walmart prepare-and-open, use app_class shopping and app_named walmart with verb read. Open only — never claim cart built, ordered, or checkout completed.",
			"For Nike prepare-and-open, use app_class shopping and app_named nike with verb read. Open only — never claim cart built, ordered, or checkout completed.",
			"For Sephora prepare-and-open, use app_class shopping and app_named sephora with verb read. Open only — never claim cart built, ordered, or checkout completed.",
			"For Wayfair prepare-and-open, use app_class shopping and app_named wayfair with verb read. Open only — never claim cart built, ordered, or checkout completed.",
			// Callers: stage1openai.Model → deeplink flow; User ask: Wave1Specs 51→54 coaching for ebay/priceline/linkedin.
			"For eBay prepare-and-open, use app_class shopping and app_named ebay with verb read. Open only — never claim cart built, bid placed, or checkout completed.",
			"For Kayak prepare-and-open, use app_class travel and app_named kayak with verb read. Open only — never claim booked.",
			"For Priceline prepare-and-open, use app_class travel and app_named priceline with verb read. Open only — never claim booked.",
			"For LinkedIn prepare-and-open, use app_class messaging and app_named linkedin with verb compose. Open only — never claim the post was posted or commented.",
			"For Pinterest prepare-and-open, use app_class messaging and app_named pinterest with verb compose. Open only — never claim pinned, posted, or saved.",
			"For Duolingo prepare-and-open, use app_class services and app_named duolingo with verb read. Open only — never claim lesson completed.",
			"For Fitbit prepare-and-open, use app_class services and app_named fitbit with verb read. Open only — never claim workout logged, synced, or saved.",
			"For Shazam prepare-and-open, use app_class media and app_named shazam with verb read. Open only — never claim identified, played, or saved.",
			// Callers: stage1openai.Model → deeplink flow; User ask: Wave1Specs 58→61 coaching for chromecast/youtubemusic/soundcloud.
			"For Chromecast prepare-and-open, use app_class media and app_named chromecast with verb play (cast/open intent). Open only — never claim cast started, playing, or connected.",
			"For YouTube Music prepare-and-open, use app_class media and app_named youtubemusic with verb play (open/search title) or read (browse/search). Open only — never claim played, added to playlist, or library changed.",
			"For SoundCloud prepare-and-open, use app_class media and app_named soundcloud with verb play (open/search title) or read (browse/search). Open only — never claim played, added to playlist, or library changed.",
			// Callers: stage1openai.Model → deeplink flow; User ask: Wave1Specs 61→64 coaching for pandora/asana/trello.
			"For Pandora prepare-and-open, use app_class media and app_named pandora with verb play (open/search title) or read (browse/search). Open only — never claim played, added to playlist, or station changed.",
			"For Asana prepare-and-open, use app_class tasks and app_named asana with verb write (create/open task intent). Open only — never claim task created, assigned, or completed. Not the Todoist completes adapter.",
			"For Trello prepare-and-open, use app_class tasks and app_named trello with verb write (create/open card intent). Open only — never claim card moved, assigned, or completed. Not the Todoist completes adapter.",
			// Callers: stage1openai.Model → deeplink flow; User ask: Wave1Specs 64→67 coaching for mstodo/googledocs/dropbox.
			"For Microsoft To Do prepare-and-open, use app_class tasks and app_named mstodo with verb write (create/open task intent). Open only — never claim task created, assigned, or completed. Not the Todoist completes adapter.",
			"For Google Docs prepare-and-open, use app_class notes and app_named googledocs with verb write (draft/open doc intent). Open only — never claim doc created, saved, or shared. Separate from Google Drive OAuth.",
			"For Dropbox prepare-and-open, use app_class notes and app_named dropbox with verb read (browse/open file intent). Open only — never claim uploaded, downloaded, shared, or synced.",
			// Callers: stage1openai.Model → deeplink flow; User ask: Wave1Specs 67→70 coaching for googlesheets/evernote/googleslides.
			"For Google Sheets prepare-and-open, use app_class notes and app_named googlesheets with verb write (draft/open sheet intent). Open only — never claim sheet created, saved, shared, or synced. Separate from Google Drive OAuth and googledocs Spec.",
			"For Evernote prepare-and-open, use app_class notes and app_named evernote with verb write (draft/open note intent). Open only — never claim notebook created, saved, shared, or synced.",
			"For Google Slides prepare-and-open, use app_class notes and app_named googleslides with verb write (draft/open slides intent). Open only — never claim slide created, saved, shared, or synced. Separate from Google Drive OAuth and googledocs Spec.",
			// Callers: stage1openai.Model → deeplink flow; User ask: Wave1Specs 70→73 coaching for pocketcasts/goodreads/kindle.
			"For Pocket Casts prepare-and-open, use app_class media and app_named pocketcasts with verb play (open/search podcast) or read (browse/search). Open only — never claim played, downloaded, or subscribed. Separate from Podcasts RT-2 RSS adapter (app_named podcasts).",
			"For Goodreads prepare-and-open, use app_class notes and app_named goodreads with verb read (browse/open book intent). Open only — never claim review posted, shelved, or rated.",
			"For Kindle prepare-and-open, use app_class media and app_named kindle with verb read (open library/book intent). Open only — never claim purchased, downloaded, or read completed. Kindle reader app — not Amazon shopping.",
			// Callers: stage1openai.Model → deeplink flow; User ask: Wave1Specs 73→76 coaching for claude/chatgpt/grok.
			"For Claude prepare-and-open, use app_class messaging and app_named claude with verb compose (draft prompt / open app). Open only — never claim replied, sent, answered, or completed chat. Operator does not call Claude APIs here.",
			"For ChatGPT prepare-and-open, use app_class messaging and app_named chatgpt with verb compose (draft prompt / open app). Open only — never claim replied, sent, answered, or completed chat. Operator does not call Chat GPT APIs here.",
			"For Grok prepare-and-open, use app_class messaging and app_named grok with verb compose (draft prompt / open app). Open only — never claim replied, sent, answered, or completed chat. Operator does not call Grok APIs here.",
			"For OpenTable prepare-and-open, use app_class food and app_named opentable with verb read. Open only — never claim a reservation was booked.",
			"For Grubhub prepare-and-open, use app_class food and app_named grubhub with verb read (browse) or order (cart intent). Open only — never claim checkout completed.",
			"Never invent account identifiers, contact details, task IDs, or other resolved handles.",
			"When a request names more than one thing subject alone cannot hold, put each named thing in fields under a short label — for a directions request, origin and destination. No field may name or contain a contact handle: no phone number, email, thread id, or contact name in a field's key or value.",
			"Fill a fields slot only when the user actually named that value in their request; otherwise send it as null rather than guessing.",
			"Return only the requested schema.",
		}, " "),
		Input: utterance,
		Text:  textConfig{Format: routeFormat()},
	}
	var encoded strings.Builder
	if err := json.NewEncoder(&encoded).Encode(request); err != nil {
		return nil, fmt.Errorf("openai stage1: encode request: %w", err)
	}
	path := c.responsesPath
	if path == "" {
		path = "/v1/responses"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(encoded.String()))
	if err != nil {
		return nil, fmt.Errorf("openai stage1: build request: %w", err)
	}
	// Brokered clients leave apiKey empty so the Android vault adds the bearer.
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	c.logger.Info("[stage1-openai] request", "model", Model, "path", path, "utterance_bytes", len(utterance), "reasoning_effort", "none", "brokered", c.apiKey == "")
	var responseBody []byte
	var statusCode int
	if c.responsesPath != "" {
		// Termux/proot: in-process Go HTTP to the Android loopback broker can
		// stall ~60s mid-body while a WebSocket is open. curl in a subprocess
		// finishes the same POST in ~1–3s (verified on-device during dogfood).
		c.logger.Info("[stage1-openai] broker curl post", "body_len", encoded.Len())
		var curlErr error
		statusCode, responseBody, curlErr = curlBrokerPOST(ctx, c.baseURL, path, []byte(encoded.String()), 60*time.Second)
		if curlErr != nil {
			c.logger.Error("[stage1-openai] request failed", "model", Model, "path", path, "error", curlErr)
			return nil, fmt.Errorf("%w: %v", ErrRouterUnreachable, curlErr)
		}
	} else {
		response, err := c.http.Do(req)
		if err != nil {
			c.logger.Error("[stage1-openai] request failed", "model", Model, "path", path, "error", err)
			return nil, fmt.Errorf("openai stage1: request failed: %w", err)
		}
		defer response.Body.Close()
		statusCode = response.StatusCode
		responseBody, _ = io.ReadAll(io.LimitReader(response.Body, 1<<20))
	}
	if statusCode < 200 || statusCode >= 300 {
		c.logger.Error("[stage1-openai] response rejected", "model", Model, "path", path, "status", statusCode, "body_len", len(responseBody))
		if c.responsesPath != "" {
			if statusCode == http.StatusServiceUnavailable && strings.Contains(string(responseBody), "no_key") {
				return nil, ErrRouterNotProvisioned
			}
			return nil, fmt.Errorf("%w: status %d", ErrRouterUnreachable, statusCode)
		}
		return nil, fmt.Errorf("openai stage1: Responses API returned status %d", statusCode)
	}
	var decoded responseEnvelope
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return nil, fmt.Errorf("openai stage1: decode response: %w", err)
	}
	var outputs []string
	for _, item := range decoded.Output {
		if item.Type != "message" || item.Role != "assistant" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" {
				outputs = append(outputs, content.Text)
			}
		}
	}
	if len(outputs) != 1 || strings.TrimSpace(outputs[0]) == "" {
		return nil, fmt.Errorf("openai stage1: response contained %d output_text blocks, want exactly one", len(outputs))
	}
	c.logger.Info("[stage1-openai] response", "model", Model, "input_tokens", decoded.Usage.InputTokens, "output_tokens", decoded.Usage.OutputTokens, "total_tokens", decoded.Usage.TotalTokens)
	return []byte(outputs[0]), nil
}

type responseRequest struct {
	Model        string          `json:"model"`
	Store        bool            `json:"store"`
	Reasoning    reasoningConfig `json:"reasoning"`
	Instructions string          `json:"instructions"`
	Input        string          `json:"input"`
	Text         textConfig      `json:"text"`
}

type reasoningConfig struct {
	Effort string `json:"effort"`
}

type textConfig struct {
	Format map[string]any `json:"format"`
}

// namedSlots lists the fixed set of named slots the model may fill in
// fields. These are exactly the slots adapters read via Fields[...]: maps
// reads origin/destination/navigate, apple reminders reads list, apple
// notes reads folder, Beeper manage ops read operation. page_id (notion)
// and feed_url (podcasts) are app-internal ids a cloud model has no
// business guessing, so they are not offered here.
var namedSlots = []string{"origin", "destination", "navigate", "list", "folder", "operation"}

func routeFormat() map[string]any {
	slotProperties := make(map[string]any, len(namedSlots))
	for _, slot := range namedSlots {
		slotProperties[slot] = map[string]any{"type": []string{"string", "null"}}
	}
	fields := map[string]any{
		"type": "object", "properties": slotProperties,
		"required":             append([]string(nil), namedSlots...),
		"additionalProperties": false,
	}

	properties := map[string]any{
		"verb": map[string]any{
			"type": "string", "enum": []string{"read", "compose", "send", "order", "book", "play", "write", "cancel", "modify"},
		},
		"app_class": map[string]any{"type": "string"},
		"app_named": map[string]any{"type": "string"},
		"subject":   map[string]any{"type": "string"},
		"body":      map[string]any{"type": "string"},
		"fields":    fields,
		"confidence": map[string]any{
			"type": "number", "minimum": 0, "maximum": 1,
		},
	}
	return map[string]any{
		"type": "json_schema", "name": "operator_route", "strict": true,
		"schema": map[string]any{
			"type": "object", "properties": properties,
			"required":             []string{"verb", "app_class", "app_named", "subject", "body", "fields", "confidence"},
			"additionalProperties": false,
		},
	}
}

type responseEnvelope struct {
	Output []struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}
