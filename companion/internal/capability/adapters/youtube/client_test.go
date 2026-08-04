package youtube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Requirement: search.list costs 100 quota units against a 10,000
// units/day default budget, i.e. about 100 searches/day for everyone.
func TestQuotaConstantsMatchTheDocumentedBudget(t *testing.T) {
	if SearchQuotaCostPerCall != 100 {
		t.Fatalf("SearchQuotaCostPerCall = %d, want 100", SearchQuotaCostPerCall)
	}
	if DefaultDailyQuotaUnits != 10000 {
		t.Fatalf("DefaultDailyQuotaUnits = %d, want 10000", DefaultDailyQuotaUnits)
	}
}

func TestHTTPClientBuildsAV3SearchRequestAndParsesTheFirstVideo(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":{"videoId":"vid1"},"snippet":{"title":"A Title","channelTitle":"A Channel"}}]}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-key-do-not-leak", nil, nil)
	videos, err := client.Search(context.Background(), "some query")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if gotPath != "/search" {
		t.Fatalf("path = %q, want /search", gotPath)
	}
	if !strings.Contains(gotQuery, "part=snippet") || !strings.Contains(gotQuery, "type=video") || !strings.Contains(gotQuery, "q=some+query") {
		t.Fatalf("query = %q, missing expected params", gotQuery)
	}
	if len(videos) != 1 || videos[0].ID != "vid1" || videos[0].Title != "A Title" || videos[0].ChannelTitle != "A Channel" {
		t.Fatalf("videos = %+v", videos)
	}
}

func TestHTTPClientDropsResultsWithoutAVideoID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":{},"snippet":{"title":"Unusable","channelTitle":"Channel"}},{"id":{"videoId":"vid2"},"snippet":{"title":"Usable","channelTitle":"Channel"}}]}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-key", nil, nil)
	videos, err := client.Search(context.Background(), "query")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(videos) != 1 || videos[0].ID != "vid2" {
		t.Fatalf("videos = %+v, want only the result with a video id", videos)
	}
}

func TestHTTPClientNeverLeaksTheAPIKeyInAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer server.Close()

	const secret = "super-secret-api-key-value"
	client := NewHTTPClient(server.URL, secret, nil, nil)
	_, err := client.Search(context.Background(), "q")
	if err == nil {
		t.Fatalf("expected an error from a 403 response")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked the api key: %v", err)
	}
}

// Sanity check on the response shape we expect from search.list, so the
// decode target in client.go stays honest about the real API's fields.
func TestSearchResponseShapeDecodesVideoID(t *testing.T) {
	raw := `{"items":[{"id":{"videoId":"xyz"},"snippet":{"title":"T","channelTitle":"C"}}]}`
	var parsed searchResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Items) != 1 || parsed.Items[0].ID.VideoID != "xyz" {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestLiveSearchWithConfiguredOperatorKey(t *testing.T) {
	key := os.Getenv("YOUTUBE_API_KEY")
	if key == "" {
		t.Skip("live YouTube key not supplied")
	}
	client := NewHTTPClient("", key, nil, nil)
	videos, err := client.Search(t.Context(), "bicycle repair basics")
	if err != nil {
		t.Fatalf("live search: %v", err)
	}
	if len(videos) == 0 || videos[0].ID == "" || videos[0].Title == "" {
		t.Fatalf("live search returned no usable video metadata: %+v", videos)
	}
	t.Logf("live search result_count=%d first_title_length=%d channel_present=%t", len(videos), len(videos[0].Title), videos[0].ChannelTitle != "")
}
