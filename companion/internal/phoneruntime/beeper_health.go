// Gate: importers=phoneruntime.Open/Health + operator-phone-runtime on Pixel;
// callers=/v1/health beeper= and NewProduction BeeperAPI; API=Beeper /v1/accounts +
// account.db access_token; schemas=BeeperAccountStatus{ID,Network,Status};
// user: "Retry Operator↔Beeper OAuth / health beeper=connected … or bridge phone-local token".
package phoneruntime

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
	_ "modernc.org/sqlite"
)

// BeeperAccountStatus is one bridge row from Beeper /v1/accounts (no secrets).
type BeeperAccountStatus struct {
	ID      string
	Network string
	Status  string
}

func classifyBeeperHealth(probe func(context.Context) ([]BeeperAccountStatus, error)) string {
	if probe == nil {
		return "unavailable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	accounts, err := probe(ctx)
	if err != nil {
		return "unavailable"
	}
	if len(accounts) == 0 {
		return "not_connected"
	}
	for _, account := range accounts {
		if strings.EqualFold(strings.TrimSpace(account.Status), "connected") {
			return "connected"
		}
	}
	return "not_connected"
}

// Fact-force (edit): importers=phoneruntime.Open/Health; callers=runtime.go
// NewProduction BeeperAPI + openBeeperAccounts; schemas=BeeperAccountStatus;
// user: "Continue OpenAI+Beeper — **SLICE 4: B4 + B5**."
func beeperAPIFromEnv(logger *slog.Logger) (*beeper.Client, bool) {
	token := loadBeeperAccessToken()
	if token == "" {
		logger.Info("[phone-runtime] Beeper API unavailable", "reason", "no_access_token")
		return nil, false
	}
	baseURL := strings.TrimSpace(os.Getenv("BEEPER_DESKTOP_BASE_URL"))
	if baseURL == "" {
		baseURL = beeper.DefaultBaseURL
	}
	client := beeper.NewClient(baseURL, beeper.StaticToken(token), nil, logger)
	readOnly := envEnabled(os.Getenv("BEEPER_READONLY"))
	if readOnly {
		logger.Info("[phone-runtime] Beeper API enabled", "base_url", baseURL, "token_present", true, "write_enabled", false)
		return client.ReadOnly(), true
	}
	logger.Info("[phone-runtime] Beeper API enabled", "base_url", baseURL, "token_present", true, "write_enabled", true)
	return client, false
}

func openBeeperAccounts(logger *slog.Logger) func(context.Context) ([]BeeperAccountStatus, error) {
	api, _ := beeperAPIFromEnv(logger)
	if api == nil {
		return nil
	}
	return func(ctx context.Context) ([]BeeperAccountStatus, error) {
		accounts, err := api.Accounts(ctx)
		if err != nil {
			logger.Warn("[phone-runtime] Beeper accounts probe failed", "error", err)
			return nil, err
		}
		out := make([]BeeperAccountStatus, 0, len(accounts))
		for _, account := range accounts {
			out = append(out, BeeperAccountStatus{
				ID:      account.ID,
				Network: account.Network,
				Status:  account.Status,
			})
		}
		logger.Info("[phone-runtime] Beeper accounts probe", "account_count", len(out))
		return out, nil
	}
}

func loadBeeperAccessToken() string {
	if token := strings.TrimSpace(os.Getenv("BEEPER_ACCESS_TOKEN")); token != "" {
		return token
	}
	for _, path := range beeperAccountDBPaths() {
		token, err := readMatrixAccessToken(path)
		if err != nil || token == "" {
			continue
		}
		return token
	}
	return ""
}

func beeperAccountDBPaths() []string {
	if explicit := strings.TrimSpace(os.Getenv("BEEPER_ACCOUNT_DB")); explicit != "" {
		return []string{explicit}
	}
	return []string{
		"/var/lib/beeper/.beeper/data-dirs/local-servers/default/account.db",
		os.ExpandEnv("$HOME/.beeper/data-dirs/local-servers/default/account.db"),
	}
}

func readMatrixAccessToken(path string) (string, error) {
	db, err := sql.Open("sqlite", path+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return "", err
	}
	defer db.Close()
	var token string
	err = db.QueryRow(`SELECT access_token FROM account LIMIT 1`).Scan(&token)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}

func envEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
