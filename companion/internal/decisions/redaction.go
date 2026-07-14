package decisions

import (
	"regexp"
	"strings"
	"unicode"
)

const redactedSecret = "<redacted:secret>"

var sensitiveName = regexp.MustCompile(`(?i)(^|[_-])(api[_-]?key|token|password|passwd|secret|credential|authorization|cookie)([_-]|$)`)

func RedactCommand(command string) (string, bool) {
	if len(command) == 0 || len(command) > 4096 || strings.IndexFunc(command, func(r rune) bool { return unicode.IsControl(r) }) >= 0 {
		return "", false
	}
	tokens, okay := commandTokens(command)
	if !okay || len(tokens) == 0 {
		return "", false
	}
	redacted := false
	understandable := true
	for index := range tokens {
		token := tokens[index]
		name, _, assignment := strings.Cut(token, "=")
		if assignment && sensitiveName.MatchString(strings.TrimLeft(name, "-")) {
			tokens[index] = name + "=" + redactedSecret
			redacted = true
			continue
		}
		if index > 0 && sensitiveFlag(tokens[index-1]) {
			tokens[index] = redactedSecret
			redacted = true
			if tokens[index-1] == "-c" || tokens[index-1] == "--command" {
				understandable = false
			}
			continue
		}
		if sensitiveReference(token) {
			tokens[index] = redactedSecret
			redacted = true
			if index > 0 && (tokens[index-1] == "-c" || tokens[index-1] == "--command") {
				understandable = false
			}
		}
	}
	if !redacted {
		return strings.Join(tokens, " "), true
	}
	if tokens[0] == redactedSecret || len(tokens) == 1 {
		understandable = false
	}
	return strings.Join(tokens, " "), understandable
}

func sensitiveFlag(token string) bool {
	trimmed := strings.TrimLeft(token, "-")
	return sensitiveName.MatchString(trimmed) || strings.EqualFold(trimmed, "bearer")
}

func sensitiveReference(token string) bool {
	trimmed := strings.Trim(token, "{}$%")
	return (strings.HasPrefix(token, "$") || strings.HasPrefix(token, "%")) && sensitiveName.MatchString(trimmed)
}

func commandTokens(command string) ([]string, bool) {
	result := make([]string, 0)
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			result = append(result, current.String())
			current.Reset()
		}
	}
	for _, character := range command {
		if escaped {
			current.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			} else {
				current.WriteRune(character)
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if unicode.IsSpace(character) {
			flush()
			continue
		}
		current.WriteRune(character)
	}
	if escaped || quote != 0 {
		return nil, false
	}
	flush()
	return result, true
}
