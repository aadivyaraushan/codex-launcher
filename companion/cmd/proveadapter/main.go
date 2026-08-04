// Command proveadapter is the hands-on driver for Wave 0's exit test. Unit
// tests cannot show that an adapter works against the real system it talks
// to, or that an adapter someone actually switched off remotely is actually
// unreachable — only a person running this against real hardware and a real
// registry can produce that evidence. Each subcommand prints a plain-text
// transcript, in the order its steps ran, that someone with no knowledge of
// this codebase can read, and ends with a one-line verdict.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/applenotes"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/notion"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: proveadapter <notes|reminders|killswitch|notion|todoist|spotify|beeper|beeper-reconnect|rotate>")
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "notes":
		err = runNotes(os.Args[2:])
	case "reminders":
		err = runReminders(os.Args[2:])
	case "killswitch":
		err = runKillswitch(os.Args[2:])
	case "notion":
		err = runNotion(os.Args[2:])
	case "todoist":
		err = runTodoist(os.Args[2:])
	case "spotify":
		err = runSpotify(os.Args[2:])
	case "beeper":
		err = runBeeper(os.Args[2:])
	case "beeper-reconnect":
		err = runBeeperReconnect(os.Args[2:])
	case "rotate":
		err = runRotate(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q; want notes, reminders, killswitch, notion, todoist, spotify, beeper, beeper-reconnect, or rotate\n", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ---- shared transcript helpers ---------------------------------------------

func step(n int, title string) {
	fmt.Printf("\n[%d] %s\n", n, title)
}

func line(format string, args ...any) {
	fmt.Printf("    %s\n", fmt.Sprintf(format, args...))
}

func verdict(format string, args ...any) {
	fmt.Printf("\nVERDICT: %s\n", fmt.Sprintf(format, args...))
}

func printManifest(m manifest.Manifest) {
	verbs := make([]string, 0, len(m.Verbs))
	for _, v := range m.Verbs {
		verbs = append(verbs, string(v))
	}
	line("runtime:          %s", m.Runtime)
	line("consent class:    %s", m.Consent)
	line("auth:             %s", m.Auth)
	line("verbs:            %s", strings.Join(verbs, ", "))
	line("declared ceiling: %s", m.Ceiling)
}

func printPreview(p adapter.Preview) {
	line("headline: %s", p.Headline)
	for _, l := range p.Lines {
		line("  - %s", l)
	}
	line("confirm:  %s", p.Confirm)
}

func printOutcome(o adapter.Outcome) {
	line("reached: %s   done: %t   handed off to: %q   detail: %s", o.Reached, o.Done, o.HandedOffTo, o.Detail)
}

// ---- notes ------------------------------------------------------------------

// runNotes drives the RT-6 Apple Notes adapter against the real Notes app on
// this Mac. It writes a note, reads it back, and proves that a write aimed
// at a folder the adapter does not own is refused rather than silently
// succeeding.
func runNotes(args []string) error {
	fs := flag.NewFlagSet("notes", flag.ExitOnError)
	note := fs.String("note", "Written by Operator's Wave 0 proving run for the Apple Notes adapter.",
		"body text to write into the proving note")
	foreign := fs.String("foreign", "Personal",
		"a folder this adapter does not own, to prove a write into it is refused")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	a, err := applenotes.New(applenotes.OsascriptRunner{})
	if err != nil {
		return fmt.Errorf("build apple notes adapter: %w", err)
	}
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		return fmt.Errorf("register apple notes adapter: %w", err)
	}
	runner := execution.New(reg)

	step(1, "Describe the adapter")
	m := a.Describe()
	printManifest(m)

	step(2, "Resolve and preview a write — nothing runs yet")
	writePlan, err := runner.Resolve(ctx, adapter.Intent{
		AdapterID: applenotes.ID,
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

	step(3, "Execute the write against the real Notes app")
	writeOutcome, err := runner.Execute(ctx, writePlan, preview.Confirmed())
	if err != nil {
		return fmt.Errorf("execute write: %w", err)
	}
	line("wrote a note titled %q into the %q folder", writePlan.Details["title"], applenotes.Folder)
	printOutcome(writeOutcome)

	step(4, "Resolve and execute a read of the same folder")
	readPlan, err := runner.Resolve(ctx, adapter.Intent{AdapterID: applenotes.ID, Verb: manifest.Read})
	if err != nil {
		return fmt.Errorf("resolve read: %w", err)
	}
	readOutcome, err := runner.Execute(ctx, readPlan, execution.Confirmation{})
	if err != nil {
		return fmt.Errorf("execute read: %w", err)
	}
	line("notes now sitting in the %q folder:", applenotes.Folder)
	for _, title := range strings.Split(strings.TrimSpace(readOutcome.Detail), "\n") {
		if title != "" {
			line("  - %s", title)
		}
	}

	step(5, fmt.Sprintf("Attempt a write into %q, a folder this adapter does not own", *foreign))
	_, err = runner.Resolve(ctx, adapter.Intent{
		AdapterID: applenotes.ID,
		Verb:      manifest.Write,
		Subject:   "Should never be created",
		Fields:    map[string]string{"folder": *foreign},
	})
	if err == nil {
		return fmt.Errorf("write into foreign folder %q was NOT refused — this is exactly the failure this step exists to catch", *foreign)
	}
	if !errors.Is(err, applenotes.ErrForeignFolder) {
		return fmt.Errorf("write into foreign folder %q failed, but for the wrong reason: %w", *foreign, err)
	}
	line("REFUSED, as it should be: %v", err)

	step(6, "Ceiling actually reached")
	line("declared ceiling: %s", m.Ceiling)
	line("measured ceiling, from the write above: %s", writeOutcome.Reached)

	verdict("apple-notes adapter proven against the real Notes app; the foreign-folder write was correctly refused")
	return nil
}

// ---- killswitch ---------------------------------------------------------

// stubNotionSession satisfies notion.Session so a Notion adapter can be
// constructed and registered without ever reaching the network. It is never
// meant to be called: this proving run only needs the adapter to exist and
// be routable, then unreachable once killed.
type stubNotionSession struct{}

func (stubNotionSession) ListTools(context.Context) ([]string, error) {
	return nil, errors.New("stub session: not connected; this proving run should never call it")
}

func (stubNotionSession) Call(context.Context, string, map[string]any) (json.RawMessage, error) {
	return nil, errors.New("stub session: not connected; this proving run should never call it")
}

// runKillswitch registers the real Apple Notes adapter and a constructed
// (but not connected) Notion adapter in one registry, then proves that
// applying a remote kill list actually makes the killed adapter unreachable
// through the registry — and that clearing the kill list restores it.
func runKillswitch(args []string) error {
	fs := flag.NewFlagSet("killswitch", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	notesAdapter, err := applenotes.New(applenotes.OsascriptRunner{})
	if err != nil {
		return fmt.Errorf("build apple notes adapter: %w", err)
	}
	notionAdapter, err := notion.New(stubNotionSession{})
	if err != nil {
		return fmt.Errorf("build notion adapter: %w", err)
	}

	r := registry.New()

	step(1, "Register both adapters and confirm both are present and routable")
	if err := r.Register(notesAdapter); err != nil {
		return fmt.Errorf("register %s: %w", applenotes.ID, err)
	}
	if err := r.Register(notionAdapter); err != nil {
		return fmt.Errorf("register %s: %w", notion.ID, err)
	}
	for _, id := range []string{applenotes.ID, notion.ID} {
		if _, err := r.Get(id); err != nil {
			return fmt.Errorf("%s should be routable right after registering, but Get returned: %w", id, err)
		}
		line("%s: registered and routable", id)
	}

	step(2, fmt.Sprintf("Apply a kill list that switches off %q only", notion.ID))
	reason := "proveadapter killswitch: exercising the remote kill switch by hand"
	changed := r.ApplyKillList(registry.KillList{Entries: []registry.KillEntry{{ID: notion.ID, Reason: reason}}})
	line("reason given: %s", reason)
	line("adapters whose on/off state changed: %v", changed)

	step(3, "Check each adapter's routability, and prove the killed one is refused")
	if _, err := r.Get(applenotes.ID); err != nil {
		return fmt.Errorf("%s should still be routable, but Get returned: %w", applenotes.ID, err)
	}
	line("%s: still routable — untouched by the kill list", applenotes.ID)

	if _, err := r.Get(notion.ID); err == nil {
		return fmt.Errorf("%s should have been refused after being killed, but Get succeeded — exactly the failure this step exists to catch", notion.ID)
	} else if !errors.Is(err, registry.ErrAdapterDisabled) {
		return fmt.Errorf("%s was refused, but for the wrong reason: %w", notion.ID, err)
	} else {
		line("%s: REFUSED via registry.Get — %v", notion.ID, err)
	}
	killReason, off := r.Disabled(notion.ID)
	line("%s: registry reports disabled=%t, reason=%q", notion.ID, off, killReason)
	// android is the caller's platform, not a wildcard: a phone runs on one
	// platform, and the only phone this has ever run on is the Pixel.
	line("adapters still offering \"write\": %s", adapterIDs(r.ForVerb(manifest.Write, manifest.PlatformAndroid)))

	step(4, "Apply an empty kill list and show the adapter comes back")
	changed = r.ApplyKillList(registry.KillList{})
	line("adapters whose on/off state changed: %v", changed)
	if _, err := r.Get(notion.ID); err != nil {
		return fmt.Errorf("%s should be routable again after an empty kill list, but Get returned: %w", notion.ID, err)
	}
	line("%s: routable again", notion.ID)

	verdict("kill switch proven: %s was switched off remotely and unreachable through the registry, then restored", notion.ID)
	return nil
}

func adapterIDs(as []adapter.Adapter) string {
	ids := make([]string, 0, len(as))
	for _, a := range as {
		ids = append(ids, a.Describe().ID)
	}
	return strings.Join(ids, ", ")
}
