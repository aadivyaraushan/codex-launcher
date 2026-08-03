package runtime

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gcalendar"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gdrive"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// Callers: runtime package tests only. User: Google Calendar + Drive Wave 1.

type fakeGoogleCalendarAPI struct {
	events  []gcalendar.Event
	created []gcalendar.CreateEvent
}

func (f *fakeGoogleCalendarAPI) ListEvents(context.Context, string) ([]gcalendar.Event, error) {
	return f.events, nil
}
func (f *fakeGoogleCalendarAPI) CreateEvent(_ context.Context, request gcalendar.CreateEvent) (gcalendar.Event, error) {
	f.created = append(f.created, request)
	return gcalendar.Event{ID: "e9", Summary: request.Summary}, nil
}
func (f *fakeGoogleCalendarAPI) Clear(context.Context) error { return nil }

type fakeGoogleDriveAPI struct {
	files   []gdrive.File
	created []gdrive.CreateFile
}

func (f *fakeGoogleDriveAPI) ListFiles(context.Context, string) ([]gdrive.File, error) {
	return f.files, nil
}
func (f *fakeGoogleDriveAPI) CreateFile(_ context.Context, request gdrive.CreateFile) (gdrive.File, error) {
	f.created = append(f.created, request)
	return gdrive.File{ID: "f9", Name: request.Name}, nil
}
func (f *fakeGoogleDriveAPI) Clear(context.Context) error { return nil }

func TestGoogleFlowComposesCalendarWriteThroughPreview(t *testing.T) {
	calendarAPI := &fakeGoogleCalendarAPI{}
	driveAPI := &fakeGoogleDriveAPI{}
	service, err := NewGoogle(GoogleConfig{
		CalendarAPI: calendarAPI,
		DriveAPI:    driveAPI,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"write","app_class":"calendar","app_named":"gcalendar","subject":"Operator Wave 1","body":"proof","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewGoogle: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-1", "Create Operator Wave 1 on my calendar")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != gcalendar.ID || preview.Verb != manifest.Write || preview.Fingerprint == "" {
		t.Fatalf("preview=%+v", preview)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-1", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !outcome.Done || len(calendarAPI.created) != 1 {
		t.Fatalf("outcome=%+v created=%d", outcome, len(calendarAPI.created))
	}
}

func TestGoogleFlowRejectsMissingRuntimeDependencies(t *testing.T) {
	if _, err := NewGoogle(GoogleConfig{}); err == nil {
		t.Fatal("empty runtime config was accepted")
	}
}
