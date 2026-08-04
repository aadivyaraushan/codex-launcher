// Package explicit routes requests that name one known app and make their
// action clear. It is the no-cloud floor for production: it never guesses an
// app, and it refuses a multi-verb app when the user did not name the verb.
package explicit

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"unicode"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

var (
	ErrNoExplicitApp = errors.New("explicit stage1: no known app was named")
	ErrVerbUnclear   = errors.New("explicit stage1: action is unclear")
)

type Rule struct {
	ID       string
	Name     string
	AppClass string
	Verbs    []manifest.Verb
}

type Model struct {
	rules  []Rule
	logger *slog.Logger
}

func New(rules []Rule, logger *slog.Logger) *Model {
	if logger == nil {
		logger = slog.Default()
	}
	return &Model{rules: append([]Rule(nil), rules...), logger: logger}
}

type word struct {
	original string
	lower    string
}

type appMatch struct {
	rule   Rule
	start  int
	length int
}

type verbMatch struct {
	verb  manifest.Verb
	index int
}

func (m *Model) Route(_ context.Context, utterance string) ([]byte, error) {
	words := splitWords(utterance)
	selected, ok := m.matchApp(words)
	if !ok {
		m.logger.Info("[stage1-explicit] route refused", "reason", "no_explicit_app", "utterance_bytes", len(utterance))
		return nil, ErrNoExplicitApp
	}
	verb, matchedVerb, err := chooseVerb(words, selected.rule.Verbs)
	if err != nil {
		m.logger.Info("[stage1-explicit] route refused", "reason", "verb_unclear", "app_id", selected.rule.ID, "utterance_bytes", len(utterance))
		return nil, err
	}
	subject := routeSubject(words, selected, matchedVerb)
	payload := map[string]any{
		"verb":       string(verb),
		"app_class":  selected.rule.AppClass,
		"app_named":  selected.rule.ID,
		"subject":    subject,
		"body":       utterance,
		"fields":     map[string]string{},
		"confidence": 0.99,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	m.logger.Info("[stage1-explicit] route ready", "app_id", selected.rule.ID, "verb", verb, "utterance_bytes", len(utterance), "subject_length", len(subject))
	return encoded, nil
}

func (m *Model) matchApp(words []word) (appMatch, bool) {
	var selected appMatch
	found := false
	for _, rule := range m.rules {
		for _, name := range []string{rule.Name, rule.ID} {
			candidate := splitWords(name)
			if len(candidate) == 0 {
				continue
			}
			for start := 0; start+len(candidate) <= len(words); start++ {
				if !sameWords(words[start:start+len(candidate)], candidate) {
					continue
				}
				match := appMatch{rule: rule, start: start, length: len(candidate)}
				if !found || match.length > selected.length {
					selected, found = match, true
				}
			}
		}
	}
	return selected, found
}

func chooseVerb(words []word, allowed []manifest.Verb) (manifest.Verb, *verbMatch, error) {
	allowedSet := make(map[manifest.Verb]bool, len(allowed))
	for _, verb := range allowed {
		allowedSet[verb] = true
	}
	aliases := map[string]manifest.Verb{
		"play": manifest.Play, "watch": manifest.Play, "listen": manifest.Play,
		"read": manifest.Read, "find": manifest.Read, "search": manifest.Read, "show": manifest.Read, "browse": manifest.Read, "check": manifest.Read,
		"send": manifest.Compose, "message": manifest.Compose, "text": manifest.Compose, "draft": manifest.Compose, "post": manifest.Compose,
		"write": manifest.Write, "add": manifest.Write, "create": manifest.Write, "save": manifest.Write, "update": manifest.Write,
		"order": manifest.Order,
	}
	var selected *verbMatch
	for index, token := range words {
		verb, known := aliases[token.lower]
		if !known {
			continue
		}
		if selected == nil {
			if !allowedSet[verb] {
				return "", nil, ErrVerbUnclear
			}
			selected = &verbMatch{verb: verb, index: index}
			continue
		}
		if selected.verb != verb && startsAnotherCommand(words, index) {
			return "", nil, ErrVerbUnclear
		}
	}
	if selected != nil {
		return selected.verb, selected, nil
	}
	if len(allowed) == 1 {
		return allowed[0], nil, nil
	}
	return "", nil, ErrVerbUnclear
}

func startsAnotherCommand(words []word, index int) bool {
	if index == 0 {
		return false
	}
	switch words[index-1].lower {
	case "and", "then", "or", "also":
		return true
	default:
		return false
	}
}

func routeSubject(words []word, app appMatch, verb *verbMatch) string {
	remove := make([]bool, len(words))
	for index := app.start; index < app.start+app.length; index++ {
		remove[index] = true
	}
	if verb != nil {
		remove[verb.index] = true
	}
	if app.start > 0 {
		preposition := words[app.start-1].lower
		if preposition == "on" || preposition == "in" || preposition == "using" || preposition == "with" {
			remove[app.start-1] = true
		}
	}
	kept := make([]string, 0, len(words))
	for index, token := range words {
		if !remove[index] {
			kept = append(kept, token.original)
		}
	}
	return strings.TrimSpace(strings.Join(kept, " "))
}

func splitWords(value string) []word {
	parts := strings.FieldsFunc(value, func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '-' && char != '_'
	})
	result := make([]word, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			result = append(result, word{original: part, lower: strings.ToLower(part)})
		}
	}
	return result
}

func sameWords(left, right []word) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].lower != right[index].lower {
			return false
		}
	}
	return true
}
