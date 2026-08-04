// Package gdrive is Operator's Wave 1 Google Drive adapter. It stays inside
// drive.file (app-created / picked / shared files) with read + preview-gated write.
package gdrive

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const ID = "gdrive"

var (
	ErrAmbiguousFile = errors.New("gdrive: more than one file matches; ask the user which one")
	ErrNoFile        = errors.New("gdrive: no file matches")
	ErrNotConnected  = errors.New("gdrive: adapter is not connected")
	ErrEmptyName     = errors.New("gdrive: file name must not be empty")
)

// API is the small part of Drive the adapter needs under drive.file.
type API interface {
	ListFiles(context.Context, string) ([]File, error)
	CreateFile(context.Context, CreateFile) (File, error)
	Clear(context.Context) error
}

type File struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
}

type CreateFile struct {
	Name    string
	Content string
}

type Adapter struct {
	api    API
	logger *slog.Logger
}

var _ adapter.Adapter = (*Adapter)(nil)

func New(api API, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{api: api, logger: logger}
}

func (a *Adapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID: ID, Runtime: manifest.RT2,
		Verbs:   []manifest.Verb{manifest.Read, manifest.Write},
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthOAuth, Cost: manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "gdrive_drive_file_oauth_read_write",
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	if a.api == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	a.logger.Info("[gdrive] resolve", "verb", in.Verb, "subject_length", len(in.Subject), "body_length", len(in.Body))
	switch in.Verb {
	case manifest.Write:
		name := strings.TrimSpace(in.Subject)
		if name == "" {
			return adapter.Plan{}, ErrEmptyName
		}
		return adapter.Plan{
			AdapterID: ID, Verb: manifest.Write, Summary: "Create a Drive file",
			Details: map[string]string{"name": name, "content": in.Body},
		}, nil
	case manifest.Read:
		return a.resolveRead(ctx, in.Subject)
	default:
		return adapter.Plan{}, fmt.Errorf("gdrive: verb %q is not supported", in.Verb)
	}
}

func (a *Adapter) resolveRead(ctx context.Context, query string) (adapter.Plan, error) {
	files, err := a.api.ListFiles(ctx, query)
	if err != nil {
		a.logger.Error("[gdrive] list files failed", "error", err)
		return adapter.Plan{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	var exact, partial []File
	for _, file := range files {
		name := strings.ToLower(strings.TrimSpace(file.Name))
		id := strings.ToLower(strings.TrimSpace(file.ID))
		if name == needle || id == needle {
			exact = append(exact, file)
			continue
		}
		if needle != "" && strings.Contains(name, needle) {
			partial = append(partial, file)
		}
	}
	matches := partial
	if len(exact) > 0 {
		matches = exact
	}
	if len(matches) == 0 {
		return adapter.Plan{}, ErrNoFile
	}
	if len(matches) > 1 {
		return adapter.Plan{}, &adapter.ClarificationError{Question: "Which matching Google Drive file did you mean?", Cause: ErrAmbiguousFile}
	}
	file := matches[0]
	return adapter.Plan{
		AdapterID: ID, Verb: manifest.Read, Handle: file.ID, Summary: "Read a Drive file",
		Details: map[string]string{"file_id": file.ID, "name": file.Name, "mime_type": file.MimeType},
	}, nil
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	if a.api == nil {
		return adapter.Preview{}, ErrNotConnected
	}
	lines := []string{plan.Details["name"]}
	if content := plan.Details["content"]; content != "" {
		lines = append(lines, content)
	}
	confirm := "Read from Drive"
	if plan.Verb == manifest.Write {
		confirm = "Create file"
	}
	return adapter.Preview{Plan: plan, Headline: plan.Summary, Lines: lines, Confirm: confirm}, nil
}

func (a *Adapter) Execute(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	if a.api == nil {
		return adapter.Outcome{}, ErrNotConnected
	}
	a.logger.Info("[gdrive] execute", "verb", plan.Verb)
	switch plan.Verb {
	case manifest.Read:
		return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: plan.Details["name"]}, nil
	case manifest.Write:
		created, err := a.api.CreateFile(ctx, CreateFile{
			Name: plan.Details["name"], Content: plan.Details["content"],
		})
		if err != nil {
			a.logger.Error("[gdrive] create file failed", "error", err)
			return adapter.Outcome{}, err
		}
		a.logger.Info("[gdrive] execute complete", "verb", plan.Verb, "done", true)
		return adapter.Outcome{
			Reached: manifest.Completes, Done: true,
			Detail: fmt.Sprintf("created Drive file %s", created.ID),
		}, nil
	default:
		return adapter.Outcome{}, fmt.Errorf("gdrive: verb %q is not supported", plan.Verb)
	}
}

// Revoke of an already-disconnected adapter reports success, not
// ErrNotConnected — see the comment on todoist's Revoke for why (a retried
// revoke should never look like a failed disconnect).
func (a *Adapter) Revoke(ctx context.Context) error {
	if a.api == nil {
		return nil
	}
	if err := a.api.Clear(ctx); err != nil {
		a.logger.Error("[gdrive] revoke failed", "error", err)
		return err
	}
	a.api = nil
	a.logger.Info("[gdrive] revoked")
	return nil
}
