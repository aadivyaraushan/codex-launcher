//go:build darwin && cgo

// Callers: Command.Run find-generic-password on ErrAuthFailed (-25293).
// Affected API: security find-generic-password -w; opaque Keychain secret bytes.
// Schema: JSON oauth records or hex-encoded JSON from security(1).
// User: "Fix Keychain ACL / store path so notion_oauth load via normal Get without -25293."
package macos

import (
	"context"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
)

// securityCLIGet reads a generic password via Apple's security(1). That tool
// can decrypt items whose ACL/partition rejects unsigned Go binaries
// (errSecAuthFailed -25293) while SecKeychainFindGenericPassword cannot.
func securityCLIGet(ctx context.Context, service, account string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "security", "find-generic-password", "-s", service, "-a", account, "-w")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("native keychain load via security CLI: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, fmt.Errorf("native keychain load via security CLI: empty secret")
	}
	secret, err := decodeSecurityPassword(raw)
	if err != nil {
		return nil, err
	}
	if len(secret) == 0 {
		return nil, fmt.Errorf("native keychain load via security CLI: empty secret")
	}
	return secret, nil
}

func decodeSecurityPassword(raw string) ([]byte, error) {
	// security -w sometimes prints hex without a 0x prefix (JSON starting '{'=0x7b).
	if looksLikeHex(raw) {
		decoded, err := hex.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("native keychain load via security CLI: hex decode: %w", err)
		}
		return decoded, nil
	}
	if strings.HasPrefix(raw, "0x") {
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			return nil, fmt.Errorf("native keychain load via security CLI: empty hex")
		}
		decoded, err := hex.DecodeString(strings.TrimPrefix(fields[0], "0x"))
		if err != nil {
			return nil, fmt.Errorf("native keychain load via security CLI: hex decode: %w", err)
		}
		return decoded, nil
	}
	return []byte(raw), nil
}

func looksLikeHex(s string) bool {
	if len(s) < 2 || len(s)%2 != 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
