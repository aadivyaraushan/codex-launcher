package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth"
	_ "modernc.org/sqlite"
)

var (
	ErrNotFound     = errors.New("openclaw auth store not found")
	ErrUnreadable   = errors.New("openclaw auth store unreadable")
	busyTimeoutMS   = 100
	primaryStoreKey = "primary"
)

func Read() ([]modelauth.Profile, error) {
	var sawFile bool
	var lastErr error
	for _, agentDir := range agentDirs() {
		sqlitePath := filepath.Join(agentDir, "openclaw-agent.sqlite")
		if fileExists(sqlitePath) {
			sawFile = true
			profiles, err := readSQLite(sqlitePath)
			if err == nil {
				return modelauth.OpenAIProfiles(profiles), nil
			}
			lastErr = err
		}
		jsonPath := filepath.Join(agentDir, "auth-profiles.json")
		if fileExists(jsonPath) {
			sawFile = true
			profiles, err := readJSON(jsonPath)
			if err == nil {
				return modelauth.OpenAIProfiles(profiles), nil
			}
			lastErr = err
		}
	}
	if sawFile {
		if lastErr == nil {
			lastErr = ErrUnreadable
		}
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, lastErr)
	}
	return nil, ErrNotFound
}

func agentDirs() []string {
	seen := map[string]bool{}
	var out []string
	add := func(dir string) {
		dir = filepath.Clean(dir)
		if dir == "." || dir == "" || seen[dir] {
			return
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return
		}
		seen[dir] = true
		out = append(out, dir)
	}
	for _, stateDir := range stateDirs() {
		add(filepath.Join(stateDir, "agents", "main", "agent"))
		matches, err := filepath.Glob(filepath.Join(stateDir, "agents", "*", "agent"))
		if err != nil {
			continue
		}
		for _, match := range matches {
			add(match)
		}
	}
	return out
}

func stateDirs() []string {
	if explicit := strings.TrimSpace(os.Getenv("OPENCLAW_STATE_DIR")); explicit != "" {
		return []string{explicit}
	}
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, ".openclaw"))
	}
	dirs = append(dirs, "/root/.openclaw")
	seen := map[string]bool{}
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = filepath.Clean(dir)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		out = append(out, dir)
	}
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func readJSON(path string) ([]modelauth.Profile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return modelauth.ParseStoreJSON(raw)
}

func readSQLite(path string) ([]modelauth.Profile, error) {
	dsn, err := readOnlyDSN(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	var storeJSON string
	err = db.QueryRow(`SELECT store_json FROM auth_profile_store WHERE store_key = ? LIMIT 1`, primaryStoreKey).Scan(&storeJSON)
	if err != nil {
		return nil, err
	}
	return modelauth.ParseStoreJSON([]byte(storeJSON))
}

func readOnlyDSN(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := u.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(1)")
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeoutMS))
	u.RawQuery = query.Encode()
	return u.String(), nil
}
