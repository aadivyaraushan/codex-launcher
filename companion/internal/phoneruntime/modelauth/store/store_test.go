package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth"
	_ "modernc.org/sqlite"
)

const secretJSON = `{
  "version": 1,
  "profiles": {
    "openai:default": {
      "type": "oauth",
      "provider": "openai",
      "access": "sk-secret-access",
      "refresh": "rt-secret-refresh"
    },
    "anthropic:default": {
      "type": "oauth",
      "provider": "anthropic",
      "access": "sk-anthropic-secret"
    },
    "openai:manual": {
      "type": "api_key",
      "provider": "openai",
      "key": "sk-other-secret"
    }
  }
}`

func TestReadSQLiteReturnsOpenAIIdTypeProviderWithoutTokens(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("OPENCLAW_STATE_DIR", stateDir)
	writeAuthSQLite(t, filepath.Join(stateDir, "agents", "main", "agent"), secretJSON)

	profiles, err := Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	assertOpenAIOauthAndKeyOnly(t, profiles)
	assertNoTokenBytes(t, profiles)
}

func TestReadJSONFallbackWhenSQLiteMissing(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("OPENCLAW_STATE_DIR", stateDir)
	agentDir := filepath.Join(stateDir, "agents", "main", "agent")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "auth-profiles.json"), []byte(secretJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	profiles, err := Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	assertOpenAIOauthAndKeyOnly(t, profiles)
	assertNoTokenBytes(t, profiles)
}

func TestReadNotFoundWhenStateDirEmpty(t *testing.T) {
	t.Setenv("OPENCLAW_STATE_DIR", t.TempDir())
	_, err := Read()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Read err = %v, want ErrNotFound", err)
	}
}

func TestReadPrefersSQLiteOverJSON(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("OPENCLAW_STATE_DIR", stateDir)
	agentDir := filepath.Join(stateDir, "agents", "main", "agent")
	writeAuthSQLite(t, agentDir, secretJSON)
	if err := os.WriteFile(filepath.Join(agentDir, "auth-profiles.json"), []byte(`{"profiles":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	profiles, err := Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if modelauth.Classify(profiles, false) != modelauth.OauthReady {
		t.Fatalf("sqlite oauth lost to empty json, got %q", modelauth.Classify(profiles, false))
	}
}

func writeAuthSQLite(t *testing.T, agentDir, storeJSON string) {
	t.Helper()
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(agentDir, "openclaw-agent.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE auth_profile_store (
		store_key TEXT NOT NULL PRIMARY KEY,
		store_json TEXT NOT NULL,
		updated_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO auth_profile_store (store_key, store_json, updated_at) VALUES (?, ?, 1)`, "primary", storeJSON)
	if err != nil {
		t.Fatal(err)
	}
}

func assertOpenAIOauthAndKeyOnly(t *testing.T, profiles []modelauth.Profile) {
	t.Helper()
	if len(profiles) != 2 {
		t.Fatalf("profiles = %+v, want openai oauth + api_key only", profiles)
	}
	byID := map[string]modelauth.Profile{}
	for _, profile := range profiles {
		byID[profile.ID] = profile
	}
	if byID["openai:default"] != (modelauth.Profile{ID: "openai:default", Type: "oauth", Provider: "openai"}) {
		t.Fatalf("oauth = %+v", byID["openai:default"])
	}
	if byID["openai:manual"] != (modelauth.Profile{ID: "openai:manual", Type: "api_key", Provider: "openai"}) {
		t.Fatalf("api_key = %+v", byID["openai:manual"])
	}
	if modelauth.Classify(profiles, false) != modelauth.OauthReady {
		t.Fatalf("Classify = %q, want oauth_ready", modelauth.Classify(profiles, false))
	}
}

func assertNoTokenBytes(t *testing.T, profiles []modelauth.Profile) {
	t.Helper()
	raw, err := json.Marshal(profiles)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, banned := range []string{"sk-", "access_token", "refresh_token", "rt-secret", "sk-secret"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("profiles leaked %q: %s", banned, raw)
		}
	}
}
