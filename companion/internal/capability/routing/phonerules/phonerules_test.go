package phonerules

import (
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	stage1explicit "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/explicit"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

func find(rules []stage1explicit.Rule, id string) *stage1explicit.Rule {
	for i := range rules {
		if rules[i].ID == id {
			return &rules[i]
		}
	}
	return nil
}

func hasVerb(rule *stage1explicit.Rule, verb manifest.Verb) bool {
	for _, v := range rule.Verbs {
		if v == verb {
			return true
		}
	}
	return false
}

// With Beeper off, the phone routes every app at its plain Wave1 label; a
// messaging app is messaging/compose.
func TestBuildIsBeeperBlindWhenBeeperIsOff(t *testing.T) {
	rules := Build(false)
	discord := find(rules, "discord")
	if discord == nil {
		t.Fatal("discord missing with beeper off")
	}
	if discord.AppClass != "messaging" || !hasVerb(discord, manifest.Compose) {
		t.Fatalf("discord off = %+v, want messaging/compose", discord)
	}
}

// With Beeper on, the apps Beeper takes over are emitted under the class and
// verb they are actually registered under, so route.AppClass can never go
// stale and orphan in stage2. Apps Beeper does not take over are untouched.
func TestBuildMovesBeeperAppsWhenBeeperIsOn(t *testing.T) {
	rules := Build(true)
	for _, id := range []string{"discord", "messages"} {
		rule := find(rules, id)
		if rule == nil {
			t.Fatalf("%s missing with beeper on", id)
		}
		if rule.AppClass != capabilityruntime.BeeperMessagingClass {
			t.Errorf("%s on: class=%q, want %q", id, rule.AppClass, capabilityruntime.BeeperMessagingClass)
		}
		if len(rule.Verbs) != 1 || rule.Verbs[0] != manifest.Send {
			t.Errorf("%s on: verbs=%v, want [send]", id, rule.Verbs)
		}
	}
	whatsapp := find(rules, "whatsapp")
	if whatsapp == nil || whatsapp.AppClass != "messaging" {
		t.Errorf("whatsapp moved unexpectedly: %+v", whatsapp)
	}
}

// instagram is Beeper-moved but is not in Wave1Specs (it has a bespoke
// off-state adapter), so the phone's keyword table has no rule for it in
// either state. The LLM router, not this table, is what routes instagram.
func TestBuildOmitsInstagramInBothStates(t *testing.T) {
	if find(Build(true), "instagram") != nil {
		t.Error("instagram should not be in phone rules with beeper on")
	}
	if find(Build(false), "instagram") != nil {
		t.Error("instagram should not be in phone rules with beeper off")
	}
}
