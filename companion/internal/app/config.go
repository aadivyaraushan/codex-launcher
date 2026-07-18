package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/configsecurity"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/relayclient"
)

const MaxConfigBytes = 1024 * 1024

// maxBoxHostLength caps RelayConfig.BoxHost the same way DNS caps a
// hostname (253 octets), so an oversized value fails validation instead of
// silently truncating somewhere downstream.
const maxBoxHostLength = 253

// minRelaySecretLength is the minimum length a relay registration secret
// must have. Mirrors relaybox.MinSecretLength: the relay secret is the sole
// access gate now that the Tailscale network gate is gone, so a trivially
// short secret must be rejected at the config boundary too.
const minRelaySecretLength = 16

var (
	ErrInvalidConfig    = errors.New("companion configuration is invalid")
	ErrUnsafeConfigFile = errors.New("companion configuration file is unsafe")
	ErrConfigExists     = errors.New("companion configuration already exists")
)

// Config is the Mac companion's on-disk configuration. The Mac never
// listens for the phone directly (see planning/relay-box-build-plan.md) —
// instead it dials out to a self-hosted relay box described by Relay.
type Config struct {
	Version      int               `json:"version"`
	ComputerName string            `json:"computerName"`
	CodexBinary  string            `json:"codexBinary,omitempty"`
	Projects     []projects.Config `json:"projects"`
	Relay        RelayConfig       `json:"relay"`
}

// RelayConfig describes the relay box this Mac registers with. BoxHost and
// MacPort are where the Mac dials out to become the control line;
// PhonePort is the box's other door, and goes into the pairing offer so the
// phone knows where to connect. PinnedKey and Secret are the two trust
// anchors: PinnedKey is what proves this is really our box (no CA
// fallback — see relayclient/dial.go), and Secret is what proves this is
// really our Mac to the box.
type RelayConfig struct {
	BoxHost   string `json:"boxHost"`
	MacPort   int    `json:"macPort"`
	PhonePort int    `json:"phonePort"`
	PinnedKey string `json:"pinnedKey"`
	Secret    string `json:"secret"`
}

func LoadConfig(path string) (Config, error) {
	root, err := ConfigRoot()
	if err != nil {
		return Config{}, ErrInvalidConfig
	}
	return loadConfig(path, root)
}

func WriteConfig(path string, config Config) error {
	root, err := ConfigRoot()
	if err != nil {
		return ErrInvalidConfig
	}
	return writeConfigAt(path, root, config)
}

func UpdateConfig(path string, config Config) error {
	root, err := ConfigRoot()
	if err != nil {
		return ErrInvalidConfig
	}
	return updateConfigAt(path, root, config)
}

func ConfigRoot() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "codex-launcher"), nil
}

func loadConfig(path, root string) (Config, error) {
	file, err := configsecurity.Open(root, path, MaxConfigBytes)
	if errors.Is(err, configsecurity.ErrTooLarge) {
		return Config{}, ErrInvalidConfig
	}
	if err != nil {
		return Config{}, ErrUnsafeConfigFile
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, MaxConfigBytes+1))
	if err != nil || len(encoded) > MaxConfigBytes {
		return Config{}, ErrInvalidConfig
	}
	if err := rejectDuplicateKeys(encoded); err != nil {
		return Config{}, ErrInvalidConfig
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, ErrInvalidConfig
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return Config{}, ErrInvalidConfig
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func writeConfigAt(path, root string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return ErrInvalidConfig
	}
	encoded = append(encoded, '\n')
	err = configsecurity.WriteNew(root, path, encoded, MaxConfigBytes)
	switch {
	case errors.Is(err, configsecurity.ErrExists):
		return ErrConfigExists
	case errors.Is(err, configsecurity.ErrTooLarge):
		return ErrInvalidConfig
	case err != nil:
		return ErrUnsafeConfigFile
	default:
		return nil
	}
}

func updateConfigAt(path, root string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return ErrInvalidConfig
	}
	encoded = append(encoded, '\n')
	err = configsecurity.Replace(root, path, encoded, MaxConfigBytes)
	switch {
	case errors.Is(err, configsecurity.ErrTooLarge):
		return ErrInvalidConfig
	case err != nil:
		return ErrUnsafeConfigFile
	default:
		return nil
	}
}

func (config Config) Validate() error {
	if config.Version != 1 || !validComputerName(config.ComputerName) {
		return ErrInvalidConfig
	}
	if config.CodexBinary != "" && !filepath.IsAbs(config.CodexBinary) {
		return ErrInvalidConfig
	}
	if _, err := projects.New(config.Projects); err != nil {
		return ErrInvalidConfig
	}
	if err := config.Relay.validate(); err != nil {
		return err
	}
	return nil
}

func validComputerName(name string) bool {
	trimmed := strings.TrimSpace(name)
	return trimmed != "" && len(trimmed) <= 80 && strings.IndexFunc(trimmed, unicode.IsControl) < 0
}

// validate checks RelayConfig on its own terms. Unlike the old Tailscale
// gate, BoxHost is a public address now (a Fly app host, a bare IP,
// whatever the operator points the box at), so this only rejects shapes
// that could never be a valid host/port/key/secret — it does not restrict
// BoxHost to any particular network range.
func (relay RelayConfig) validate() error {
	if !validBoxHost(relay.BoxHost) {
		return ErrInvalidConfig
	}
	if relay.MacPort < 1 || relay.MacPort > 65535 || relay.PhonePort < 1 || relay.PhonePort > 65535 {
		return ErrInvalidConfig
	}
	if relay.MacPort == relay.PhonePort {
		return ErrInvalidConfig
	}
	pinnedKey, err := base64.StdEncoding.DecodeString(relay.PinnedKey)
	if err != nil || len(pinnedKey) == 0 {
		return ErrInvalidConfig
	}
	if len(strings.TrimSpace(relay.Secret)) < minRelaySecretLength {
		return ErrInvalidConfig
	}
	// Checked on the raw (untrimmed) secret so an embedded or trailing
	// control character (e.g. a newline) is caught even though TrimSpace
	// would strip a trailing one before the length check above ever saw it.
	if strings.IndexFunc(relay.Secret, unicode.IsControl) >= 0 {
		return ErrInvalidConfig
	}
	return nil
}

// validBoxHost accepts either a literal IP address (v4 or v6 — a relay box
// hostname is frequently just an address) or a plausible DNS hostname: a
// non-empty string, no whitespace or control characters, within the DNS
// hostname length cap. It deliberately does not check that the hostname
// resolves or is reachable — that is doctor's job, not a config-shape
// check.
func validBoxHost(host string) bool {
	if host == "" || len(host) > maxBoxHostLength {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	for _, r := range host {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// RelayClientConfig adapts the persisted RelayConfig into the shape
// relayclient.Listen needs to dial out. PinnedKey is decoded here (already
// validated by Validate) rather than at every call site, so a bad base64
// value can never reach relayclient as a silently-empty pin.
func (config Config) RelayClientConfig() (relayclient.Config, error) {
	pinnedKey, err := base64.StdEncoding.DecodeString(config.Relay.PinnedKey)
	if err != nil {
		return relayclient.Config{}, ErrInvalidConfig
	}
	return relayclient.Config{
		BoxAddr:         net.JoinHostPort(config.Relay.BoxHost, strconv.Itoa(config.Relay.MacPort)),
		Secret:          config.Relay.Secret,
		PinnedPublicKey: pinnedKey,
	}, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return ErrInvalidConfig
}

func rejectDuplicateKeys(encoded []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	return ensureJSONEnd(decoder)
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make([]string, 0)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return ErrInvalidConfig
			}
			for _, previous := range seen {
				if strings.EqualFold(previous, key) {
					return ErrInvalidConfig
				}
			}
			seen = append(seen, key)
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return ErrInvalidConfig
	}
}
