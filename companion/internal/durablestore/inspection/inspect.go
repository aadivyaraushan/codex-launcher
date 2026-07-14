package inspection

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var (
	ErrStateUnavailable   = errors.New("companion state is unavailable")
	ErrSchemaIncompatible = errors.New("companion state schema is incompatible")
	ErrIdentityLoss       = errors.New("host identity is missing while paired devices remain")
	ErrInvalidIdentity    = errors.New("host identity is invalid")
)

type State struct {
	SchemaCompatible    bool
	IdentityPresent     bool
	IdentityFingerprint string
	PairedDevices       int
}

func Inspect(ctx context.Context, path string) (State, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return State{}, ErrStateUnavailable
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.Mode().IsRegular() {
		return State{}, ErrStateUnavailable
	}
	databaseURL := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := databaseURL.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(1)")
	databaseURL.RawQuery = query.Encode()
	database, err := sql.Open("sqlite", databaseURL.String())
	if err != nil {
		return State{}, ErrStateUnavailable
	}
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		return State{}, ErrStateUnavailable
	}
	compatible, err := hasRequiredTables(ctx, database)
	if err != nil {
		return State{}, ErrStateUnavailable
	}
	if !compatible {
		return State{SchemaCompatible: false}, ErrSchemaIncompatible
	}
	state := State{SchemaCompatible: true}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM paired_devices`).Scan(&state.PairedDevices); err != nil {
		return State{}, ErrStateUnavailable
	}
	var identity []byte
	err = database.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'host_identity'`).Scan(&identity)
	if errors.Is(err, sql.ErrNoRows) {
		if state.PairedDevices != 0 {
			return state, ErrIdentityLoss
		}
		return state, nil
	}
	if err != nil {
		return State{}, ErrStateUnavailable
	}
	if len(identity) != ed25519.PrivateKeySize {
		return state, ErrInvalidIdentity
	}
	publicKey := ed25519.PrivateKey(identity).Public().(ed25519.PublicKey)
	encoded, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return state, ErrInvalidIdentity
	}
	state.IdentityPresent = true
	state.IdentityFingerprint = base64.RawURLEncoding.EncodeToString(encoded)
	return state, nil
}

func hasRequiredTables(ctx context.Context, database *sql.DB) (bool, error) {
	required := map[string]bool{
		"metadata": false, "paired_devices": false, "pairing_offers": false, "prompt_entries": false,
		"event_state": false, "journal_events": false, "event_acks": false,
	}
	rows, err := database.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if _, found := required[name]; found {
			required[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	for _, found := range required {
		if !found {
			return false, nil
		}
	}
	return true, nil
}
