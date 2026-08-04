package beeper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The contract these tests hold the client to was read off a live Beeper
// Desktop's own OpenAPI document (GET /v1/spec), not the docs site — the docs
// reference page 404s, and the spike that trusted prose over the running
// service is exactly why this package exists.

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func staticToken(value string) TokenSource { return StaticToken(value) }

// captureLogger returns a logger plus the buffer it writes to, so a test can
// assert on what did and did not reach the log.
func captureLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, nil)), buf
}

func TestSearchChatsSendsBearerTokenAndReturnsTypedChats(t *testing.T) {
	var gotAuth, gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[
			{"id":"c1","accountID":"instagramgo","network":"Instagram","title":"Maya","type":"single"},
			{"id":"c2","accountID":"gmessages","network":"Google Messages","title":"Maya R","type":"single"}
		]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok-abc"), server.Client(), discardLogger())
	chats, err := client.SearchChats(context.Background(), "Maya")
	if err != nil {
		t.Fatalf("SearchChats returned an error: %v", err)
	}
	if gotAuth != "Bearer tok-abc" {
		t.Fatalf("expected a bearer token header, got %q", gotAuth)
	}
	if gotPath != "/v1/chats/search" {
		t.Fatalf("expected /v1/chats/search, got %q", gotPath)
	}
	if gotQuery != "Maya" {
		t.Fatalf("expected the search term to travel as ?query=, got %q", gotQuery)
	}
	if len(chats) != 2 {
		t.Fatalf("expected 2 chats, got %d", len(chats))
	}
	if chats[0].ID != "c1" || chats[0].Network != "Instagram" || chats[0].Title != "Maya" {
		t.Fatalf("first chat decoded wrong: %+v", chats[0])
	}
}

func TestAccountsReturnsTheConnectedNetworkAccountIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/accounts" {
			t.Fatalf("request = %s %s, want GET /v1/accounts", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `[{"accountID":"google-account-live","network":"Google Messages","status":"connected"}]`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	accounts, err := client.Accounts(t.Context())
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].ID != "google-account-live" || accounts[0].Network != "Google Messages" || accounts[0].Status != "connected" {
		t.Fatalf("accounts = %+v", accounts)
	}
}

// Regression: a real Beeper rejects "?query=" with
// VALIDATION_ERROR "String must contain at least 1 character(s)". An empty
// search must omit the parameter, not send it blank. The fake server in these
// tests accepted the blank happily, which is how this got through the first time.
func TestEmptySearchOmitsTheQueryParameterInsteadOfSendingItBlank(t *testing.T) {
	var hadQueryParam bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadQueryParam = r.URL.Query()["query"]
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if _, err := client.SearchChats(context.Background(), ""); err != nil {
		t.Fatalf("an empty search should be allowed: %v", err)
	}
	if hadQueryParam {
		t.Fatal("an empty search must omit ?query= entirely; a real Beeper 400s on a blank one")
	}
}

func TestSearchChatsFailsLoudlyWhenBeeperRejectsTheRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Unauthorized: Invalid or missing token","code":"unauthorized"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("bad"), server.Client(), discardLogger())
	if _, err := client.SearchChats(context.Background(), "Maya"); err == nil {
		t.Fatal("a 401 from Beeper must be an error, not an empty result")
	}
}

func TestStartChatPostsAnExactPhoneToTheSelectedAccount(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody struct {
		AccountID string `json:"accountID"`
		User      struct {
			PhoneNumber string `json:"phoneNumber"`
		} `json:"user"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"google-chat-1","accountID":"gmessages","network":"Google Messages","title":"wife","type":"single"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	chat, err := client.StartChat(t.Context(), "gmessages", "+12243228828")
	if err != nil {
		t.Fatalf("StartChat: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/chats/start" {
		t.Fatalf("request = %s %s, want POST /v1/chats/start", gotMethod, gotPath)
	}
	if gotBody.AccountID != "gmessages" || gotBody.User.PhoneNumber != "+12243228828" {
		t.Fatalf("start body = %+v", gotBody)
	}
	if chat.ID != "google-chat-1" || chat.Title != "wife" || chat.Network != "Google Messages" {
		t.Fatalf("chat = %+v", chat)
	}
}

func TestSendPostsTheTextAndReturnsThePendingMessageID(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"chatID":"c1","pendingMessageID":"pm-77"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	sent, err := client.Send(context.Background(), "c1", "running about ten minutes late")
	if err != nil {
		t.Fatalf("Send returned an error: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/chats/c1/messages" {
		t.Fatalf("expected POST /v1/chats/c1/messages, got %s %s", gotMethod, gotPath)
	}
	if gotBody["text"] != "running about ten minutes late" {
		t.Fatalf("message text did not reach Beeper: %+v", gotBody)
	}
	if sent.PendingMessageID != "pm-77" || sent.ChatID != "c1" {
		t.Fatalf("send result decoded wrong: %+v", sent)
	}
}

func TestSendRefusesEmptyTextWithoutCallingBeeper(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if _, err := client.Send(context.Background(), "c1", "   "); err == nil {
		t.Fatal("blank message text must be refused")
	}
	if called {
		t.Fatal("a blank message must not reach Beeper at all")
	}
}

// The owner's phone and accounts are on the other end of this client. Read-only
// is the guard that makes it safe to point it at a live Beeper while building.
func TestReadOnlyClientRefusesToSendAndNeverOpensAConnection(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger()).ReadOnly()
	_, err := client.Send(context.Background(), "c1", "hello")
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
	if called {
		t.Fatal("a read-only client must not issue the request at all")
	}
}

// ResolveOne is the contact-book question in one call: it must never guess
// which person was meant.
func TestResolveOneReturnsTheChatWhenExactlyOneMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[{"id":"c9","accountID":"instagramgo","network":"Instagram","title":"Maya","type":"single"}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	chat, err := client.ResolveOne(context.Background(), "Maya")
	if err != nil {
		t.Fatalf("ResolveOne returned an error: %v", err)
	}
	if chat.ID != "c9" {
		t.Fatalf("expected chat c9, got %+v", chat)
	}
}

func TestResolveOneRefusesToGuessBetweenTwoPeople(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[
			{"id":"c1","accountID":"instagramgo","network":"Instagram","title":"Maya","type":"single"},
			{"id":"c2","accountID":"gmessages","network":"Google Messages","title":"Maya R","type":"single"}
		]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	_, err := client.ResolveOne(context.Background(), "Maya")
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("expected an AmbiguousError, got %v", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("expected both candidates carried back, got %d", len(ambiguous.Candidates))
	}
	// The user only ever reads the question string, so the networks must be in it.
	if !strings.Contains(ambiguous.Error(), "Instagram") || !strings.Contains(ambiguous.Error(), "Google Messages") {
		t.Fatalf("the ambiguity message must name the networks, got %q", ambiguous.Error())
	}
}

func TestResolveOneSaysNobodyMatchedRatherThanReturningAnEmptyChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if _, err := client.ResolveOne(context.Background(), "Nobody"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("expected ErrNoMatch, got %v", err)
	}
}

// House rule: log lines carry counts, lengths and ids — never user content.
func TestLogsNeverCarryTheMessageTextOrTheToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"chatID":"c1","pendingMessageID":"pm-1"}`)
	}))
	defer server.Close()

	logger, buf := captureLogger()
	const secret = "meet me at the back entrance at nine"
	client := NewClient(server.URL, staticToken("super-secret-token"), server.Client(), logger)
	if _, err := client.Send(context.Background(), "c1", secret); err != nil {
		t.Fatalf("Send returned an error: %v", err)
	}
	logged := buf.String()
	if strings.Contains(logged, secret) {
		t.Fatalf("the message text reached the log:\n%s", logged)
	}
	if strings.Contains(logged, "super-secret-token") {
		t.Fatalf("the access token reached the log:\n%s", logged)
	}
	if !strings.Contains(logged, "text_length") {
		t.Fatalf("expected the length to be logged instead of the text, got:\n%s", logged)
	}
}
