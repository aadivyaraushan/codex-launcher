package main

import (
	"testing"

	slackadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/slack"
)

func TestValidateSlackWorkspaceAcceptsSelectedHost(t *testing.T) {
	identity := slackadapter.WorkspaceIdentity{URL: "https://aadivyasagents.slack.com/", Team: "aadivya's agents"}
	if err := validateSlackWorkspace(identity, "aadivyasagents.slack.com"); err != nil {
		t.Fatalf("validate selected workspace: %v", err)
	}
}

func TestValidateSlackWorkspaceRefusesDifferentHost(t *testing.T) {
	identity := slackadapter.WorkspaceIdentity{URL: "https://another-workspace.slack.com/", Team: "another workspace"}
	if err := validateSlackWorkspace(identity, "aadivyasagents.slack.com"); err == nil {
		t.Fatal("validate different workspace returned nil")
	}
}
