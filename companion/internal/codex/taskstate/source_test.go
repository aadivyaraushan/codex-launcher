package taskstate

import (
	"errors"
	"testing"
)

func TestAdapterRouterNeverFallsDesktopTasksThroughToAppServer(t *testing.T) {
	desktop := fakeAdapter{source: SourceDesktop}
	appServer := fakeAdapter{source: SourceAppServer}
	router, err := NewAdapterRouter(desktop, appServer)
	if err != nil {
		t.Fatal(err)
	}
	got, err := router.For(Task{ID: "desktop-1", Source: SourceDesktop})
	if err != nil || got.TaskSource() != SourceDesktop {
		t.Fatalf("desktop route = %#v, %v", got, err)
	}
	got, err = router.For(Task{ID: "cli-1", Source: SourceAppServer})
	if err != nil || got.TaskSource() != SourceAppServer {
		t.Fatalf("app-server route = %#v, %v", got, err)
	}
	if _, err := router.For(Task{ID: "unknown-1"}); !errors.Is(err, ErrUnknownTaskSource) {
		t.Fatalf("unknown route error = %v", err)
	}
}

func TestAdapterRouterRejectsMislabelledAdapters(t *testing.T) {
	if _, err := NewAdapterRouter(fakeAdapter{source: SourceAppServer}, fakeAdapter{source: SourceAppServer}); !errors.Is(err, ErrAdapterSourceMismatch) {
		t.Fatalf("constructor error = %v", err)
	}
}

func TestCatalogCandidateCannotRouteToEitherRuntime(t *testing.T) {
	desktop := fakeAdapter{source: SourceDesktop}
	appServer := fakeAdapter{source: SourceAppServer}
	router, err := NewAdapterRouter(desktop, appServer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := router.For(Task{ID: "candidate-1", Source: SourceCatalog}); !errors.Is(err, ErrUnresolvedTaskSource) {
		t.Fatalf("catalog route error = %v", err)
	}
}

type fakeAdapter struct{ source Source }

func (adapter fakeAdapter) TaskSource() Source { return adapter.source }
