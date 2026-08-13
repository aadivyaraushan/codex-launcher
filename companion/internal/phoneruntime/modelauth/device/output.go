package device

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

type Prompt struct {
	UserCode        string
	VerificationURL string
}

var (
	codeLine  = regexp.MustCompile(`(?i)(?:^|\n)\s*(?:code|user code|user_code)\s*[:\-]\s*([A-Z0-9][A-Z0-9\-]{3,})`)
	urlLine   = regexp.MustCompile(`https://auth\.openai\.com/[^\s"'<>]+`)
	secretish = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_\-]+|rt-[A-Za-z0-9_\-]+|eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+|bearer\s+\S+|(?:access_token|refresh_token|client_secret|api_key|secret)\s*[=:]\s*\S+)`)
)

func ParseOutput(stdout string) (Prompt, bool) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return Prompt{}, false
	}
	var prompt Prompt
	if parsed, ok := parseJSONPrompt(trimmed); ok {
		prompt = parsed
	}
	if prompt.UserCode == "" {
		if match := codeLine.FindStringSubmatch(stdout); len(match) == 2 {
			prompt.UserCode = strings.TrimSpace(match[1])
		}
	}
	if prompt.VerificationURL == "" {
		if match := urlLine.FindString(stdout); match != "" {
			prompt.VerificationURL = strings.TrimRight(match, ".,);")
		}
	}
	if !validPrompt(prompt) {
		return Prompt{}, false
	}
	return prompt, true
}

func parseJSONPrompt(raw string) (Prompt, bool) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return Prompt{}, false
	}
	var row struct {
		UserCode        string `json:"userCode"`
		UserCodeSnake   string `json:"user_code"`
		VerificationURL string `json:"verificationUrl"`
		VerificationURI string `json:"verification_uri"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &row); err != nil {
		return Prompt{}, false
	}
	code := strings.TrimSpace(row.UserCode)
	if code == "" {
		code = strings.TrimSpace(row.UserCodeSnake)
	}
	u := strings.TrimSpace(row.VerificationURL)
	if u == "" {
		u = strings.TrimSpace(row.VerificationURI)
	}
	prompt := Prompt{UserCode: code, VerificationURL: u}
	if !validPrompt(prompt) {
		return Prompt{}, false
	}
	return prompt, true
}

func validPrompt(prompt Prompt) bool {
	if prompt.UserCode == "" || prompt.VerificationURL == "" {
		return false
	}
	parsed, err := url.Parse(prompt.VerificationURL)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" || parsed.Host != "auth.openai.com" {
		return false
	}
	return true
}

func Redact(s string) string {
	return secretish.ReplaceAllString(s, "[redacted]")
}
