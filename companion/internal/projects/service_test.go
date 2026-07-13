package projects

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestServiceExposesOnlyOpaqueProjectChoices(t *testing.T) {
	root := canonicalTempDir(t)
	service, err := New([]Config{{ID: "project-main", DisplayName: "Main project", Path: root}})
	if err != nil {
		t.Fatal(err)
	}

	choices := service.List()
	if len(choices) != 1 || choices[0] != (Choice{ID: "project-main", DisplayName: "Main project"}) {
		t.Fatalf("choices = %#v", choices)
	}
	resolved, err := service.Resolve("project-main")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(root)
	if resolved != want {
		t.Fatalf("resolved path = %q, want %q", resolved, want)
	}
	if _, err := service.Resolve(root); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("raw path lookup error = %v", err)
	}
}

func TestServiceRejectsInvalidDuplicateAndSymlinkedConfiguration(t *testing.T) {
	root := canonicalTempDir(t)
	for name, configs := range map[string][]Config{
		"relative path":   {{ID: "project-main", DisplayName: "Main", Path: "relative"}},
		"path shaped id":  {{ID: "../main", DisplayName: "Main", Path: root}},
		"control in name": {{ID: "project-main", DisplayName: "Main\nInjected", Path: root}},
		"duplicate id": {
			{ID: "project-main", DisplayName: "Main", Path: root},
			{ID: "project-main", DisplayName: "Other", Path: root},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(configs); !errors.Is(err, ErrInvalidProject) {
				t.Fatalf("configuration error = %v", err)
			}
		})
	}

	target := canonicalTempDir(t)
	link := filepath.Join(canonicalTempDir(t), "linked-project")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := New([]Config{{ID: "project-main", DisplayName: "Main", Path: link}}); !errors.Is(err, ErrUnsafeProjectPath) {
		t.Fatalf("symlink configuration error = %v", err)
	}
	child := filepath.Join(target, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := New([]Config{{ID: "project-main", DisplayName: "Main", Path: filepath.Join(link, "child")}}); !errors.Is(err, ErrUnsafeProjectPath) {
		t.Fatalf("ancestor symlink configuration error = %v", err)
	}
}

func TestServiceProjectCountMatchesTheWireLimit(t *testing.T) {
	root := canonicalTempDir(t)
	configs := make([]Config, MaxChoices+1)
	for index := range configs {
		configs[index] = Config{ID: fmt.Sprintf("project-%03d", index), DisplayName: fmt.Sprintf("Project %d", index), Path: root}
	}
	if _, err := New(configs[:MaxChoices]); err != nil {
		t.Fatalf("%d projects were rejected: %v", MaxChoices, err)
	}
	if _, err := New(configs); !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("%d-project configuration error = %v, want %v", MaxChoices+1, err, ErrInvalidProject)
	}
}

func TestResolveFailsWhenConfiguredFolderDisappearsOrChanges(t *testing.T) {
	container := canonicalTempDir(t)
	root := filepath.Join(container, "project")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	service, err := New([]Config{{ID: "project-main", DisplayName: "Main", Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, filepath.Join(container, "renamed")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Resolve("project-main"); !errors.Is(err, ErrProjectUnavailable) {
		t.Fatalf("replaced folder error = %v", err)
	}
}

func TestUnknownProjectInputCannotLeakAPathToLogs(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	service, err := NewWithLogger([]Config{{ID: "project-main", DisplayName: "Main", Path: canonicalTempDir(t)}}, logger)
	if err != nil {
		t.Fatal(err)
	}
	privatePath := "/Users/example/private-project"
	_, _ = service.Resolve(privatePath)
	if bytes.Contains(output.Bytes(), []byte(privatePath)) {
		t.Fatalf("unknown project path leaked to logs: %s", output.String())
	}
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
