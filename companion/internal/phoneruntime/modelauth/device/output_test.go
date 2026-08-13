package device

import "testing"

func TestParseDeviceLoginOutputReadsCodeAndUrlOnly(t *testing.T) {
	stdout := `
Requesting device code…
Open this URL in any browser and enter the code:

https://auth.openai.com/codex/device

Code: AB12-CD34

Waiting for device authorization…
access_token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.secret
refresh_token=rt-should-never-be-returned
`
	got, ok := ParseOutput(stdout)
	if !ok {
		t.Fatal("ParseOutput returned false")
	}
	if got.UserCode != "AB12-CD34" {
		t.Fatalf("UserCode = %q, want AB12-CD34", got.UserCode)
	}
	if got.VerificationURL != "https://auth.openai.com/codex/device" {
		t.Fatalf("VerificationURL = %q", got.VerificationURL)
	}
}

func TestParseDeviceLoginJSONShape(t *testing.T) {
	stdout := `{"userCode":"ZX9K-Q2LM","verificationUrl":"https://auth.openai.com/codex/device","access_token":"sk-leak"}`
	got, ok := ParseOutput(stdout)
	if !ok {
		t.Fatal("ParseOutput JSON returned false")
	}
	if got.UserCode != "ZX9K-Q2LM" {
		t.Fatalf("UserCode = %q", got.UserCode)
	}
	if got.VerificationURL != "https://auth.openai.com/codex/device" {
		t.Fatalf("VerificationURL = %q", got.VerificationURL)
	}
}

func TestRedactStripsTokensFromLogs(t *testing.T) {
	in := "ok access_token=sk-abc123 refresh_token=rt-zzz bearer eyJhbGciOi secret=nope"
	out := Redact(in)
	for _, banned := range []string{"sk-abc123", "rt-zzz", "eyJhbGciOi", "nope"} {
		if containsFold(out, banned) {
			t.Fatalf("Redact leaked %q: %q", banned, out)
		}
	}
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
