// Package applereminders is Operator's RT-6 proving adapter: it reads and
// writes Apple Reminders on the owner's own Mac by running AppleScript through
// osascript. There is no token and no vendor account anywhere in this
// route — auth is local, consent is class A, and the only thing the
// adapter ever touches is a list it created itself.
package applereminders

import (
	"context"
	"errors"
	"fmt"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// ID is this adapter's manifest id.
const ID = "apple-reminders"

// List is the name of the Reminders list this adapter creates and owns. Every
// write lands here and nowhere else — a scheduled write must never land in
// something the user made.
const List = "Operator"

// Script-name constants. Each one names both which AppleScript template to
// run and, in the fake runner used by tests, which canned response to hand
// back.
const (
	ScriptListExists     = "list_exists"
	ScriptCreateList     = "create_list"
	ScriptCreateReminder = "create_reminder"
	ScriptReadReminders  = "read_reminders"
)

// Fixed AppleScript templates. Every one takes its input through "on run
// argv" and indexes into argv — never through string interpolation — so
// nothing a caller passes in Script.Args can change what the script does.
const (
	scriptListExistsSource = `on run argv
	set listName to item 1 of argv
	tell application "Reminders"
		if exists list listName then
			return "true"
		else
			return "false"
		end if
	end tell
end run
`

	scriptCreateListSource = `on run argv
	set listName to item 1 of argv
	tell application "Reminders"
		make new list with properties {name:listName}
	end tell
end run
`

	scriptCreateReminderSource = `on run argv
	set listName to item 1 of argv
	set reminderName to item 2 of argv
	set reminderBody to item 3 of argv
	tell application "Reminders"
		tell list listName
			make new reminder with properties {name:reminderName, body:reminderBody}
		end tell
	end tell
end run
`

	scriptReadRemindersSource = `on run argv
	set listName to item 1 of argv
	set output to ""
	tell application "Reminders"
		tell list listName
			repeat with r in reminders
				set output to output & (name of r) & linefeed
			end repeat
		end tell
	end tell
	return output
end run
`
)

// Script is one AppleScript run to make. Source is a fixed template
// containing no user text; user text travels in Args and reaches the
// script only as an osascript argv parameter.
type Script struct {
	Name   string
	Source string
	Args   map[string]string
}

// ScriptRunner runs a Script and returns its trimmed stdout. The real
// implementation (OsascriptRunner, in runner.go) shells out to osascript;
// tests substitute a recording fake.
type ScriptRunner interface {
	Run(context.Context, Script) (string, error)
}

// ErrForeignList is returned when a write names a list other than the
// one this adapter owns. This is the general tier-1 rule made concrete: a
// scheduled write must never land in something the user made — silent,
// repeating, hard to undo.
var ErrForeignList = errors.New("apple reminders: a write must target the adapter's own list")

// Adapter is Operator's Apple Reminders adapter.
type Adapter struct {
	runner ScriptRunner
}

// New returns an Adapter backed by the given ScriptRunner.
func New(runner ScriptRunner) (*Adapter, error) {
	if runner == nil {
		return nil, errors.New("apple reminders: runner must not be nil")
	}
	return &Adapter{runner: runner}, nil
}

// Describe implements adapter.Adapter.
func (a *Adapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID:            ID,
		Runtime:       manifest.RT6,
		Verbs:         []manifest.Verb{manifest.Read, manifest.Write},
		Ceiling:       manifest.Completes,
		Consent:       manifest.ConsentA,
		Auth:          manifest.AuthLocal,
		Cost:          manifest.CostFree,
		Gates:         []manifest.Gate{manifest.GateNone},
		Capacity:      manifest.Capacity{Kind: manifest.CapacityNone},
		Region:        []string{"global"},
		// android, not both: same quirk as Apple Notes — no iPhone has run this yet.
		Platform:      manifest.PlatformAndroid,
		ProvesCeiling: "TestTheFirstWriteCreatesTheAdaptersOwnList",
		Unshipped: "no build registers this adapter; the only place it is constructed is the " +
			"owner-only proveadapter command (cmd/proveadapter/reminders.go:32). It drives " +
			"Reminders.app through osascript, which macOS refuses until someone clicks Allow in " +
			"the Automation privacy pane on the Mac itself, and an unattended serve has no way " +
			"to obtain that click. Written down here so a finished, tested adapter that no user " +
			"can reach is a stated decision rather than something nobody noticed.",
	}
}

// Resolve implements adapter.Adapter.
func (a *Adapter) Resolve(_ context.Context, in adapter.Intent) (adapter.Plan, error) {
	if !a.Describe().Allows(in.Verb) {
		return adapter.Plan{}, fmt.Errorf("apple reminders: verb %q is not offered", in.Verb)
	}

	switch in.Verb {
	case manifest.Write:
		if l, ok := in.Fields["list"]; ok && l != List {
			return adapter.Plan{}, fmt.Errorf("%w: %q", ErrForeignList, l)
		}
		title := in.Subject
		body := in.Body
		return adapter.Plan{
			AdapterID: ID,
			Verb:      manifest.Write,
			Handle:    List,
			Summary:   fmt.Sprintf("Create a reminder titled %q in %s", title, List),
			Details: map[string]string{
				"list":  List,
				"title": title,
				"body":  body,
			},
		}, nil

	case manifest.Read:
		return adapter.Plan{
			AdapterID: ID,
			Verb:      manifest.Read,
			Handle:    List,
			Summary:   fmt.Sprintf("Read reminders in %s", List),
			Details: map[string]string{
				"list": List,
			},
		}, nil

	default:
		return adapter.Plan{}, fmt.Errorf("apple reminders: verb %q is not supported", in.Verb)
	}
}

// Preview implements adapter.Adapter. It runs no script — it only describes
// what Execute would do.
func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	list := plan.Details["list"]

	switch plan.Verb {
	case manifest.Write:
		title := plan.Details["title"]
		body := plan.Details["body"]
		return adapter.Preview{
			Plan:     plan,
			Headline: fmt.Sprintf("Create a reminder in %s", list),
			Lines: []string{
				fmt.Sprintf("List: %s", list),
				fmt.Sprintf("Title: %s", title),
				fmt.Sprintf("Body: %s", body),
			},
			Confirm: "Create reminder",
		}, nil

	case manifest.Read:
		return adapter.Preview{
			Plan:     plan,
			Headline: fmt.Sprintf("Read reminders in %s", list),
			Lines:    []string{fmt.Sprintf("List: %s", list)},
			Confirm:  "Read reminders",
		}, nil

	default:
		return adapter.Preview{}, fmt.Errorf("apple reminders: verb %q is not supported", plan.Verb)
	}
}

// Execute implements adapter.Adapter.
func (a *Adapter) Execute(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	switch plan.Verb {
	case manifest.Write:
		return a.executeWrite(ctx, plan)
	case manifest.Read:
		return a.executeRead(ctx, plan)
	default:
		return adapter.Outcome{}, fmt.Errorf("apple reminders: verb %q is not supported", plan.Verb)
	}
}

func (a *Adapter) executeWrite(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	list := plan.Details["list"]

	exists, err := a.runner.Run(ctx, Script{
		Name:   ScriptListExists,
		Source: scriptListExistsSource,
		Args:   map[string]string{"list": list},
	})
	if err != nil {
		return adapter.Outcome{}, fmt.Errorf("apple reminders: checking list %q: %w", list, err)
	}
	if exists != "true" {
		if _, err := a.runner.Run(ctx, Script{
			Name:   ScriptCreateList,
			Source: scriptCreateListSource,
			Args:   map[string]string{"list": list},
		}); err != nil {
			return adapter.Outcome{}, fmt.Errorf("apple reminders: creating list %q: %w", list, err)
		}
	}

	if _, err := a.runner.Run(ctx, Script{
		Name:   ScriptCreateReminder,
		Source: scriptCreateReminderSource,
		Args: map[string]string{
			"list":  list,
			"title": plan.Details["title"],
			"body":  plan.Details["body"],
		},
	}); err != nil {
		return adapter.Outcome{}, fmt.Errorf("apple reminders: creating reminder: %w", err)
	}

	return adapter.Outcome{Reached: manifest.Completes, Done: true}, nil
}

func (a *Adapter) executeRead(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	list := plan.Details["list"]

	out, err := a.runner.Run(ctx, Script{
		Name:   ScriptReadReminders,
		Source: scriptReadRemindersSource,
		Args:   map[string]string{"list": list},
	})
	if err != nil {
		return adapter.Outcome{}, fmt.Errorf("apple reminders: reading reminders in %q: %w", list, err)
	}

	return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: out}, nil
}

// Revoke implements adapter.Adapter. There is no token in this route at
// all, but the consent framework calls Revoke uniformly across every
// adapter, so this always succeeds.
func (a *Adapter) Revoke(_ context.Context) error {
	return nil
}
