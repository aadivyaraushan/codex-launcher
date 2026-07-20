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
	Connection   ConnectionConfig  `json:"connection,omitempty"`
	// Relay, ListenHost, and ListenPort are populated only for recognized
	// version-1 input. They keep old in-process callers working while every
	// persisted write uses Connection and version 2.
	Relay      RelayConfig `json:"-"`
	ListenHost string      `json:"-"`
	ListenPort int         `json:"-"`
}

type ConfigSource string

const (
	ConfigSourceV2          ConfigSource = "v2"
	ConfigSourceRelayV1     ConfigSource = "relay_v1"
	ConfigSourceTailscaleV1 ConfigSource = "tailscale_v1"
)

type ConnectionMode string

const (
	ConnectionModeTailscale ConnectionMode = "tailscale"
	ConnectionModeRelay     ConnectionMode = "relay"
)

// ConnectionConfig is deliberately tagged by Mode. Its MarshalJSON method
// writes only the matching settings block so mixed transports cannot appear
// in a newly written configuration.
type ConnectionConfig struct {
	Mode      ConnectionMode
	Tailscale TailscaleConfig
	Relay     RelayConfig
}

func (connection ConnectionConfig) MarshalJSON() ([]byte, error) {
	switch connection.Mode {
	case ConnectionModeTailscale:
		return json.Marshal(struct {
			Mode      ConnectionMode  `json:"mode"`
			Tailscale TailscaleConfig `json:"tailscale"`
		}{Mode: connection.Mode, Tailscale: connection.Tailscale})
	case ConnectionModeRelay:
		return json.Marshal(struct {
			Mode  ConnectionMode `json:"mode"`
			Relay RelayConfig    `json:"relay"`
		}{Mode: connection.Mode, Relay: connection.Relay})
	default:
		return json.Marshal(struct {
			Mode ConnectionMode `json:"mode"`
		}{Mode: connection.Mode})
	}
}

type TailscaleConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type PairingTarget struct {
	Host string
	Port int
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
	config, _, err := loadConfigWithSource(path, root)
	return config, err
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
	config, _, err := loadConfigWithSource(path, root)
	return config, err
}

func LoadConfigWithSource(path string) (Config, ConfigSource, error) {
	root, err := ConfigRoot()
	if err != nil {
		return Config{}, "", ErrInvalidConfig
	}
	return loadConfigWithSource(path, root)
}

// loadConfigWithSource reads strict v2 config plus the two exact v1 shapes.
// It never writes: replacement can therefore start the new binary using an
// old file and publish its rollback snapshot unchanged.
func loadConfigWithSource(path, root string) (Config, ConfigSource, error) {
	file, err := configsecurity.Open(root, path, MaxConfigBytes)
	if errors.Is(err, configsecurity.ErrTooLarge) {
		return Config{}, "", ErrInvalidConfig
	}
	if err != nil {
		return Config{}, "", ErrUnsafeConfigFile
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, MaxConfigBytes+1))
	if err != nil || len(encoded) > MaxConfigBytes {
		return Config{}, "", ErrInvalidConfig
	}
	if err := rejectDuplicateKeys(encoded); err != nil {
		return Config{}, "", ErrInvalidConfig
	}
	return decodeConfig(encoded)
}

func writeConfigAt(path, root string, config Config) error {
	config, err := config.NormalizeV2()
	if err != nil {
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
	config, err := config.NormalizeV2()
	if err != nil {
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
	if !validComputerName(config.ComputerName) {
		return ErrInvalidConfig
	}
	if config.CodexBinary != "" && !filepath.IsAbs(config.CodexBinary) {
		return ErrInvalidConfig
	}
	if _, err := projects.New(config.Projects); err != nil {
		return ErrInvalidConfig
	}
	switch config.Version {
	case 1:
		if err := config.Relay.validate(); err != nil {
			return err
		}
		return nil
	case 2:
		return config.Connection.validate()
	default:
		return ErrInvalidConfig
	}
}

func (config Config) NormalizeV2() (Config, error) {
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	if config.Version == 2 {
		return config, nil
	}
	config.Version = 2
	if config.ListenHost != "" || config.ListenPort != 0 {
		config.Connection = ConnectionConfig{Mode: ConnectionModeTailscale, Tailscale: TailscaleConfig{Host: config.ListenHost, Port: config.ListenPort}}
	} else {
		config.Connection = ConnectionConfig{Mode: ConnectionModeRelay, Relay: config.Relay}
	}
	if err := config.Connection.validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (connection ConnectionConfig) validate() error {
	switch connection.Mode {
	case ConnectionModeTailscale:
		if connection.Relay != (RelayConfig{}) || !validTailscaleHost(connection.Tailscale.Host) || !validPort(connection.Tailscale.Port) {
			return ErrInvalidConfig
		}
	case ConnectionModeRelay:
		if connection.Tailscale != (TailscaleConfig{}) || connection.Relay.validate() != nil || !validPublicEndpoint(connection.Relay.BoxHost) {
			return ErrInvalidConfig
		}
	default:
		return ErrInvalidConfig
	}
	return nil
}

func validPort(port int) bool { return port >= 1 && port <= 65535 }

func validTailscaleHost(host string) bool {
	address, err := netip.ParseAddr(host)
	if err != nil || address.String() != host {
		return false
	}
	if address.Is4() {
		return netip.MustParsePrefix("100.64.0.0/10").Contains(address)
	}
	return netip.MustParsePrefix("fd7a:115c:a1e0::/48").Contains(address)
}

func ValidTailscaleHost(host string) bool { return validTailscaleHost(host) }

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

// validPublicEndpoint mirrors Android's public endpoint classifier for relay
// configuration. It is intentionally structural: DNS resolution and current
// reachability are live doctor/setup checks, not config parsing work.
func validPublicEndpoint(host string) bool {
	if host == "" || len(host) > maxBoxHostLength {
		return false
	}
	if address, err := netip.ParseAddr(host); err == nil {
		return address.String() == host && address.IsGlobalUnicast() && !address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast() && !address.IsMulticast() && !address.IsUnspecified()
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-') {
				return false
			}
		}
	}
	return strings.IndexFunc(labels[len(labels)-1], func(character rune) bool { return unicode.IsLetter(character) && character <= unicode.MaxASCII }) >= 0
}

func (config Config) PairingTarget() (PairingTarget, error) {
	normalized, err := config.NormalizeV2()
	if err != nil {
		return PairingTarget{}, err
	}
	switch normalized.Connection.Mode {
	case ConnectionModeTailscale:
		return PairingTarget{Host: normalized.Connection.Tailscale.Host, Port: normalized.Connection.Tailscale.Port}, nil
	case ConnectionModeRelay:
		return PairingTarget{Host: normalized.Connection.Relay.BoxHost, Port: normalized.Connection.Relay.PhonePort}, nil
	default:
		return PairingTarget{}, ErrInvalidConfig
	}
}

func (config Config) ConnectionMode() ConnectionMode {
	normalized, err := config.NormalizeV2()
	if err != nil {
		return ""
	}
	return normalized.Connection.Mode
}

func (config Config) RelaySettings() (RelayConfig, error) {
	normalized, err := config.NormalizeV2()
	if err != nil || normalized.Connection.Mode != ConnectionModeRelay {
		return RelayConfig{}, ErrInvalidConfig
	}
	return normalized.Connection.Relay, nil
}

func decodeConfig(encoded []byte) (Config, ConfigSource, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return Config{}, "", ErrInvalidConfig
	}
	versionRaw, ok := fields["version"]
	if !ok {
		return Config{}, "", ErrInvalidConfig
	}
	var version int
	if err := json.Unmarshal(versionRaw, &version); err != nil {
		return Config{}, "", ErrInvalidConfig
	}
	switch version {
	case 2:
		if !hasRequiredKeys(fields, []string{"version", "computerName", "projects", "connection"}, "codexBinary") {
			return Config{}, "", ErrInvalidConfig
		}
		var disk struct {
			Version      int               `json:"version"`
			ComputerName string            `json:"computerName"`
			CodexBinary  string            `json:"codexBinary"`
			Projects     []projects.Config `json:"projects"`
			Connection   json.RawMessage   `json:"connection"`
		}
		if err := decodeStrict(encoded, &disk); err != nil {
			return Config{}, "", ErrInvalidConfig
		}
		connection, err := decodeConnection(disk.Connection)
		if err != nil {
			return Config{}, "", ErrInvalidConfig
		}
		config := Config{Version: disk.Version, ComputerName: disk.ComputerName, CodexBinary: disk.CodexBinary, Projects: disk.Projects, Connection: connection}
		if connection.Mode == ConnectionModeRelay {
			config.Relay = connection.Relay
		}
		if err := config.Validate(); err != nil {
			return Config{}, "", err
		}
		return config, ConfigSourceV2, nil
	case 1:
		return decodeLegacyConfig(encoded, fields)
	default:
		return Config{}, "", ErrInvalidConfig
	}
}

func decodeConnection(encoded json.RawMessage) (ConnectionConfig, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return ConnectionConfig{}, err
	}
	modeRaw, ok := fields["mode"]
	if !ok {
		return ConnectionConfig{}, ErrInvalidConfig
	}
	var mode ConnectionMode
	if err := json.Unmarshal(modeRaw, &mode); err != nil {
		return ConnectionConfig{}, err
	}
	switch mode {
	case ConnectionModeRelay:
		if !hasExactKeys(fields, "mode", "relay") {
			return ConnectionConfig{}, ErrInvalidConfig
		}
		var disk struct {
			Mode  ConnectionMode `json:"mode"`
			Relay RelayConfig    `json:"relay"`
		}
		if err := decodeStrict(encoded, &disk); err != nil {
			return ConnectionConfig{}, err
		}
		return ConnectionConfig{Mode: disk.Mode, Relay: disk.Relay}, nil
	case ConnectionModeTailscale:
		if !hasExactKeys(fields, "mode", "tailscale") {
			return ConnectionConfig{}, ErrInvalidConfig
		}
		var disk struct {
			Mode      ConnectionMode  `json:"mode"`
			Tailscale TailscaleConfig `json:"tailscale"`
		}
		if err := decodeStrict(encoded, &disk); err != nil {
			return ConnectionConfig{}, err
		}
		return ConnectionConfig{Mode: disk.Mode, Tailscale: disk.Tailscale}, nil
	default:
		return ConnectionConfig{}, ErrInvalidConfig
	}
}

func decodeLegacyConfig(encoded []byte, fields map[string]json.RawMessage) (Config, ConfigSource, error) {
	if hasRequiredKeys(fields, []string{"version", "computerName", "projects", "relay"}, "codexBinary") {
		var disk struct {
			Version      int               `json:"version"`
			ComputerName string            `json:"computerName"`
			CodexBinary  string            `json:"codexBinary"`
			Projects     []projects.Config `json:"projects"`
			Relay        RelayConfig       `json:"relay"`
		}
		if err := decodeStrict(encoded, &disk); err != nil {
			return Config{}, "", ErrInvalidConfig
		}
		legacy := Config{Version: 1, ComputerName: disk.ComputerName, CodexBinary: disk.CodexBinary, Projects: disk.Projects, Relay: disk.Relay}
		normalized, err := legacy.NormalizeV2()
		if err != nil {
			return Config{}, "", err
		}
		normalized.Relay = disk.Relay
		return normalized, ConfigSourceRelayV1, nil
	}
	if hasRequiredKeys(fields, []string{"version", "computerName", "projects", "listenHost", "listenPort"}, "codexBinary") {
		var disk struct {
			Version      int               `json:"version"`
			ComputerName string            `json:"computerName"`
			CodexBinary  string            `json:"codexBinary"`
			Projects     []projects.Config `json:"projects"`
			ListenHost   string            `json:"listenHost"`
			ListenPort   int               `json:"listenPort"`
		}
		if err := decodeStrict(encoded, &disk); err != nil {
			return Config{}, "", ErrInvalidConfig
		}
		legacy := Config{Version: 1, ComputerName: disk.ComputerName, CodexBinary: disk.CodexBinary, Projects: disk.Projects, Relay: RelayConfig{BoxHost: "legacy.invalid", MacPort: 1, PhonePort: 2, PinnedKey: base64.StdEncoding.EncodeToString([]byte("legacy")), Secret: strings.Repeat("x", minRelaySecretLength)}, ListenHost: disk.ListenHost, ListenPort: disk.ListenPort}
		normalized, err := legacy.NormalizeV2()
		if err != nil {
			return Config{}, "", err
		}
		normalized.ListenHost, normalized.ListenPort = disk.ListenHost, disk.ListenPort
		return normalized, ConfigSourceTailscaleV1, nil
	}
	return Config{}, "", ErrInvalidConfig
}

func hasExactKeys(fields map[string]json.RawMessage, names ...string) bool {
	if len(fields) != len(names) {
		return false
	}
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			return false
		}
	}
	return true
}

func hasRequiredKeys(fields map[string]json.RawMessage, required []string, optional ...string) bool {
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, name := range required {
		allowed[name] = struct{}{}
		if _, ok := fields[name]; !ok {
			return false
		}
	}
	for _, name := range optional {
		allowed[name] = struct{}{}
	}
	for name := range fields {
		if _, ok := allowed[name]; !ok {
			return false
		}
	}
	return true
}

func decodeStrict(encoded []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return ensureJSONEnd(decoder)
}

// RelayClientConfig adapts the persisted RelayConfig into the shape
// relayclient.Listen needs to dial out. PinnedKey is decoded here (already
// validated by Validate) rather than at every call site, so a bad base64
// value can never reach relayclient as a silently-empty pin.
func (config Config) RelayClientConfig() (relayclient.Config, error) {
	relay, err := config.RelaySettings()
	if err != nil {
		return relayclient.Config{}, ErrInvalidConfig
	}
	pinnedKey, err := base64.StdEncoding.DecodeString(relay.PinnedKey)
	if err != nil {
		return relayclient.Config{}, ErrInvalidConfig
	}
	return relayclient.Config{
		BoxAddr:         net.JoinHostPort(relay.BoxHost, strconv.Itoa(relay.MacPort)),
		Secret:          relay.Secret,
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
