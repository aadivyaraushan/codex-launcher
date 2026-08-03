package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/applereminders"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

// runReminders drives the RT-6 Apple Reminders adapter against the real
// Reminders app on this Mac. It writes a reminder, reads it back, and proves
// that a write aimed at a list the adapter does not own is refused rather
// than silently succeeding.
func runReminders(args []string) error {
	fs := flag.NewFlagSet("reminders", flag.ExitOnError)
	note := fs.String("reminder", "Written by Operator's Wave 1 proving run for the Apple Reminders adapter.",
		"body text to write into the proving reminder")
	foreign := fs.String("foreign", "Personal",
		"a list this adapter does not own, to prove a write into it is refused")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	a, err := applereminders.New(applereminders.OsascriptRunner{})
	if err != nil {
		return fmt.Errorf("build apple reminders adapter: %w", err)
	}
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		return fmt.Errorf("register apple reminders adapter: %w", err)
	}
	runner := execution.New(reg)

	step(1, "Describe the adapter")
	m := a.Describe()
	printManifest(m)

	step(2, "Resolve and preview a write — nothing runs yet")
	writePlan, err := runner.Resolve(ctx, adapter.Intent{
		AdapterID: applereminders.ID,
		Verb:      manifest.Write,
		Subject:   "Operator proving run",
		Body:      *note,
	})
	if err != nil {
		return fmt.Errorf("resolve write: %w", err)
	}
	preview, err := runner.Preview(ctx, writePlan)
	if err != nil {
		return fmt.Errorf("preview write: %w", err)
	}
	line("this is what would happen — read it before anything runs:")
	printPreview(preview.Preview)

	step(3, "Execute the write against the real Reminders app")
	writeOutcome, err := runner.Execute(ctx, writePlan, preview.Confirmed())
	if err != nil {
		return fmt.Errorf("execute write: %w", err)
	}
	line("wrote a reminder titled %q into the %q list", writePlan.Details["title"], applereminders.List)
	printOutcome(writeOutcome)

	step(4, "Resolve and execute a read of the same list")
	readPlan, err := runner.Resolve(ctx, adapter.Intent{AdapterID: applereminders.ID, Verb: manifest.Read})
	if err != nil {
		return fmt.Errorf("resolve read: %w", err)
	}
	readOutcome, err := runner.Execute(ctx, readPlan, execution.Confirmation{})
	if err != nil {
		return fmt.Errorf("execute read: %w", err)
	}
	line("reminders now sitting in the %q list:", applereminders.List)
	for _, title := range strings.Split(strings.TrimSpace(readOutcome.Detail), "\n") {
		if title != "" {
			line("  - %s", title)
		}
	}

	step(5, fmt.Sprintf("Attempt a write into %q, a list this adapter does not own", *foreign))
	_, err = runner.Resolve(ctx, adapter.Intent{
		AdapterID: applereminders.ID,
		Verb:      manifest.Write,
		Subject:   "Should never be created",
		Fields:    map[string]string{"list": *foreign},
	})
	if err == nil {
		return fmt.Errorf("write into foreign list %q was NOT refused — this is exactly the failure this step exists to catch", *foreign)
	}
	if !errors.Is(err, applereminders.ErrForeignList) {
		return fmt.Errorf("write into foreign list %q failed, but for the wrong reason: %w", *foreign, err)
	}
	line("REFUSED, as it should be: %v", err)

	step(6, "Ceiling actually reached")
	line("declared ceiling: %s", m.Ceiling)
	line("measured ceiling, from the write above: %s", writeOutcome.Reached)

	verdict("apple-reminders adapter proven against the real Reminders app; the foreign-list write was correctly refused")
	return nil
}
