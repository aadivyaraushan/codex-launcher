package modelauth

import (
	"encoding/json"
	"strings"
)

type Status string

const (
	OauthReady Status = "oauth_ready"
	Missing    Status = "missing"
	Pending    Status = "pending"
)

type Profile struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Provider string `json:"provider"`
}

func ParseStoreJSON(raw []byte) ([]Profile, error) {
	var mapped struct {
		Profiles map[string]json.RawMessage `json:"profiles"`
	}
	if err := json.Unmarshal(raw, &mapped); err == nil && mapped.Profiles != nil {
		out := make([]Profile, 0, len(mapped.Profiles))
		for id, item := range mapped.Profiles {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			var row struct {
				Type     string `json:"type"`
				Provider string `json:"provider"`
			}
			if json.Unmarshal(item, &row) != nil {
				continue
			}
			out = append(out, Profile{
				ID:       id,
				Type:     strings.TrimSpace(row.Type),
				Provider: strings.TrimSpace(row.Provider),
			})
		}
		return out, nil
	}
	return ParseAuthListJSON(raw)
}

func OpenAIProfiles(profiles []Profile) []Profile {
	out := make([]Profile, 0, len(profiles))
	for _, profile := range profiles {
		if IsOpenAIProfile(profile) {
			out = append(out, profile)
		}
	}
	return out
}

func IsOpenAIProfile(profile Profile) bool {
	provider := strings.ToLower(strings.TrimSpace(profile.Provider))
	id := strings.ToLower(strings.TrimSpace(profile.ID))
	return strings.HasPrefix(provider, "openai") || strings.HasPrefix(id, "openai")
}

func Classify(profiles []Profile, loginPending bool) Status {
	if hasOAuth(profiles) {
		return OauthReady
	}
	if loginPending {
		return Pending
	}
	return Missing
}

func hasOAuth(profiles []Profile) bool {
	for _, profile := range profiles {
		if IsOAuthType(profile.Type) {
			return true
		}
	}
	return false
}

func IsOAuthType(raw string) bool {
	kind := strings.ToLower(strings.TrimSpace(raw))
	if kind == "" || kind == "api_key" || kind == "apikey" || kind == "api-key" || kind == "key" {
		return false
	}
	return kind == "oauth" || strings.HasPrefix(kind, "oauth")
}

func ParseAuthListJSON(raw []byte) ([]Profile, error) {
	var envelope struct {
		Profiles []json.RawMessage `json:"profiles"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(envelope.Profiles))
	for _, item := range envelope.Profiles {
		var row struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Provider string `json:"provider"`
		}
		if err := json.Unmarshal(item, &row); err != nil {
			continue
		}
		id := strings.TrimSpace(row.ID)
		if id == "" {
			continue
		}
		out = append(out, Profile{
			ID:       id,
			Type:     strings.TrimSpace(row.Type),
			Provider: strings.TrimSpace(row.Provider),
		})
	}
	return out, nil
}

func PreferOrder(profiles []Profile) []string {
	oauth := make([]string, 0, len(profiles))
	other := make([]string, 0, len(profiles))
	seen := map[string]bool{}
	appendID := func(dst []string, id string) []string {
		if id == "" || seen[id] {
			return dst
		}
		seen[id] = true
		return append(dst, id)
	}
	for _, profile := range profiles {
		if IsOAuthType(profile.Type) {
			oauth = appendID(oauth, profile.ID)
		}
	}
	for _, profile := range profiles {
		if !IsOAuthType(profile.Type) {
			other = appendID(other, profile.ID)
		}
	}
	return append(oauth, other...)
}
