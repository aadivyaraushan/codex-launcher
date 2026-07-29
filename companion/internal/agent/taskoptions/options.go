package taskoptions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxModels           = 32
	maxReasoningOptions = 16
)

var ErrInvalidCatalog = errors.New("Codex new-task option catalog is invalid")

type Catalog struct {
	Models          []Model          `json:"models"`
	PermissionModes []PermissionMode `json:"permissionModes"`
}

type Model struct {
	ID                 string      `json:"id"`
	WireName           string      `json:"-"`
	DisplayName        string      `json:"displayName"`
	Default            bool        `json:"isDefault"`
	DefaultReasoningID string      `json:"defaultReasoningId"`
	Reasoning          []Reasoning `json:"reasoning"`
}

type Reasoning struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

type PermissionMode struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Default     bool   `json:"isDefault"`
}

type modelListResponse struct {
	Data []struct {
		ID                     string `json:"id"`
		WireName               string `json:"model"`
		DisplayName            string `json:"displayName"`
		Hidden                 bool   `json:"hidden"`
		Default                bool   `json:"isDefault"`
		DefaultReasoningEffort string `json:"defaultReasoningEffort"`
		Reasoning              []struct {
			ID          string `json:"reasoningEffort"`
			Description string `json:"description"`
		} `json:"supportedReasoningEfforts"`
	} `json:"data"`
}

func Load(ctx context.Context, list func(context.Context) (json.RawMessage, error)) (Catalog, error) {
	if list == nil {
		return Catalog{}, ErrInvalidCatalog
	}
	raw, err := list(ctx)
	if err != nil {
		return Catalog{}, err
	}
	var response modelListResponse
	if json.Unmarshal(raw, &response) != nil {
		return Catalog{}, ErrInvalidCatalog
	}
	models := make([]Model, 0, len(response.Data))
	seenModels := make(map[string]struct{})
	defaultCount := 0
	for _, candidate := range response.Data {
		if candidate.Hidden {
			continue
		}
		if len(models) >= maxModels || !safeID(candidate.ID) || !safeID(candidate.WireName) || !safeText(candidate.DisplayName, 128) ||
			!safeID(candidate.DefaultReasoningEffort) || len(candidate.Reasoning) == 0 || len(candidate.Reasoning) > maxReasoningOptions {
			return Catalog{}, ErrInvalidCatalog
		}
		if _, duplicate := seenModels[candidate.ID]; duplicate {
			return Catalog{}, ErrInvalidCatalog
		}
		seenModels[candidate.ID] = struct{}{}
		reasoning := make([]Reasoning, 0, len(candidate.Reasoning))
		seenReasoning := make(map[string]struct{})
		defaultPresent := false
		for _, option := range candidate.Reasoning {
			if !safeID(option.ID) || !safeText(option.Description, 512) {
				return Catalog{}, ErrInvalidCatalog
			}
			if _, duplicate := seenReasoning[option.ID]; duplicate {
				return Catalog{}, ErrInvalidCatalog
			}
			seenReasoning[option.ID] = struct{}{}
			defaultPresent = defaultPresent || option.ID == candidate.DefaultReasoningEffort
			reasoning = append(reasoning, Reasoning{ID: option.ID, DisplayName: displayID(option.ID), Description: option.Description})
		}
		if !defaultPresent {
			return Catalog{}, ErrInvalidCatalog
		}
		if candidate.Default {
			defaultCount++
		}
		models = append(models, Model{
			ID: candidate.ID, WireName: candidate.WireName, DisplayName: candidate.DisplayName, Default: candidate.Default,
			DefaultReasoningID: candidate.DefaultReasoningEffort, Reasoning: reasoning,
		})
	}
	if len(models) == 0 || defaultCount != 1 {
		return Catalog{}, ErrInvalidCatalog
	}
	return Catalog{Models: models, PermissionModes: permissionModes()}, nil
}

func permissionModes() []PermissionMode {
	return []PermissionMode{
		{ID: "read-only", DisplayName: "Read only", Description: "Codex can inspect the project but cannot change files.", Default: false},
		{ID: "workspace-write", DisplayName: "Workspace", Description: "Codex can read and change files inside the selected project.", Default: true},
		{ID: "danger-full-access", DisplayName: "Full access", Description: "Codex can read and change files anywhere your computer account can access. Approvals still follow the task policy.", Default: false},
	}
}

func displayID(value string) string {
	words := strings.ReplaceAll(value, "-", " ")
	runes := []rune(words)
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func safeID(value string) bool {
	length := utf8.RuneCountInString(value)
	return length >= 1 && length <= 128 && !strings.ContainsFunc(value, unicode.IsControl) && strings.TrimSpace(value) == value
}

func safeText(value string, maximum int) bool {
	length := utf8.RuneCountInString(value)
	return length >= 1 && length <= maximum && !strings.ContainsFunc(value, unicode.IsControl) && strings.TrimSpace(value) == value
}
