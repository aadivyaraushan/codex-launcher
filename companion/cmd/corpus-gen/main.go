// Command corpus-gen builds a route-sweep JSONL corpus from the live
// deeplink Wave1 inventory, so the corpus tracks the inventory instead of
// drifting from a hand-maintained copy.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/beepermessage"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/deeplink"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/discovery"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

var verbPhrasing = map[manifest.Verb]string{
	manifest.Play:    "play something on %s",
	manifest.Read:    "check my %s",
	manifest.Order:   "order something from %s",
	manifest.Compose: "send a message on %s",
	manifest.Write:   "add a note in %s",
	manifest.Send:    "send something on %s",
}

// beeperMovedClass and beeperMovedVerb are the class and verb an app lands in
// once Beeper takes it over. They must match what production.go registers for
// beeperSpecs (byClass["beeper_messaging"], and beepermessage's Describe verb
// Send), or a generated "on" label would grade a real move as a misroute.
const (
	beeperMovedClass = capabilityruntime.BeeperMessagingClass
	beeperMovedVerb  = string(manifest.Send)
)

func main() {
	outPath := flag.String("out", "", "JSONL output path (default stdout)")
	flag.Parse()

	if err := run(*outPath, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "corpus-gen:", err)
		os.Exit(1)
	}
}

func run(outPath string, stdout io.Writer) error {
	out := stdout
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("create %s: %w", outPath, err)
		}
		defer f.Close()
		out = f
	}

	for _, c := range generate() {
		line, err := json.Marshal(c)
		if err != nil {
			return fmt.Errorf("marshal case for %s/%s: %w", c.ExpectedApp, c.ExpectedVerb, err)
		}
		if _, err := out.Write(append(line, '\n')); err != nil {
			return fmt.Errorf("write case for %s/%s: %w", c.ExpectedApp, c.ExpectedVerb, err)
		}
	}
	return nil
}

// generate builds Cases from the Wave1 inventory. Most apps get one
// beeper:"both" case per verb. An app Beeper takes over is graded against a
// different label per state, so it gets a beeper:"off" case at its Wave1
// class/verb and a beeper:"on" case at beeper_messaging/send instead.
func generate() []discovery.Case {
	moved := beeperMovedIDs()
	var cases []discovery.Case
	for _, spec := range deeplink.Wave1Specs() {
		for _, verb := range spec.Verbs {
			utterance := phrase(verb, spec.AppName)
			if moved[spec.ID] {
				cases = append(cases,
					discovery.Case{
						Utterance:     utterance,
						ExpectedApp:   spec.ID,
						ExpectedClass: spec.AppClass,
						ExpectedVerb:  string(verb),
						Beeper:        "off",
						Note:          "auto: " + spec.ID + " " + string(verb) + " coverage (Beeper off)",
					},
					discovery.Case{
						Utterance:     utterance,
						ExpectedApp:   spec.ID,
						ExpectedClass: beeperMovedClass,
						ExpectedVerb:  beeperMovedVerb,
						Beeper:        "on",
						Note:          "auto: " + spec.ID + " taken over by Beeper (beeper_messaging/send)",
					},
				)
				continue
			}
			cases = append(cases, discovery.Case{
				Utterance:     utterance,
				ExpectedApp:   spec.ID,
				ExpectedClass: spec.AppClass,
				ExpectedVerb:  string(verb),
				Beeper:        "both",
				Note:          "auto: " + spec.ID + " " + string(verb) + " coverage",
			})
		}
	}
	return cases
}

// beeperMovedIDs is the set of app ids Beeper takes over when it connects,
// read from the same source production.go uses (beepermessage.ProductionSpecs),
// so the generator can't drift from which apps actually move.
func beeperMovedIDs() map[string]bool {
	moved := map[string]bool{}
	for _, spec := range beepermessage.ProductionSpecs() {
		moved[spec.ID] = true
	}
	return moved
}

func phrase(verb manifest.Verb, appName string) string {
	if tmpl, ok := verbPhrasing[verb]; ok {
		return fmt.Sprintf(tmpl, appName)
	}
	return fmt.Sprintf("do %s on %s", verb, appName)
}
