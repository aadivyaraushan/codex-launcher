// Package liveconnection holds the OAuth checks that talk to the real
// providers instead of a fake server.
//
// Why this package exists: every other OAuth test in this tree points at an
// httptest server, so all of them pass even when the stored client id and
// secret are wrong, or when a provider quietly moves an endpoint. The two
// live tests that did exist (google, slack) only fetched the *authorize*
// page, which never uses the client secret at all — so a broken secret sailed
// through them.
//
// These run only when OAUTH_LIVE=1 is set, because they need the network.
// The skip message names what goes untested so a green run never reads as
// "the providers are wired up".
//
// Nothing here spends money and nothing here needs a human at a browser.
// See saved-results/what-oauth-can-be-tested-without-the-owner.md for the
// measured behaviour each assertion is built on, including the two providers
// (Microsoft, Slack) whose secrets provably cannot be checked this way.
package liveconnection

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/google"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/microsoft"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/todoist"
)

const liveGate = "OAUTH_LIVE"

func requireLive(t *testing.T, untested string) {
	t.Helper()
	if os.Getenv(liveGate) != "1" {
		t.Skipf("%s=1 not set, so this is UNTESTED: %s", liveGate, untested)
	}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func liveClient() *http.Client {
	return &http.Client{
		Timeout: 25 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// postForm sends a form and returns the status plus the decoded OAuth error
// fields. It never returns a body, so a token can't reach the test log.
func postForm(t *testing.T, endpoint string, form url.Values) (int, string, string, bool) {
	t.Helper()
	resp, err := liveClient().PostForm(endpoint, form)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16384))
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var decoded struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("response from %s was not JSON (status %d)", endpoint, resp.StatusCode)
	}
	return resp.StatusCode, decoded.Error, decoded.Description, decoded.AccessToken != ""
}

// TestGoogleAcceptsTheStoredClientIDAndSecret proves the stored Google
// credentials are real, without a browser and without a user.
//
// The trick: exchange a code that is deliberately invalid. Google checks the
// client before the code, so the error it picks is the answer —
// "invalid_grant" means the client was accepted and only the code was bad;
// "invalid_client" means the credentials themselves are wrong.
func TestGoogleAcceptsTheStoredClientIDAndSecret(t *testing.T) {
	requireLive(t, "whether the stored Google client id and secret still work")
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	redirect := os.Getenv("GOOGLE_REDIRECT_URI")
	if clientID == "" || clientSecret == "" || redirect == "" {
		t.Skip("UNTESTED: GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET / GOOGLE_REDIRECT_URI absent")
	}

	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirect},
		"grant_type":    {"authorization_code"},
		"code":          {"deliberately-invalid-probe-code"},
	}
	status, oauthErr, desc, gotToken := postForm(t, "https://oauth2.googleapis.com/token", form)
	if gotToken {
		t.Fatalf("an invalid code minted a token, which should be impossible (status %d)", status)
	}
	if oauthErr == "invalid_client" {
		t.Fatalf("Google rejected the stored credentials: %s (%s). "+
			"Re-issue the client secret in the Cloud console and update GOOGLE_CLIENT_SECRET.", oauthErr, desc)
	}
	if oauthErr != "invalid_grant" {
		t.Fatalf("expected invalid_grant (credentials fine, code bad), got %q: %s", oauthErr, desc)
	}
	t.Logf("google credentials accepted: status=%d error=%s", status, oauthErr)
}

// TestSpotifyMintsARealAppTokenFromTheStoredCredentials is the strongest
// credential check available anywhere in this tree: Spotify's
// client-credentials grant hands back a real token for a valid pair and
// "invalid_client" for a bad one, with no user and no consent screen.
//
// It is needed because Spotify's authorize page validates nothing before
// login — a garbage client id and a garbage redirect both get the same
// redirect to the sign-in page — so the authorize-page style of check used
// for Google and Slack can never fail here.
func TestSpotifyMintsARealAppTokenFromTheStoredCredentials(t *testing.T) {
	requireLive(t, "whether the stored Spotify client id and secret still work")
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		t.Skip("UNTESTED: SPOTIFY_CLIENT_ID / SPOTIFY_CLIENT_SECRET absent")
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://accounts.spotify.com/api/token",
		strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.SetBasicAuth(clientID, clientSecret)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := liveClient().Do(request)
	if err != nil {
		t.Fatalf("POST token: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16384))
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var decoded struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("response was not JSON (status %d)", resp.StatusCode)
	}
	if decoded.Error != "" {
		t.Fatalf("Spotify rejected the stored credentials: %s (%s). "+
			"Re-issue them in the Spotify developer dashboard.", decoded.Error, decoded.Description)
	}
	if resp.StatusCode != http.StatusOK || decoded.AccessToken == "" || decoded.TokenType != "Bearer" {
		t.Fatalf("expected a Bearer token, got status=%d token_type=%q token_present=%t",
			resp.StatusCode, decoded.TokenType, decoded.AccessToken != "")
	}
	// Length only. The token itself never reaches the log.
	t.Logf("spotify credentials accepted: token_len=%d expires_in=%d", len(decoded.AccessToken), decoded.ExpiresIn)
}

// TestTodoistRegistersItsOwnClientWithNoOwnerSetup runs the whole Todoist
// start flow against the real service.
//
// Todoist is the one provider that needs nothing from the owner: it supports
// dynamic client registration, so the app registers itself at run time and
// uses PKCE instead of a secret. That means this test proves the real code
// path end to end, up to the point a human has to sign in.
func TestTodoistRegistersItsOwnClientWithNoOwnerSetup(t *testing.T) {
	requireLive(t, "whether Todoist still hands out a client id at run time")

	const redirect = "http://127.0.0.1:9187/callback"
	flow := todoist.New(todoist.Config{
		ClientName: "Operator",
		HTTPClient: liveClient(),
		Logger:     quietLogger(),
	})

	auth, err := flow.Start(context.Background(), redirect, []manifest.Verb{manifest.Read, manifest.Write})
	if err != nil {
		t.Fatalf("Start against the real Todoist API: %v", err)
	}
	parsed, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	query := parsed.Query()
	if query.Get("client_id") == "" {
		t.Fatal("registration returned no client id")
	}
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Fatalf("PKCE missing: method=%q challenge_present=%t",
			query.Get("code_challenge_method"), query.Get("code_challenge") != "")
	}

	resp, err := liveClient().Get(auth.URL)
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16384))
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	// Todoist answers a good client with a redirect to its sign-in page, and a
	// bad one with a JSON error naming the missing or unknown argument.
	combined := strings.ToLower(string(body) + " " + resp.Header.Get("Location"))
	for _, marker := range []string{"argument_missing", "error_code", "invalid_client", "unauthorized"} {
		if strings.Contains(combined, marker) {
			t.Fatalf("Todoist rejected the freshly registered client: marker=%s status=%d", marker, resp.StatusCode)
		}
	}
	if resp.StatusCode != http.StatusFound || !strings.Contains(resp.Header.Get("Location"), "/users/showlogin") {
		t.Fatalf("expected a redirect to the Todoist sign-in page, got status=%d location_present=%t",
			resp.StatusCode, resp.Header.Get("Location") != "")
	}
	t.Logf("todoist connected with no owner setup: status=%d scope=%s state_len=%d",
		resp.StatusCode, query.Get("scope"), len(auth.State))
}

// TestGoogleAndMicrosoftEndpointsStillMatchLiveDiscovery catches the failure
// nothing else here would: a provider moving an endpoint out from under the
// hardcoded URL. It needs no credentials, because both publish their current
// endpoints at a public address.
func TestGoogleAndMicrosoftEndpointsStillMatchLiveDiscovery(t *testing.T) {
	requireLive(t, "whether Google and Microsoft still serve the endpoints this code has hardcoded")

	cases := []struct {
		provider     string
		discoveryURL string
		authorizeURL string
	}{
		{
			provider:     "google",
			discoveryURL: "https://accounts.google.com/.well-known/openid-configuration",
			authorizeURL: startedAuthorizeURL(t, func() (string, error) {
				flow := google.New(google.Config{
					ClientID: "probe", ClientSecret: "probe", Logger: quietLogger(),
				})
				auth, err := flow.Start(context.Background(), "http://127.0.0.1:9187/callback",
					[]manifest.Verb{manifest.Read})
				return auth.URL, err
			}),
		},
		{
			provider:     "microsoft",
			discoveryURL: "https://login.microsoftonline.com/common/v2.0/.well-known/openid-configuration",
			authorizeURL: startedAuthorizeURL(t, func() (string, error) {
				flow := microsoft.New(microsoft.Config{
					ClientID: "probe", ClientSecret: "probe", Tenant: "common", Logger: quietLogger(),
				})
				auth, err := flow.Start(context.Background(), "http://127.0.0.1:9187/callback",
					[]manifest.Verb{manifest.Read})
				return auth.URL, err
			}),
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.provider, func(t *testing.T) {
			resp, err := liveClient().Get(testCase.discoveryURL)
			if err != nil {
				t.Fatalf("GET discovery: %v", err)
			}
			defer resp.Body.Close()
			var document struct {
				AuthorizationEndpoint string `json:"authorization_endpoint"`
			}
			if err := json.NewDecoder(io.LimitReader(resp.Body, 262144)).Decode(&document); err != nil {
				t.Fatalf("decode discovery document: %v", err)
			}
			ours, err := url.Parse(testCase.authorizeURL)
			if err != nil {
				t.Fatalf("parse our authorize URL: %v", err)
			}
			live, err := url.Parse(document.AuthorizationEndpoint)
			if err != nil {
				t.Fatalf("parse published authorize endpoint: %v", err)
			}
			if ours.Host != live.Host || ours.Path != live.Path {
				t.Fatalf("%s moved its authorize endpoint: this code builds %s%s, the provider now publishes %s%s",
					testCase.provider, ours.Host, ours.Path, live.Host, live.Path)
			}
			t.Logf("%s authorize endpoint still %s%s", testCase.provider, live.Host, live.Path)
		})
	}
}

func startedAuthorizeURL(t *testing.T, start func() (string, error)) string {
	t.Helper()
	authorizeURL, err := start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return authorizeURL
}
