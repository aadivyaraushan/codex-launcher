// Package applenotes is Operator's RT-6 proving adapter: it reads and
// writes Apple Notes on the owner's own Mac by running AppleScript through
// osascript. There is no token and no vendor account anywhere in this
// route — auth is local, consent is class A, and the only thing the
// adapter ever touches is a folder it created itself.
package applenotes

import (
	"context"
	"errors"
	"fmt"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// ID is this adapter's manifest id.
const ID = "apple-notes"

// Folder is the name of the folder this adapter creates and owns. Every
// write lands here and nowhere else — a scheduled write must never land in
// something the user made.
const Folder = "Operator"

// Script-name constants. Each one names both which AppleScript template to
// run and, in the fake runner used by tests, which canned response to hand
// back.
const (
	ScriptFolderExists = "folder_exists"
	ScriptCreateFolder = "create_folder"
	ScriptCreateNote   = "create_note"
	ScriptReadNotes    = "read_notes"
)

// Fixed AppleScript templates. Every one takes its input through "on run
// argv" and indexes into argv — never through string interpolation — so
// nothing a caller passes in Script.Args can change what the script does.
const (
	scriptFolderExistsSource = `on run argv
	set folderName to item 1 of argv
	tell application "Notes"
		if exists folder folderName then
			return "true"
		else
			return "false"
		end if
	end tell
end run
`

	scriptCreateFolderSource = `on run argv
	set folderName to item 1 of argv
	tell application "Notes"
		make new folder with properties {name:folderName}
	end tell
end run
`

	scriptCreateNoteSource = `on run argv
	set folderName to item 1 of argv
	set noteTitle to item 2 of argv
	set noteBody to item 3 of argv
	tell application "Notes"
		tell folder folderName
			make new note with properties {name:noteTitle, body:noteBody}
		end tell
	end tell
end run
`

	scriptReadNotesSource = `on run argv
	set folderName to item 1 of argv
	set output to ""
	tell application "Notes"
		tell folder folderName
			repeat with n in notes
				set output to output & (name of n) & linefeed
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

// ErrForeignFolder is returned when a write names a folder other than the
// one this adapter owns. This is the general tier-1 rule made concrete: a
// scheduled write must never land in something the user made — silent,
// repeating, hard to undo.
var ErrForeignFolder = errors.New("apple notes: a write must target the adapter's own folder")

// Adapter is Operator's Apple Notes adapter.
type Adapter struct {
	runner ScriptRunner
}

// New returns an Adapter backed by the given ScriptRunner.
func New(runner ScriptRunner) (*Adapter, error) {
	if runner == nil {
		return nil, errors.New("apple notes: runner must not be nil")
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
		// android, not both: no iPhone has run any part of this yet.
		Platform:      manifest.PlatformAndroid,
		ProvesCeiling: "TestTheFirstWriteCreatesTheAdaptersOwnFolder",
		Unshipped: "no build registers this adapter; the only place it is constructed is the " +
			"owner-only proveadapter command (cmd/proveadapter/main.go:114, :218). It drives " +
			"Notes.app through osascript, which macOS refuses until someone clicks Allow in the " +
			"Automation privacy pane on the Mac itself, and an unattended serve has no way to " +
			"obtain that click. Written down here so a finished, tested adapter that no user can " +
			"reach is a stated decision rather than something nobody noticed.",
	}
}

// Resolve implements adapter.Adapter.
func (a *Adapter) Resolve(_ context.Context, in adapter.Intent) (adapter.Plan, error) {
	if !a.Describe().Allows(in.Verb) {
		return adapter.Plan{}, fmt.Errorf("apple notes: verb %q is not offered", in.Verb)
	}

	switch in.Verb {
	case manifest.Write:
		if f, ok := in.Fields["folder"]; ok && f != Folder {
			return adapter.Plan{}, fmt.Errorf("%w: %q", ErrForeignFolder, f)
		}
		title := in.Subject
		body := in.Body
		return adapter.Plan{
			AdapterID: ID,
			Verb:      manifest.Write,
			Handle:    Folder,
			Summary:   fmt.Sprintf("Create a note titled %q in %s", title, Folder),
			Details: map[string]string{
				"folder": Folder,
				"title":  title,
				"body":   body,
			},
		}, nil

	case manifest.Read:
		return adapter.Plan{
			AdapterID: ID,
			Verb:      manifest.Read,
			Handle:    Folder,
			Summary:   fmt.Sprintf("Read notes in %s", Folder),
			Details: map[string]string{
				"folder": Folder,
			},
		}, nil

	default:
		// Unreachable given the Allows check above, but keeps this method
		// honest on its own if the manifest ever grows a verb this switch
		// does not handle yet.
		return adapter.Plan{}, fmt.Errorf("apple notes: verb %q is not supported", in.Verb)
	}
}

// Preview implements adapter.Adapter. It runs no script — it only describes
// what Execute would do.
func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	folder := plan.Details["folder"]

	switch plan.Verb {
	case manifest.Write:
		title := plan.Details["title"]
		body := plan.Details["body"]
		return adapter.Preview{
			Plan:     plan,
			Headline: fmt.Sprintf("Create a note in %s", folder),
			Lines: []string{
				fmt.Sprintf("Folder: %s", folder),
				fmt.Sprintf("Title: %s", title),
				fmt.Sprintf("Body: %s", body),
			},
			Confirm: "Create note",
		}, nil

	case manifest.Read:
		return adapter.Preview{
			Plan:     plan,
			Headline: fmt.Sprintf("Read notes in %s", folder),
			Lines:    []string{fmt.Sprintf("Folder: %s", folder)},
			Confirm:  "Read notes",
		}, nil

	default:
		return adapter.Preview{}, fmt.Errorf("apple notes: verb %q is not supported", plan.Verb)
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
		return adapter.Outcome{}, fmt.Errorf("apple notes: verb %q is not supported", plan.Verb)
	}
}

func (a *Adapter) executeWrite(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	folder := plan.Details["folder"]

	exists, err := a.runner.Run(ctx, Script{
		Name:   ScriptFolderExists,
		Source: scriptFolderExistsSource,
		Args:   map[string]string{"folder": folder},
	})
	if err != nil {
		return adapter.Outcome{}, fmt.Errorf("apple notes: checking folder %q: %w", folder, err)
	}
	if exists != "true" {
		if _, err := a.runner.Run(ctx, Script{
			Name:   ScriptCreateFolder,
			Source: scriptCreateFolderSource,
			Args:   map[string]string{"folder": folder},
		}); err != nil {
			return adapter.Outcome{}, fmt.Errorf("apple notes: creating folder %q: %w", folder, err)
		}
	}

	if _, err := a.runner.Run(ctx, Script{
		Name:   ScriptCreateNote,
		Source: scriptCreateNoteSource,
		Args: map[string]string{
			"folder": folder,
			"title":  plan.Details["title"],
			"body":   plan.Details["body"],
		},
	}); err != nil {
		return adapter.Outcome{}, fmt.Errorf("apple notes: creating note: %w", err)
	}

	return adapter.Outcome{Reached: manifest.Completes, Done: true}, nil
}

func (a *Adapter) executeRead(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	folder := plan.Details["folder"]

	out, err := a.runner.Run(ctx, Script{
		Name:   ScriptReadNotes,
		Source: scriptReadNotesSource,
		Args:   map[string]string{"folder": folder},
	})
	if err != nil {
		return adapter.Outcome{}, fmt.Errorf("apple notes: reading notes in %q: %w", folder, err)
	}

	return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: out}, nil
}

// Revoke implements adapter.Adapter. There is no token in this route at
// all, but the consent framework calls Revoke uniformly across every
// adapter, so this always succeeds.
func (a *Adapter) Revoke(_ context.Context) error {
	return nil
}
