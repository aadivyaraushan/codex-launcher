package gdrive

// Callers: gdrive HTTPClient unit tests only.
// Affected API: ListFiles/CreateFile request shape + Bearer + multipart.
// Schema: Drive files JSON list + multipart upload. No secrets.
// User: follow-up on Google judge — add httptest coverage for Drive client.

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type staticToken string

func (s staticToken) AccessToken(context.Context) (string, error) { return string(s), nil }

func TestHTTPClientListsAndCreatesWithBearerAuth(t *testing.T) {
	var sawList, sawCreate bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-secret" {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/drive/v3/files"):
			sawList = true
			if !strings.Contains(r.URL.RawQuery, "trashed") {
				t.Errorf("query=%s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"files":[{"id":"f1","name":"notes.txt","mimeType":"text/plain"}]}`)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files"):
			sawCreate = true
			if !strings.Contains(r.Header.Get("Content-Type"), "multipart/") {
				t.Errorf("content-type=%q", r.Header.Get("Content-Type"))
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "hello-drive") {
				t.Errorf("multipart body missing content")
			}
			_, _ = io.WriteString(w, `{"id":"f2","name":"hello.txt","mimeType":"text/plain"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	files, err := client.ListFiles(context.Background(), "")
	if err != nil || len(files) != 1 || files[0].ID != "f1" {
		t.Fatalf("ListFiles=%+v err=%v", files, err)
	}
	created, err := client.CreateFile(context.Background(), CreateFile{Name: "hello.txt", Content: "hello-drive"})
	if err != nil || created.ID != "f2" {
		t.Fatalf("CreateFile=%+v err=%v", created, err)
	}
	if !sawList || !sawCreate {
		t.Fatalf("sawList=%v sawCreate=%v", sawList, sawCreate)
	}
}
