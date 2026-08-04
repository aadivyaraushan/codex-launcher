package gdrive

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

type fakeAPI struct {
	files   []File
	created []CreateFile
	reply   File
	cleared int
	err     error
}

func (f *fakeAPI) ListFiles(context.Context, string) ([]File, error) { return f.files, f.err }
func (f *fakeAPI) CreateFile(_ context.Context, request CreateFile) (File, error) {
	f.created = append(f.created, request)
	return f.reply, f.err
}
func (f *fakeAPI) Clear(context.Context) error { f.cleared++; return f.err }

func TestManifestIsFreeAndroidRT2DriveFileOAuthRoute(t *testing.T) {
	a := New(&fakeAPI{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	if m.ID != ID || m.Runtime != manifest.RT2 || m.Auth != manifest.AuthOAuth || m.Cost != manifest.CostFree {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if !m.Allows(manifest.Read) || !m.Allows(manifest.Write) {
		t.Fatalf("drive must offer read and write: %v", m.Verbs)
	}
	if !strings.Contains(m.ProvesCeiling, "drive.file") && !strings.Contains(m.ProvesCeiling, "drive_file") {
		t.Fatalf("proves ceiling should name drive.file: %q", m.ProvesCeiling)
	}
}

func TestReadFindsOneFileAndRefusesAmbiguousMatch(t *testing.T) {
	api := &fakeAPI{files: []File{
		{ID: "f1", Name: "alpha-notes.txt"},
		{ID: "f2", Name: "beta-notes.txt"},
	}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "notes"})
	if !errors.Is(err, ErrAmbiguousFile) {
		t.Fatalf("ambiguous read returned %v", err)
	}
	var question *adapter.ClarificationError
	if !errors.As(err, &question) || question.Question == "" {
		t.Fatalf("ambiguous read is not a user question: %T %v", err, err)
	}
	plan, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "beta-notes.txt"})
	if err != nil {
		t.Fatalf("exact read: %v", err)
	}
	if plan.Handle != "f2" || plan.Details["name"] != "beta-notes.txt" {
		t.Fatalf("plan=%+v", plan)
	}
	out, err := a.Execute(context.Background(), plan)
	if err != nil || !out.Done || !strings.Contains(out.Detail, "beta-notes.txt") {
		t.Fatalf("execute=%+v err=%v", out, err)
	}
}

func TestWriteNeedsExactPreviewAndCreatesOnce(t *testing.T) {
	api := &fakeAPI{reply: File{ID: "f9", Name: "operator-wave1.txt"}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	run := execution.New(reg)
	ctx := context.Background()

	plan, err := run.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Write, Subject: "operator-wave1.txt", Body: "hello from Operator",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := run.Execute(ctx, plan, execution.Confirmation{}); !errors.Is(err, execution.ErrPreviewRequired) {
		t.Fatalf("unconfirmed write returned %v", err)
	}
	if len(api.created) != 0 {
		t.Fatalf("unconfirmed write created %d files", len(api.created))
	}
	preview, err := run.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil || !out.Done || len(api.created) != 1 {
		t.Fatalf("confirmed write outcome=%+v created=%d err=%v", out, len(api.created), err)
	}
	if api.created[0].Name != "operator-wave1.txt" || api.created[0].Content != "hello from Operator" {
		t.Fatalf("created=%+v", api.created[0])
	}
}
