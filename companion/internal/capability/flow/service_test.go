package flow

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	todoistadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/todoist"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/consent"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

type fakeTodoistAPI struct {
	created []todoistadapter.CreateTask
}

func (f *fakeTodoistAPI) ListTasks(context.Context) ([]todoistadapter.Task, error) { return nil, nil }
func (f *fakeTodoistAPI) CreateTask(_ context.Context, request todoistadapter.CreateTask) (todoistadapter.Task, error) {
	f.created = append(f.created, request)
	return todoistadapter.Task{ID: "task-1", Content: request.Content, Description: request.Description}, nil
}
func (f *fakeTodoistAPI) Clear(context.Context) error { return nil }

func todoistService(t *testing.T, model stage1.ModelFunc) (*Service, *fakeTodoistAPI) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	api := &fakeTodoistAPI{}
	a := todoistadapter.New(api, logger)
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	resolver := stage2.New(reg, contacts.NewGraph(nil), stage2.ClassMap{
		"tasks": {Adapters: []string{todoistadapter.ID}, Addressing: stage2.ToAThing},
	}, manifest.PlatformAndroid)
	gate := consent.New(time.Now, nil, consent.NoVault{}, consent.NoVault{})
	return New(stage1.New(model), resolver, execution.New(reg), gate, logger), api
}

func TestCapitalizedModelAppNameStillPreviewsAndExecutesExactlyOnce(t *testing.T) {
	service, api := todoistService(t, func(context.Context, string) ([]byte, error) {
		return []byte(`{"verb":"write","app_class":"tasks","app_named":"Todoist","subject":"Buy oat milk","body":"Before tomorrow","confidence":0.98}`), nil
	})
	ctx := context.Background()
	preview, err := service.Prepare(ctx, "pixel-9/session-1/1", "request-1", "Add buy oat milk to Todoist before tomorrow")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != "todoist" || preview.Verb != manifest.Write || preview.Fingerprint == "" {
		t.Fatalf("preview=%+v", preview)
	}
	if len(preview.Lines) != 2 || preview.Lines[0] != "Buy oat milk" || preview.Lines[1] != "Before tomorrow" {
		t.Fatalf("preview lines=%v", preview.Lines)
	}

	if _, err := service.Confirm(ctx, "pixel-9/session-1/1", "request-1", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); !errors.Is(err, ErrFingerprintMismatch) {
		t.Fatalf("wrong fingerprint returned %v", err)
	}
	if len(api.created) != 0 {
		t.Fatalf("wrong fingerprint created %d tasks", len(api.created))
	}
	if _, err := service.Confirm(ctx, "pixel-9/session-2/2", "request-1", preview.Fingerprint); !errors.Is(err, ErrUnknownRequest) {
		t.Fatalf("confirmation from another phone session returned %v", err)
	}
	outcome, err := service.Confirm(ctx, "pixel-9/session-1/1", "request-1", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !outcome.Done || len(api.created) != 1 {
		t.Fatalf("outcome=%+v created=%d", outcome, len(api.created))
	}
	if _, err := service.Confirm(ctx, "pixel-9/session-1/1", "request-1", preview.Fingerprint); !errors.Is(err, ErrUnknownRequest) {
		t.Fatalf("second confirmation returned %v", err)
	}
}

func TestLowConfidenceRouteReturnsTheQuestionAndCreatesNothing(t *testing.T) {
	service, api := todoistService(t, func(context.Context, string) ([]byte, error) {
		return []byte(`{"verb":"write","app_class":"tasks","app_named":"todoist","subject":"Buy oat milk","body":"","confidence":0.2}`), nil
	})
	_, err := service.Prepare(context.Background(), "pixel-9/session-1/1", "request-2", "maybe do something")
	var question *QuestionError
	if !errors.As(err, &question) || question.Question == "" {
		t.Fatalf("low confidence returned %v", err)
	}
	if len(api.created) != 0 {
		t.Fatalf("low confidence created %d tasks", len(api.created))
	}
}

func TestCancelDropsThePendingPreviewWithoutExecuting(t *testing.T) {
	service, api := todoistService(t, func(context.Context, string) ([]byte, error) {
		return []byte(`{"verb":"write","app_class":"tasks","app_named":"todoist","subject":"Buy oat milk","body":"","confidence":0.99}`), nil
	})
	preview, err := service.Prepare(context.Background(), "pixel-9/session-1/1", "request-3", "add task")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := service.Cancel("pixel-9/session-1/1", "request-3", preview.Fingerprint); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if _, err := service.Confirm(context.Background(), "pixel-9/session-1/1", "request-3", preview.Fingerprint); !errors.Is(err, ErrUnknownRequest) {
		t.Fatalf("confirm after cancel returned %v", err)
	}
	if len(api.created) != 0 {
		t.Fatalf("cancelled preview created %d tasks", len(api.created))
	}
}

type manualExpiry struct {
	run func()
}

func (m *manualExpiry) Stop() bool { return true }

func TestPendingPreviewExpiresAndCannotBeConfirmed(t *testing.T) {
	service, api := todoistService(t, func(context.Context, string) ([]byte, error) {
		return []byte(`{"verb":"write","app_class":"tasks","app_named":"todoist","subject":"Private task","body":"","confidence":0.99}`), nil
	})
	var expiry *manualExpiry
	service.after = func(_ time.Duration, run func()) expiryTimer {
		expiry = &manualExpiry{run: run}
		return expiry
	}
	preview, err := service.Prepare(context.Background(), "pixel-9/session-1/1", "request-expiring", "add a private task")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	expiry.run()
	if _, err := service.Confirm(context.Background(), "pixel-9/session-1/1", "request-expiring", preview.Fingerprint); !errors.Is(err, ErrUnknownRequest) {
		t.Fatalf("expired confirmation returned %v", err)
	}
	if len(api.created) != 0 {
		t.Fatalf("expired preview created %d tasks", len(api.created))
	}
}

var _ adapter.Adapter = (*todoistadapter.Adapter)(nil)
