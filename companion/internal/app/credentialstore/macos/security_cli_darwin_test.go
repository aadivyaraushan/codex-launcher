//go:build darwin && cgo

// Callers: unit tests for securityCLIGet decode. User: Keychain Get without -25293.
package macos

import "testing"

func TestDecodeSecurityPasswordHexJSON(t *testing.T) {
	raw := hexOf(`{"provider":"notion"}`)
	got, err := decodeSecurityPassword(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"provider":"notion"}` {
		t.Fatalf("got %q", got)
	}
}

func TestDecodeSecurityPasswordPlainJSON(t *testing.T) {
	got, err := decodeSecurityPassword(`{"client_id":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"client_id":"x"}` {
		t.Fatalf("got %q", got)
	}
}

func hexOf(s string) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(s)*2)
	for i := 0; i < len(s); i++ {
		out[i*2] = hexdigits[s[i]>>4]
		out[i*2+1] = hexdigits[s[i]&0x0f]
	}
	return string(out)
}
