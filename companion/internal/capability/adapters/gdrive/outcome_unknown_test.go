package gdrive

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
)

// swallowsTheRequest is a server that accepts the connection, reads the whole
// request, and then drops the connection without answering. That is what a
// lost reply actually looks like: the request arrived, Drive may well have
// acted on it, and only the answer went missing.
//
// The earlier version of these tests used `server.Close()` and a comment
// claiming "the request leaves but no reply ever comes back". It does not.
// With nothing listening, the connection is refused and the request never
// leaves the machine — a certain failure, not an ambiguous one. The tests
// passed only because the rule they were checking treated every non-GET
// transport error as ambiguous, which is the bug that rule has since been
// narrowed to fix.
func swallowsTheRequest(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("could not take over the connection: %v", err)
			return
		}
		_ = connection.Close()
	}))
	t.Cleanup(server.Close)
	return server
}

// A lost reply to a write (file creation) is ambiguous: Drive may have
// already created the file. A lost reply to a read never is — nothing
// changed, so it is always safe to just retry.
func TestHTTPClientClassifiesLostRepliesByReadOrWrite(t *testing.T) {
	t.Run("a write whose reply never arrives is an unknown outcome", func(t *testing.T) {
		server := swallowsTheRequest(t)

		client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
		_, err := client.CreateFile(context.Background(), CreateFile{Name: "notes.txt", Content: "hello"})

		var unknown *capabilityadapter.OutcomeUnknownError
		if !errors.As(err, &unknown) {
			t.Fatalf("expected an outcome-unknown error, got %v", err)
		}
		if unknown.AdapterID != ID {
			t.Fatalf("adapter id = %q, want %q", unknown.AdapterID, ID)
		}
	})

	// The mirror of the case above, and the reason the rule is not simply
	// "every transport error is ambiguous": nothing was sent, so nothing can
	// have happened, and telling someone to go and check would be noise.
	t.Run("a write that never reached the server is a plain failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		server.Close() // nothing is listening: the connection is refused

		client := NewHTTPClient(server.URL, staticToken("access-secret"), http.DefaultClient, slog.New(slog.NewTextHandler(io.Discard, nil)))
		_, err := client.CreateFile(context.Background(), CreateFile{Name: "notes.txt", Content: "hello"})

		var unknown *capabilityadapter.OutcomeUnknownError
		if errors.As(err, &unknown) {
			t.Fatalf("a request that was refused was marked as an unknown outcome: %v", err)
		}
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("a read whose reply never arrives is a plain failure", func(t *testing.T) {
		server := swallowsTheRequest(t)

		client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
		_, err := client.ListFiles(context.Background(), "notes")

		var unknown *capabilityadapter.OutcomeUnknownError
		if errors.As(err, &unknown) {
			t.Fatalf("a read was marked as an unknown outcome: %v", err)
		}
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("a write Drive answered with a non-2xx status is a plain failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
		_, err := client.CreateFile(context.Background(), CreateFile{Name: "notes.txt", Content: "hello"})

		var unknown *capabilityadapter.OutcomeUnknownError
		if errors.As(err, &unknown) {
			t.Fatalf("a non-2xx write was marked as an unknown outcome: %v", err)
		}
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}
