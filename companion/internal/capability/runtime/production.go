package runtime

import (
	"errors"
	"log/slog"
	"strings"
	"time"

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

// oauthReason explains, in plain English, why Spotify, Todoist, Slack,
// Google Calendar/Drive, Outlook, and Teams are never registered here even
// when their client id and secret are present in the environment. Every one
// of them needs a real user sign-in: a browser sent to a consent screen and
// a loopback callback listener that blocks until that round trip finishes.
// That flow exists today only inside the owner-only proof commands
// (serve-spotify-proof and its siblings), which take an io.Writer to print
// the sign-in link to and hold the process open while a person clicks
// through it. Plain `serve` starts unattended on a machine nobody is
// watching, so there is no moment to run that flow, and there is no stored,
// refreshable token sitting around to load instead — persisting a token
// across restarts is future work, not something to fake here by pretending
// a client id is as good as a signed-in user. Registering one of these
// adapters anyway would let the router choose it, let the user confirm a
// preview, and only then discover — after they had already said yes — that
// there was never a credential behind it.
const oauthReason = "requires an interactive OAuth sign-in (browser + loopback callback) that only exists in the owner-only proof commands today; unattended serve has no moment to run it and no persisted refresh token to load instead"

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
	// YouTubeAPIKey is the YouTube Data API v3 key (YOUTUBE_API_KEY).
	YouTubeAPIKey string
	// PodcastsFeedURL is the one RSS feed the Podcasts adapter reads
	// (PODCASTS_FEED_URL). There is no partner account to connect — the
	// feed URL itself is the only thing standing between "nothing to
	// search" and a working adapter.
	PodcastsFeedURL string
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
	youtubeCredentialed := youtubeAPIKey != ""

	// The credential-free deep-link pack is the floor every install gets,
	// signed in or not: it can only open an app and hand it a prepared
	// draft, never search or read on the user's behalf, but it needs
	// nothing from anyone to do that much. Register all of Wave1Specs
	// except the one id a credentialed adapter below is about to take
	// over — the registry refuses two adapters under the same id, and the
	// credentialed version can do strictly more than the hand-off, so it
	// should win rather than sit unregistered beside it.
	for _, spec := range deeplinkadapter.Wave1Specs() {
		if spec.ID == youtubeadapter.ID && youtubeCredentialed {
			continue
		}
		if err := reg.Register(deeplinkadapter.New(spec, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, spec.ID)
		byClass[spec.AppClass] = append(byClass[spec.AppClass], spec.ID)
	}

	// Instagram never needed an account credential in the first place —
	// it prepares a draft and opens the app, nothing more — so it is
	// unconditional, the same as the deep-link pack.
	if err := reg.Register(instagramadapter.New(logger)); err != nil {
		return nil, Inventory{}, err
	}
	inv.Registered = append(inv.Registered, instagramadapter.ID)
	byClass["messaging"] = append(byClass["messaging"], instagramadapter.ID)

	// Notification reply needs no credential either — it never talks to a
	// service at all, it hands the reply to the phone. Unconditional for the
	// same reason Instagram is: there is no key or sign-in that could ever
	// be missing.
	if err := reg.Register(notificationreplyadapter.New(logger)); err != nil {
		return nil, Inventory{}, err
	}
	inv.Registered = append(inv.Registered, notificationreplyadapter.ID)
	byClass[notificationreplyadapter.Class] = append(byClass[notificationreplyadapter.Class], notificationreplyadapter.ID)

	// Maps: a Places/Routes API key is the only connection, no OAuth. With
	// no key the adapter would fail the moment anyone asked it to look
	// something up, so it is left out entirely rather than registered to
	// fail after a preview has already been confirmed.
	mapsAPIKey := strings.TrimSpace(config.MapsAPIKey)
	if mapsAPIKey == "" {
		inv.Skipped[mapsadapter.ID] = "no GOOGLE_MAPS_API_KEY configured"
	} else {
		client := mapsadapter.NewHTTPClient("", "", mapsAPIKey, nil, logger)
		if err := reg.Register(mapsadapter.New(client, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, mapsadapter.ID)
		byClass["travel"] = append(byClass["travel"], mapsadapter.ID)
	}

	// YouTube: a Data API v3 key is the only connection, no OAuth. Without
	// it, the deep-link hand-off registered above already covers "open
	// YouTube" — this only adds real search-and-play on top.
	if !youtubeCredentialed {
		inv.Skipped[youtubeadapter.ID] = "no YOUTUBE_API_KEY configured; the deep-link hand-off still covers opening the app"
	} else {
		client := youtubeadapter.NewHTTPClient("", youtubeAPIKey, nil, logger)
		if err := reg.Register(youtubeadapter.New(client, logger)); err != nil {
			return nil, Inventory{}, err
		}
		inv.Registered = append(inv.Registered, youtubeadapter.ID)
		byClass["entertainment"] = append(byClass["entertainment"], youtubeadapter.ID)
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

	// Every adapter that needs the interactive OAuth sign-in described in
	// oauthReason. There is no environment variable that substitutes for a
	// person clicking through a consent screen, so these are always left out
	// of an unattended production build.
	//
	// This list has to match what the adapters themselves declare
	// (Auth: AuthOAuth). Nothing here can check that on its own, because
	// these adapters are never constructed in this build and an unbuilt
	// adapter has no manifest to ask. signin_accounted_for_test.go does the
	// checking instead: it builds every adapter in the repo, reads what each
	// declared, and fails if one that needs a sign-in is missing from this
	// map. Notion was missing from it until that test was written, so it
	// appeared in neither Registered nor Skipped and the inventory simply
	// did not mention it.
	for id, reason := range map[string]string{
		todoistadapter.ID: oauthReason,
		slackadapter.ID:   oauthReason,
		gcalendar.ID:      oauthReason,
		gdrive.ID:         oauthReason,
		outlook.ID:        oauthReason,
		msteams.ID:        oauthReason,
		notionadapter.ID:  oauthReason,

		// Spotify is the one id two different adapters answer to: this
		// full-control one, and a hand-off that only opens the app. The
		// registry is keyed by id, so the hand-off is what actually ships
		// until the sign-in exists — the demotion to class H the
		// authorization gate describes. Saying only "skipped, needs OAuth"
		// here would contradict the same id appearing in Registered, and
		// leave whoever reads the inventory unable to tell which of the two
		// the user actually gets.
		spotifyadapter.ID: oauthReason +
			"; the hand-off adapter of the same id is registered in its place, so Spotify still opens on the phone — it just cannot be driven",
	} {
		inv.Skipped[id] = reason
	}

	classes := classMapFor(byClass)
	inv.Classes = byClass
	inv.reg = reg
	resolver := stage2.New(reg, contacts.NewGraph(time.Now), classes, manifest.PlatformAndroid)
	runner := execution.New(reg)
	inv.tel = runner.Telemetry()

	logger.Info("[capability-runtime] production flow ready",
		"registered_count", len(inv.Registered),
		"skipped_count", len(inv.Skipped),
		"class_count", len(classes),
		"platform", manifest.PlatformAndroid,
	)

	return flow.New(stage1.New(config.Model), resolver, runner, ConsentGate(), logger), inv, nil
}
