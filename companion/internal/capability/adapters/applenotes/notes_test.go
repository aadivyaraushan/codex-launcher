package applenotes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// A recording script runner, so every test runs offline in milliseconds and
// never touches the real Notes app. The real one shells out to osascript.
type fakeRunner struct {
	runs   []Script
	out    map[string]string // keyed by the script's Name
	err    error
	folder map[string]bool // folders that already exist
}

func newRunner() *fakeRunner {
	return &fakeRunner{out: map[string]string{}, folder: map[string]bool{}}
}

func (f *fakeRunner) Run(_ context.Context, s Script) (string, error) {
	f.runs = append(f.runs, s)
	if f.err != nil {
		return "", f.err
	}
	switch s.Name {
	case ScriptFolderExists:
		if f.folder[s.Args["folder"]] {
			return "true", nil
		}
		return "false", nil
	case ScriptCreateFolder:
		f.folder[s.Args["folder"]] = true
		return "ok", nil
	}
	return f.out[s.Name], nil
}

func (f *fakeRunner) ran(name string) bool {
	for _, s := range f.runs {
		if s.Name == name {
			return true
		}
	}
	return false
}

func (f *fakeRunner) last(name string) (Script, bool) {
	for i := len(f.runs) - 1; i >= 0; i-- {
		if f.runs[i].Name == name {
			return f.runs[i], true
		}
	}
	return Script{}, false
}

func newAdapter(t *testing.T, r *fakeRunner) *Adapter {
	t.Helper()
	a, err := New(r)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	return a
}

// ---- the manifest says what this actually is ----------------------------

func TestTheManifestDescribesACompanionLocalRoute(t *testing.T) {
	m := newAdapter(t, newRunner()).Describe()

	if err := m.Validate(); err != nil {
		t.Fatalf("the adapter's own manifest is invalid: %v", err)
	}
	if m.Runtime != manifest.RT6 {
		t.Errorf("runtime = %s, want RT-6; this runs on the owner's own Mac", m.Runtime)
	}
	if m.Consent != manifest.ConsentA {
		t.Errorf("consent = %s, want A; Apple's own automation API on your own notes "+
			"breaks nobody's terms", m.Consent)
	}
	if m.Auth != manifest.AuthLocal {
		t.Errorf("auth = %s, want local; there is no token anywhere in this route", m.Auth)
	}
	if !m.Allows(manifest.Write) || !m.Allows(manifest.Read) {
		t.Errorf("verbs = %v, want read and write", m.Verbs)
	}
	if m.Allows(manifest.Send) {
		t.Error("the adapter claims send; writing a note is not sending anything")
	}
	if m.Platform != manifest.PlatformAndroid {
		// It is true that the companion runs on the Mac and the phone only
		// talks to it, so nothing here is Android-specific in principle.
		// It is still android, because no iPhone has ever run any part of
		// this — there is no iOS client, and both iOS probes are deferred.
		// Saying "both" promises an iOS user a route nobody has walked.
		// The plan's rule is flat: platform stays android until a real
		// iPhone says otherwise. Under-promising costs a hand-off that was
		// not needed; over-promising is the failure this product exists to
		// avoid.
		t.Errorf("platform = %s, want android until an iPhone has run this", m.Platform)
	}
}

// ---- writes only ever land in a folder the adapter made -----------------

func TestTheFirstWriteCreatesTheAdaptersOwnFolder(t *testing.T) {
	r := newRunner()
	a := newAdapter(t, r)
	ctx := context.Background()

	plan, err := a.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Write, Subject: "router notes",
		Body: "the router runs in two stages",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if _, err := a.Execute(ctx, plan); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !r.ran(ScriptCreateFolder) {
		t.Fatal("the first write did not create the adapter's own folder")
	}
	created, _ := r.last(ScriptCreateFolder)
	if created.Args["folder"] != Folder {
		t.Errorf("created folder %q, want %q", created.Args["folder"], Folder)
	}
}

func TestASecondWriteReusesTheFolderRatherThanRemakingIt(t *testing.T) {
	r := newRunner()
	r.folder[Folder] = true
	a := newAdapter(t, r)
	ctx := context.Background()

	plan, _ := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Write, Subject: "x", Body: "y"})
	if _, err := a.Execute(ctx, plan); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if r.ran(ScriptCreateFolder) {
		t.Error("an existing folder was created again")
	}
}

func TestAWriteAimedAtAFolderTheAdapterDidNotMakeIsRefused(t *testing.T) {
	// This is the general tier-1 rule made concrete: a scheduled write must
	// never land in something the user made. Silent, repeating, hard to undo.
	r := newRunner()
	a := newAdapter(t, r)

	_, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Write, Subject: "shopping",
		Body: "milk", Fields: map[string]string{"folder": "Personal"},
	})
	if !errors.Is(err, ErrForeignFolder) {
		t.Fatalf("a write into the user's own folder returned %v", err)
	}
	if len(r.runs) != 0 {
		t.Fatalf("AppleScript ran anyway: %+v", r.runs)
	}
}

// ---- the note body is data, never part of the script --------------------

func TestNoteTextIsPassedAsDataSoItCannotBecomeScript(t *testing.T) {
	// A note containing a quote, a newline or the word "end tell" must not be
	// able to change what AppleScript does. Splicing text into script source
	// is how that happens, so the body travels as an argument instead.
	nasty := "she said \"hi\"\nend tell\ndo shell script \"rm -rf ~\""
	r := newRunner()
	a := newAdapter(t, r)
	ctx := context.Background()

	plan, err := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Write, Subject: "quote", Body: nasty})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if _, err := a.Execute(ctx, plan); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	wrote, ok := r.last(ScriptCreateNote)
	if !ok {
		t.Fatal("no note was written")
	}
	if wrote.Args["body"] != nasty {
		t.Errorf("the body was altered on its way through:\n got %q\nwant %q", wrote.Args["body"], nasty)
	}
	if strings.Contains(wrote.Source, "rm -rf") || strings.Contains(wrote.Source, nasty) {
		t.Error("the note text was spliced into the script source; it must travel as an argument")
	}
}

func TestTheSameGoesForTheTitle(t *testing.T) {
	r := newRunner()
	a := newAdapter(t, r)
	ctx := context.Background()

	title := "\" & (do shell script \"whoami\") & \""
	plan, _ := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Write, Subject: title, Body: "x"})
	if _, err := a.Execute(ctx, plan); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	wrote, _ := r.last(ScriptCreateNote)
	if strings.Contains(wrote.Source, "whoami") {
		t.Error("the title was spliced into the script source")
	}
}

// ---- preview says what will happen, before it happens --------------------

func TestThePreviewNamesTheFolderTheTitleAndTheText(t *testing.T) {
	r := newRunner()
	a := newAdapter(t, r)
	ctx := context.Background()

	plan, _ := a.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Write, Subject: "router notes",
		Body: "the router runs in two stages",
	})
	p, err := a.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	joined := p.Headline + "\n" + strings.Join(p.Lines, "\n")
	for _, want := range []string{Folder, "router notes", "the router runs in two stages"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the preview does not mention %q:\n%s", want, joined)
		}
	}
	if p.Confirm == "" {
		t.Error("the preview has no confirm label")
	}
	if len(r.runs) != 0 {
		t.Fatalf("previewing wrote something: %+v", r.runs)
	}
}

// ---- reading ------------------------------------------------------------

func TestReadingLooksInsideTheAdaptersFolderAndNowhereElse(t *testing.T) {
	r := newRunner()
	r.out[ScriptReadNotes] = "router notes\nweekly plan"
	a := newAdapter(t, r)
	ctx := context.Background()

	plan, err := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "router"})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	out, err := a.Execute(ctx, plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !out.Done {
		t.Error("a read that returned notes says it did not finish")
	}
	if !strings.Contains(out.Detail, "router notes") {
		t.Errorf("the outcome does not carry what was read: %q", out.Detail)
	}

	read, _ := r.last(ScriptReadNotes)
	if read.Args["folder"] != Folder {
		t.Errorf("the read looked in %q, want %q", read.Args["folder"], Folder)
	}
}

// ---- outcomes are honest ------------------------------------------------

func TestAWriteThatSucceedsReportsCompletes(t *testing.T) {
	a := newAdapter(t, newRunner())
	ctx := context.Background()

	plan, _ := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Write, Subject: "x", Body: "y"})
	out, err := a.Execute(ctx, plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.Reached != manifest.Completes || !out.Done {
		t.Errorf("outcome = %+v, want completes and done", out)
	}
	if out.HandedOffTo != "" {
		t.Errorf("a completed write claims it handed off to %q", out.HandedOffTo)
	}
}

func TestAFailingScriptIsAFailureNotAHandOff(t *testing.T) {
	r := newRunner()
	r.err = errors.New("Notes got an error: Application isn't running")
	a := newAdapter(t, r)
	ctx := context.Background()

	plan, _ := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Write, Subject: "x", Body: "y"})
	out, err := a.Execute(ctx, plan)
	if err == nil {
		t.Fatal("a failing script reported success")
	}
	if out.Done || out.HandedOffTo != "" {
		t.Errorf("a failure was dressed up as %+v", out)
	}
}

func TestAVerbTheAdapterDoesNotOfferIsRefusedByTheAdapterItself(t *testing.T) {
	// The runner already blocks this, but an adapter that would happily do it
	// if called directly is one refactor away from doing it.
	a := newAdapter(t, newRunner())
	if _, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Send, Subject: "Maya", Body: "hi",
	}); err == nil {
		t.Fatal("the notes adapter accepted a send")
	}
}

// ---- revoke -------------------------------------------------------------

func TestRevokeHasNothingToDeleteAndSaysSoWithoutFailing(t *testing.T) {
	// There is no token in this route at all. Revoke still has to succeed,
	// because the consent framework calls it uniformly.
	if err := newAdapter(t, newRunner()).Revoke(context.Background()); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}
}
