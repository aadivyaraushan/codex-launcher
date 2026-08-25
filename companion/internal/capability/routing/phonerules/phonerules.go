// Package phonerules builds the stage1 explicit-app rules the phone routes
// with. It is Beeper-aware: when Beeper is connected, the apps it takes over
// (beepermessage.ProductionSpecs) are emitted under the class and verb they
// are actually registered under (capabilityruntime.BeeperMessagingClass /
// manifest.Send), not their stale Wave1 messaging/compose label — so
// route.AppClass can never go stale and orphan in stage2.
package phonerules

import (
	beepermessage "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/beepermessage"
	deeplinkadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/deeplink"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	stage1explicit "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/explicit"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

// Build returns one explicit-app rule per Wave1Specs entry. When beeperOn is
// true, the apps beepermessage.ProductionSpecs names are emitted under
// BeeperMessagingClass with verb Send instead of their Wave1 label.
func Build(beeperOn bool) []stage1explicit.Rule {
	moved := map[string]bool{}
	if beeperOn {
		for _, spec := range beepermessage.ProductionSpecs() {
			moved[spec.ID] = true
		}
	}
	specs := deeplinkadapter.Wave1Specs()
	rules := make([]stage1explicit.Rule, 0, len(specs))
	for _, spec := range specs {
		if moved[spec.ID] {
			rules = append(rules, stage1explicit.Rule{
				ID:       spec.ID,
				Name:     spec.AppName,
				AppClass: capabilityruntime.BeeperMessagingClass,
				Verbs:    []manifest.Verb{manifest.Send},
			})
			continue
		}
		rules = append(rules, stage1explicit.Rule{ID: spec.ID, Name: spec.AppName, AppClass: spec.AppClass, Verbs: spec.Verbs})
	}
	return rules
}
