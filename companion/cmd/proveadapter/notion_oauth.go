package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore"
	oauthcredential "github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore/oauth"
	notionadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/notion"
	notionoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/notion"
)

const (
	notionClientRecord = "notion_oauth_client"
	notionTokenRecord  = "notion_oauth"
)

func runNotion(args []string) error {
	fs := flag.NewFlagSet("notion", flag.ExitOnError)
	redirectURI := fs.String("redirect", "http://127.0.0.1:9197/oauth/notion/callback", "loopback OAuth callback registered dynamically with Notion")
	reauthorize := fs.Bool("reauthorize", false, "ignore a fresh stored connection and run browser OAuth again")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Escape hatch when Keychain ACL is apple-tool-only / -25293: pass a token
	// already obtained (never invent). Callers: live proof after security -w export.
	if tok := strings.TrimSpace(os.Getenv("OPERATOR_NOTION_ACCESS_TOKEN")); tok != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		step(1, "Use OPERATOR_NOTION_ACCESS_TOKEN (Keychain ACL bypass for this run)")
		return measureNotionConnection(ctx, tok)
	}
	parsed, err := url.Parse(*redirectURI)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		return errors.New("notion proof: redirect must be an http://127.0.0.1 loopback URL")
	}
	listener, err := net.Listen("tcp", parsed.Host)
	if err != nil {
		return fmt.Errorf("notion proof: listen for OAuth callback: %w", err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	logger := slog.Default()
	store := credentialstore.NewKeychain(logger)
	flow := notionoauth.New(notionoauth.Config{ServerURL: notionadapter.ServerURL, Logger: logger})
	if !*reauthorize {
		if record, loadErr := oauthcredential.Load(ctx, store, notionTokenRecord); loadErr == nil && (record.ExpiresAt.IsZero() || record.ExpiresAt.After(time.Now().Add(time.Minute))) {
			step(1, "Reuse the fresh Notion workspace connection from macOS Keychain")
			line("workspace id present=%t; refresh token present=%t", record.Metadata["workspace_id"] != "", record.RefreshToken != "")
			return measureNotionConnection(ctx, record.AccessToken)
		}
	}

	step(1, "Discover Notion's OAuth endpoints and reuse or register Operator")
	metadata, err := flow.Discover(ctx)
	if err != nil {
		return err
	}
	client, reused, err := loadOrRegisterNotionClient(ctx, store, flow, metadata, *redirectURI)
	if err != nil {
		return err
	}
	line("dynamic client ready; reused=%t; client id is stored in macOS Keychain", reused)

	authorization, err := flow.Start(ctx, metadata, client, *redirectURI)
	if err != nil {
		return err
	}
	tokens := make(chan notionoauth.TokenSet, 1)
	callbackErrors := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(parsed.Path, func(w http.ResponseWriter, r *http.Request) {
		if providerError := strings.TrimSpace(r.URL.Query().Get("error")); providerError != "" {
			err := fmt.Errorf("notion proof: provider callback error %s", providerError)
			http.Error(w, "Notion sign-in was not accepted. Return to Operator.", http.StatusBadRequest)
			callbackErrors <- err
			return
		}
		set, err := flow.Callback(ctx, r.URL.Query().Get("state"), r.URL.Query().Get("code"))
		if err != nil {
			http.Error(w, "Notion sign-in was not accepted. Return to Operator.", http.StatusBadRequest)
			callbackErrors <- err
			return
		}
		_, _ = w.Write([]byte("Notion is connected. You can return to Operator."))
		tokens <- set
	})
	server := &http.Server{Handler: mux}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			callbackErrors <- serveErr
		}
	}()
	defer server.Shutdown(context.Background())

	step(2, "Complete Notion OAuth in the approved browser")
	fmt.Printf("AUTH_URL=%s\n", authorization.URL)
	line("redirect: %s", *redirectURI)
	var tokenSet notionoauth.TokenSet
	select {
	case tokenSet = <-tokens:
	case callbackErr := <-callbackErrors:
		return callbackErr
	case <-ctx.Done():
		return fmt.Errorf("notion proof: OAuth timed out: %w", ctx.Err())
	}

	if err := oauthcredential.Save(ctx, store, notionTokenRecord, oauthcredential.Record{
		Provider: "notion", Account: tokenSet.WorkspaceID, AccessToken: tokenSet.AccessToken,
		RefreshToken: tokenSet.RefreshToken, TokenType: tokenSet.TokenType,
		Scopes: strings.Fields(tokenSet.Scope), ExpiresAt: tokenSet.ExpiresAt,
		Metadata: map[string]string{
			"workspace_id": tokenSet.WorkspaceID, "user_id": tokenSet.UserID,
			"client_id": client.ClientID, "client_secret": client.ClientSecret,
			"authorization_endpoint": metadata.AuthorizationEndpoint, "token_endpoint": metadata.TokenEndpoint,
			"registration_endpoint": metadata.RegistrationEndpoint, "resource": metadata.Resource,
		},
	}); err != nil {
		return err
	}
	line("refreshable workspace credential stored in macOS Keychain; workspace id present=%t", tokenSet.WorkspaceID != "")

	return measureNotionConnection(ctx, tokenSet.AccessToken)
}

// Callers: runNotion after Keychain load or OAuth callback. User: live Notion proof.
func measureNotionConnection(ctx context.Context, accessToken string) error {
	step(3, "Measure the connected workspace's live MCP tools")
	session := notionadapter.NewHTTPSession(nil, notionadapter.ServerURL, accessToken)
	adapter, err := notionadapter.New(session)
	if err != nil {
		return err
	}
	ceiling, err := adapter.Connect(ctx)
	if err != nil {
		return fmt.Errorf("notion proof: connect hosted MCP: %w", err)
	}
	line("measured ceiling: %s", ceiling)
	if err := proveNotionReadWrite(ctx, accessToken); err != nil {
		return err
	}
	verdict("Notion OAuth, hosted MCP connect, and Operator-container write+read completed")
	return nil
}

// Callers: runNotion in this file (~line 62). Existing helper — not a new file.
// Data: Keychain JSON ClientRegistration {client_id, client_secret,...}; no date fields.
// User: "Diagnose macOS status -25293... Smallest fix so user (or agent) can load Notion client"
//
// Reuses a Keychain-stored DCR client, or registers a new one. go-run ACLs bind by
// cdhash; when that binary is gone, Get returns -25293 (interaction disabled) — delete
// and re-register instead of hard-failing (do not invent secrets).
func loadOrRegisterNotionClient(ctx context.Context, store *credentialstore.Keychain, flow *notionoauth.Flow, metadata notionoauth.Metadata, redirectURI string) (notionoauth.ClientRegistration, bool, error) {
	encoded, err := store.Get(ctx, notionClientRecord)
	if err == nil {
		var client notionoauth.ClientRegistration
		if json.Unmarshal(encoded, &client) == nil && client.ClientID != "" {
			return client, true, nil
		}
		_ = store.Delete(ctx, notionClientRecord)
	} else if !errors.Is(err, credentialstore.ErrNotFound) {
		if delErr := store.Delete(ctx, notionClientRecord); delErr != nil {
			return notionoauth.ClientRegistration{}, false, fmt.Errorf(
				"notion proof: Keychain item %q is unreadable (%v); delete it in Keychain Access (login → %s / %s) or run: security delete-generic-password -s %s -a %s — then re-run proveadapter notion: %w",
				notionClientRecord, err, credentialstore.Service, notionClientRecord, credentialstore.Service, notionClientRecord, delErr,
			)
		}
	}
	client, err := flow.Register(ctx, metadata, redirectURI)
	if err != nil {
		return notionoauth.ClientRegistration{}, false, err
	}
	encoded, err = json.Marshal(client)
	if err != nil {
		return notionoauth.ClientRegistration{}, false, err
	}
	if err := store.Put(ctx, notionClientRecord, encoded); err != nil {
		return notionoauth.ClientRegistration{}, false, err
	}
	return client, false, nil
}
