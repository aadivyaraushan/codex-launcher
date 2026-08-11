package runtime

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	// Fact-force: callers=NewProduction maps switch; API=MapsBrokerBaseURL;
	// user: "Maps Go→Android Places/Routes RPC"
	mapsbroker "github.com/codex-launcher/codex-launcher/companion/internal/androidbroker/maps"
	beepermessage "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/beepermessage"
	deeplinkadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/deeplink"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gcalendar"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gdrive"
	instagramadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/instagram"
	mapsadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/maps"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/msteams"
	notificationreplyadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/notificationreply"
	notionadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/notion"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/outlook"
	podcastsadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/podcasts"
	slackadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/slack"
	spotifyadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/spotify"
	todoistadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/todoist"
	youtubeadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/youtube"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/consent"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/verification"
)

// ErrMissingProductionDependency is returned when NewProduction has nothing
// to route with. A companion that starts up with no model cannot classify
// any request, so it must refuse at startup rather than come up looking
// healthy and refuse every request later, one at a time, forever.
var ErrMissingProductionDependency = errors.New("capability runtime: a routing model is required")

// classAddressing is the complete, named declaration of how every class
// production actually serves is addressed — not a shortlist of exceptions
// with a fallback for the rest. There is deliberately no default in
// classMapFor for a class missing from this map: falling back to ToAThing
// would be the same mistake this whole fix removes, just moved one level
// up — the next person who adds a class addressed to a person would get a
// silent, wrong ToAThing instead of a silent, wrong "false", and nothing
// would force them to make the call. Leaving an unlisted class at the zero
// value instead means Resolve refuses it by name the first time anyone
// asks, which is what makes adding the entry here mandatory rather than
// optional.
var classAddressing = map[string]stage2.Addressing{
	"beeper_messaging":             stage2.ResolvedByAdapter,
	"calendar":                     stage2.ToAThing,
	"drive":                        stage2.ToAThing,
	"email":                        stage2.ToAThing,
	"music":                        stage2.ToAThing,
	"slack":                        stage2.ToAThing,
	"messaging":                    stage2.ToAPerson,
	"money":                        stage2.ToAPerson,
	notificationreplyadapter.Class: stage2.ResolvedOnTheDevice,
	"entertainment":                stage2.ToAThing,
	"finance":                      stage2.ToAThing,
	"food":                         stage2.ToAThing,
	"media":                        stage2.ToAThing,
	"notes":                        stage2.ToAThing,
	"rides":                        stage2.ToAThing,
	"services":                     stage2.ToAThing,
	"shopping":                     stage2.ToAThing,
	"tasks":                        stage2.ToAThing,
	"travel":                       stage2.ToAThing,
}

// classMapFor turns the class-to-adapter-ids map NewProduction built while
// registering adapters into the declared ClassMap stage 2 needs. A class
// present in classAddressing gets exactly the Addressing declared there; a
// class that shows up in byClass but was never added to classAddressing is
// left at its zero value, AddressingUndeclared, so stage 2 refuses it
// rather than guessing. It is a package-level function, not inlined into
// NewProduction, so a test in this package can hold it against the
// router's rules directly instead of standing up the whole production flow
// to check one map.
func classMapFor(byClass map[string][]string) stage2.ClassMap {
	classes := stage2.ClassMap{}
	for class, ids := range byClass {
		classes[class] = stage2.Class{Adapters: ids, Addressing: classAddressing[class]}
	}
	return classes
}

// oauthReason is recorded when no complete, refreshable user connection was
// loaded at startup. A client id or secret alone is never treated as a signed
// in account.
const oauthReason = "no complete refreshable OAuth connection was loaded from macOS Keychain"

// ConsentGate builds the gate a runtime hands its flow. Every adapter that
// ships today is consent class A, which passes with nothing granted, so
// there is no registered copy and no vault to revoke against yet. The gate
// is wired anyway so the first class B adapter cannot ship with a
// decorative consent screen.
func ConsentGate() *consent.Store {
	return consent.New(time.Now, nil, consent.NoVault{}, consent.NoVault{})
}

// ProductionConfig names every credential the plain, unattended `serve`
// command can plausibly supply from its own environment. Each credential
// here unlocks exactly one adapter that needs nothing more than an API key
// or a feed URL to work — no human has to be watching a browser for these.
// Adapters that need a real OAuth sign-in are not configurable here at all;
// see oauthReason above.
type ProductionConfig struct {
	Model  stage1.ModelFunc
	Logger *slog.Logger

	// MapsAPIKey is the Google Places/Routes API key (GOOGLE_MAPS_API_KEY).
	MapsAPIKey string
	// MapsBrokerBaseURL is the Android loopback Maps broker (e.g.
	// http://127.0.0.1:9451). Used when MapsAPIKey is empty so the key can
	// stay in the Android Keystore vault.
	MapsBrokerBaseURL string
	// YouTubeAPIKey is the YouTube Data API v3 key (YOUTUBE_API_KEY).
	YouTubeAPIKey string
	// PodcastsFeedURL is the one RSS feed the Podcasts adapter reads
	// (PODCASTS_FEED_URL). There is no partner account to connect — the
	// feed URL itself is the only thing standing between "nothing to
	// search" and a working adapter.
	PodcastsFeedURL string
	// Fact-force (edit): callers=cmd/codex-launcher/production.go,
	// phoneruntime/runtime.go, beeper_stage2_readonly_test.go; existing file
	// production.go (not new). User: "Continue OpenAI+Beeper — SLICE 4: B4 + B5".
	// BeeperAPI is present when the local or remote Beeper target is
	// authenticated. It replaces the Instagram, Discord, and Google Messages
	// hand-offs with Beeper network adapters under beeper_messaging.
	BeeperAPI beepermessage.API
	// BeeperReadOnly keeps those adapters registered but with Verbs:[read]
	// only. Callers should also pass a client.ReadOnly() API so writes are
	// refused at the HTTP layer.
	BeeperReadOnly bool
	// Disconnected is the restart-safe set of adapters the user turned off.
	// PersistDisconnect records a new disconnect before the live adapter is removed.
	Disconnected      map[string]bool
	PersistDisconnect func(context.Context, string) error
	// OAuth APIs are constructed by serve only when a complete, refreshable
	// token record was loaded from macOS Keychain.
	GoogleCalendarAPI gcalendar.API
	GoogleDriveAPI    gdrive.API
	SlackAPI          slackadapter.API
	OutlookAPI        outlook.API
	SpotifyAPI        spotifyadapter.API
	// NotionAdapter is already authenticated and connected by the startup
	// path, which measures the workspace's live MCP tools before registration.
	NotionAdapter *notionadapter.Adapter
}

// Inventory is the honest record of what NewProduction actually built: what
// went in, what was left out and why, and what the router can actually
// reach. It exists so a build that quietly drops an adapter, or quietly
// leaves one unreachable, shows up in a test instead of in a support
// ticket.
type Inventory struct {
	// Registered is every adapter id actually placed in the registry.
	Registered []string
	// Skipped maps an adapter id that was left out to the plain-English
	// reason it was left out.
	Skipped map[string]string
	// Classes is the same class-to-adapters map handed to the stage 2
	// resolver, so a test can check that everything registered is
	// reachable and everything reachable was registered.
	Classes map[string][]string

	// reg is the registry these adapters went into. It stays unexported
	// because nothing outside this package should reach past the flow to
	// the adapters — the one caller is the contract suite next door, which
	// needs the live adapters rather than their ids so it can hold every
	// one of them against the same rules.
	reg *registry.Registry

	// tel is the execution runner's own live Telemetry — the one every real
	// request actually feeds. It stays unexported for the same reason reg
	// does; AdapterIDs and Report below are the only doors the world outside
	// this package gets onto it.
	tel *verification.Telemetry

	// runner is the same execution.Runner the flow itself was built with. It
	// stays unexported for the same reason reg does; Runner below is the
	// one door the world outside this package gets onto it.
	runner *execution.Runner
}

// Runner returns the execution.Runner this inventory's flow was built with,
// so a second door onto the registry — the phone-runtime agent bridge,
// today — drives the exact same adapters and telemetry rather than a second,
// disconnected runner that would never see real traffic.
func (i Inventory) Runner() *execution.Runner {
	return i.runner
}

// ApplyKillList switches adapters on and off to match a remote list. It is
// the one operation the world outside this package gets on the registry:
// the kill switch has to reach the adapters, and nothing else does.
func (i Inventory) ApplyKillList(list registry.KillList) []string {
	return i.reg.ApplyKillList(list)
}

// AdapterIDs returns the id of every adapter this inventory registered.
// Together with Report below, it makes Inventory itself satisfy
// alerts.Source — the alert watcher has to enumerate what to check, and
// this is the one door onto the registry that lets it.
func (i Inventory) AdapterIDs() []string {
	return i.reg.AdapterIDs()
}

// Report returns what real production traffic has shown about one adapter,
// read straight from the live Telemetry the execution runner has been
// feeding on every request — not a second, empty one that would never see
// a real observation.
func (i Inventory) Report(adapterID string) (verification.Report, error) {
	return i.tel.Report(adapterID)
}

// NewProduction builds the one capability flow the plain `serve` command
// runs with. Earlier, `serve` never built a flow at all: every adapter in
// this repo was reachable only from its own owner-only `serve-<name>-proof`
// command, so an ordinary phone talking to an ordinary `serve` companion had
// its capability requests refused outright by handler.go's nil check. This
// is the fix — one shared registry and one shared router class map spanning
// every adapter that can come up unattended, plus an honest Inventory
// explaining what did not make it in.
func NewProduction(config ProductionConfig) (*flow.Service, Inventory, error) {
	if config.Model == nil {
		return nil, Inventory{}, ErrMissingProductionDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	logger := config.Logger

	reg := registry.New()
	inv := Inventory{Skipped: map[string]string{}, Classes: map[string][]string{}}
	byClass := map[string][]string{}

	youtubeAPIKey := strings.TrimSpace(config.YouTubeAPIKey)
	youtubeCredentialed := youtubeAPIKey != "" && !config.Disconnected[youtubeadapter.ID]

	// The credential-free deep-link pack is the floor every install gets,
	// signed in or not: it can only open an app and hand it a prepared
	// draft, never search or read on the user's behalf, but it needs
	// nothing from anyone to do that much. Register all of Wave1Specs
	// except the one id a credentialed adapter below is about to take
	// over — the registry refuses two adapters under the same id, and the
	// credentialed version can do strictly more than the hand-off, so it
	// should win rather than sit unregistered beside it.
	beeperSpecs := beepermessage.ProductionSpecs()
	beeperIDs := map[string]bool{}
	if config.BeeperAPI != nil {
		for _, spec := range beeperSpecs {
			if !config.Disconnected[spec.ID] {
				beeperIDs[spec.ID] = true
			}
		}
	}
	credentialedIDs := map[string]bool{}
	if config.GoogleCalendarAPI != nil {
		credentialedIDs[gcalendar.ID] = true
	}
	if config.GoogleDriveAPI != nil {
		credentialedIDs[gdrive.ID] = true
	}
	if config.SlackAPI != nil {
		credentialedIDs[slackadapter.ID] = true
	}
	if config.OutlookAPI != nil {
		credentialedIDs[outlook.ID] = true
	}
	if config.SpotifyAPI != nil {
		credentialedIDs[spotifyadapter.ID] = true
	}
	if config.NotionAdapter != nil {
		credentialedIDs[notionadapter.ID] = true
	}
	for _, spec := range deeplinkadapter.Wave1Specs() {
		if config.Disconnected[spec.ID] {
			continue
		}
		if spec.ID == youtubeadapter.ID && youtubeCredentialed {
			continue
		}
		if beeperIDs[spec.ID] {
			continue
		}
		if credentialedIDs[spec.ID] {
			continue
		}
		if err := reg.Register(deeplinkadapter.New(spec, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, spec.ID)
		byClass[spec.AppClass] = append(byClass[spec.AppClass], spec.ID)
	}

	if config.BeeperAPI == nil {
		// Without Beeper, Instagram keeps its narrow draft-and-open floor.
		if err := reg.Register(instagramadapter.New(logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, instagramadapter.ID)
		byClass["messaging"] = append(byClass["messaging"], instagramadapter.ID)
	} else {
		for _, spec := range beeperSpecs {
			if config.Disconnected[spec.ID] {
				inv.Skipped[spec.ID] = "disconnected by the user"
				continue
			}
			adapterID := spec.ID
			revoke := func(ctx context.Context) error {
				if config.PersistDisconnect == nil {
					return nil
				}
				return config.PersistDisconnect(ctx, adapterID)
			}
			var beeperAdapter *beepermessage.Adapter
			if config.BeeperReadOnly {
				beeperAdapter = beepermessage.NewReadOnlyWithRevoke(spec, config.BeeperAPI, revoke, logger)
			} else {
				beeperAdapter = beepermessage.NewWithRevoke(spec, config.BeeperAPI, revoke, logger)
			}
			if err := reg.Register(beeperAdapter); err != nil {
				return nil, Inventory{}, err
			}
			inv.Registered = append(inv.Registered, spec.ID)
			byClass["beeper_messaging"] = append(byClass["beeper_messaging"], spec.ID)
		}
	}

	// Notification reply needs no credential either — it never talks to a
	// service at all, it hands the reply to the phone. Unconditional for the
	// same reason Instagram is: there is no key or sign-in that could ever
	// be missing.
	if err := reg.Register(notificationreplyadapter.New(logger)); err != nil {
		return nil, Inventory{}, err
	}
	inv.Registered = append(inv.Registered, notificationreplyadapter.ID)
	byClass[notificationreplyadapter.Class] = append(byClass[notificationreplyadapter.Class], notificationreplyadapter.ID)

	// Maps: Places/Routes via Linux API key, or Android Keystore broker over
	// loopback (phone-runtime). With neither, leave it out rather than fail
	// after a preview has already been confirmed.
	mapsAPIKey := strings.TrimSpace(config.MapsAPIKey)
	mapsBrokerURL := strings.TrimSpace(config.MapsBrokerBaseURL)
	switch {
	case mapsAPIKey != "":
		client := mapsadapter.NewHTTPClient("", "", mapsAPIKey, nil, logger)
		if err := reg.Register(mapsadapter.New(client, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, mapsadapter.ID)
		byClass["travel"] = append(byClass["travel"], mapsadapter.ID)
	case mapsBrokerURL != "":
		client := mapsbroker.NewClient(mapsBrokerURL, nil, logger)
		if err := reg.Register(mapsadapter.New(client, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, mapsadapter.ID)
		byClass["travel"] = append(byClass["travel"], mapsadapter.ID)
	default:
		inv.Skipped[mapsadapter.ID] = "no GOOGLE_MAPS_API_KEY or Android maps broker URL configured"
	}

	// YouTube: a Data API v3 key is the only connection, no OAuth. Without
	// it, the deep-link hand-off registered above already covers "open
	// YouTube" — this only adds real search-and-play on top.
	if !youtubeCredentialed {
		if config.Disconnected[youtubeadapter.ID] {
			inv.Skipped[youtubeadapter.ID] = "disconnected by the user; the deep-link hand-off still covers opening the app"
		} else {
			inv.Skipped[youtubeadapter.ID] = "no YOUTUBE_API_KEY configured; the deep-link hand-off still covers opening the app"
		}
	} else {
		client := youtubeadapter.NewHTTPClient("", youtubeAPIKey, nil, logger)
		revoke := func(ctx context.Context) error {
			if config.PersistDisconnect == nil {
				return nil
			}
			return config.PersistDisconnect(ctx, youtubeadapter.ID)
		}
		if err := reg.Register(youtubeadapter.NewWithRevoke(client, revoke, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, youtubeadapter.ID)
		byClass["media"] = append(byClass["media"], youtubeadapter.ID)
	}

	// Podcasts: a plain RSS feed URL is the only connection, no OAuth and
	// no partner API. With no feed configured there is nothing to search,
	// so it is left out.
	feedURL := strings.TrimSpace(config.PodcastsFeedURL)
	if feedURL == "" {
		inv.Skipped[podcastsadapter.ID] = "no PODCASTS_FEED_URL configured"
	} else {
		a := podcastsadapter.New(podcastsadapter.FeedConfig{URL: feedURL}, podcastsadapter.NewHTTPClient(nil, logger), logger)
		if err := reg.Register(a); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, podcastsadapter.ID)
		byClass["media"] = append(byClass["media"], podcastsadapter.ID)
	}

	if config.GoogleCalendarAPI != nil {
		if err := reg.Register(gcalendar.New(config.GoogleCalendarAPI, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, gcalendar.ID)
		byClass["calendar"] = append(byClass["calendar"], gcalendar.ID)
	} else {
		inv.Skipped[gcalendar.ID] = oauthReason
	}
	if config.GoogleDriveAPI != nil {
		if err := reg.Register(gdrive.New(config.GoogleDriveAPI, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, gdrive.ID)
		byClass["drive"] = append(byClass["drive"], gdrive.ID)
	} else {
		inv.Skipped[gdrive.ID] = oauthReason
	}
	if config.SlackAPI != nil {
		if err := reg.Register(slackadapter.New(config.SlackAPI, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, slackadapter.ID)
		byClass["slack"] = append(byClass["slack"], slackadapter.ID)
	} else {
		inv.Skipped[slackadapter.ID] = oauthReason
	}
	if config.OutlookAPI != nil {
		if err := reg.Register(outlook.New(config.OutlookAPI, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, outlook.ID)
		byClass["email"] = append(byClass["email"], outlook.ID)
	} else {
		inv.Skipped[outlook.ID] = oauthReason
	}
	if config.SpotifyAPI != nil {
		if err := reg.Register(spotifyadapter.New(config.SpotifyAPI, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, spotifyadapter.ID)
		byClass["media"] = append(byClass["media"], spotifyadapter.ID)
	} else {
		inv.Skipped[spotifyadapter.ID] = oauthReason +
			"; the hand-off adapter of the same id is registered in its place, so Spotify still opens on the phone — it just cannot be driven"
	}
	if config.NotionAdapter != nil {
		if err := reg.Register(config.NotionAdapter); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, notionadapter.ID)
		byClass["notes"] = append(byClass["notes"], notionadapter.ID)
	} else {
		inv.Skipped[notionadapter.ID] = oauthReason
	}

	// These remaining adapters still have no persisted production connection.
	//
	// This list has to match what the adapters themselves declare
	// (Auth: AuthOAuth). Nothing here can check that on its own, because
	// these adapters are never constructed in this build and an unbuilt
	// adapter has no manifest to ask. signin_accounted_for_test.go does the
	// checking instead: it builds every adapter in the repo, reads what each
	// declared, and fails if one that needs a sign-in is missing from either
	// production wiring or this map.
	for id, reason := range map[string]string{
		todoistadapter.ID: oauthReason,
		msteams.ID:        oauthReason,
	} {
		inv.Skipped[id] = reason
	}

	classes := classMapFor(byClass)
	inv.Classes = byClass
	inv.reg = reg
	resolver := stage2.New(reg, contacts.NewGraph(time.Now), classes, manifest.PlatformAndroid)
	runner := execution.New(reg)
	inv.tel = runner.Telemetry()
	inv.runner = runner

	logger.Info("[capability-runtime] production flow ready",
		"registered_count", len(inv.Registered),
		"skipped_count", len(inv.Skipped),
		"class_count", len(classes),
		"platform", manifest.PlatformAndroid,
	)

	return flow.New(stage1.New(config.Model), resolver, runner, ConsentGate(), logger), inv, nil
}
