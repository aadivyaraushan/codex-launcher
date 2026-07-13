package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/configsecurity"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

const MaxConfigBytes = 1024 * 1024

var (
	ErrInvalidConfig    = errors.New("companion configuration is invalid")
	ErrUnsafeConfigFile = errors.New("companion configuration file is unsafe")
	ErrConfigExists     = errors.New("companion configuration already exists")
)

var (
	tailscaleIPv4 = netip.MustParsePrefix("100.64.0.0/10")
	tailscaleIPv6 = netip.MustParsePrefix("fd7a:115c:a1e0::/48")
)

type Config struct {
	Version      int               `json:"version"`
	ComputerName string            `json:"computerName"`
	ListenHost   string            `json:"listenHost"`
	ListenPort   int               `json:"listenPort"`
	CodexBinary  string            `json:"codexBinary,omitempty"`
	Projects     []projects.Config `json:"projects"`
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

func (config Config) Validate() error {
	if config.Version != 1 || !validComputerName(config.ComputerName) || !validListenHost(config.ListenHost) || config.ListenPort < 1 || config.ListenPort > 65535 {
		return ErrInvalidConfig
	}
	if config.CodexBinary != "" && !filepath.IsAbs(config.CodexBinary) {
		return ErrInvalidConfig
	}
	if _, err := projects.New(config.Projects); err != nil {
		return ErrInvalidConfig
	}
	return nil
}

func validComputerName(name string) bool {
	trimmed := strings.TrimSpace(name)
	return trimmed != "" && len(trimmed) <= 80 && strings.IndexFunc(trimmed, unicode.IsControl) < 0
}

func validListenHost(host string) bool {
	address, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	address = address.Unmap()
	return tailscaleIPv4.Contains(address) || tailscaleIPv6.Contains(address)
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
