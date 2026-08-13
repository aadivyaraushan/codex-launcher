package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth/store"
)

func TestListOpenAIPrefersStoreAndSkipsCLI(t *testing.T) {
	want := []modelauth.Profile{{ID: "openai:default", Type: "oauth", Provider: "openai"}}
	run := func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("CLI must not run when the on-disk store is readable")
		return nil, nil
	}
	profiles, err := listOpenAI(context.Background(), func() ([]modelauth.Profile, error) {
		return want, nil
	}, run)
	if err != nil {
		t.Fatalf("listOpenAI: %v", err)
	}
	if len(profiles) != 1 || profiles[0] != want[0] {
		t.Fatalf("profiles = %+v", profiles)
	}
}

func TestListOpenAIFallsBackToCLIWhenStoreMissing(t *testing.T) {
	var ran bool
	run := func(context.Context, string, ...string) ([]byte, error) {
		ran = true
		return []byte(`{"profiles":[{"id":"openai:default","type":"oauth","provider":"openai"}]}`), nil
	}
	profiles, err := listOpenAI(context.Background(), func() ([]modelauth.Profile, error) {
		return nil, store.ErrNotFound
	}, run)
	if err != nil {
		t.Fatalf("listOpenAI: %v", err)
	}
	if !ran {
		t.Fatal("CLI fallback was not used")
	}
	if len(profiles) != 1 || profiles[0].Type != "oauth" {
		t.Fatalf("profiles = %+v", profiles)
	}
}

func TestListOpenAIDoesNotSpawnCLIWhenStoreUnreadable(t *testing.T) {
	run := func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("CLI must not run when the store exists but is unreadable")
		return nil, nil
	}
	_, err := listOpenAI(context.Background(), func() ([]modelauth.Profile, error) {
		return nil, store.ErrUnreadable
	}, run)
	if !errors.Is(err, store.ErrUnreadable) {
		t.Fatalf("err = %v, want ErrUnreadable", err)
	}
}

func TestListOpenAIParsesJsonAndIgnoresStderrNoise(t *testing.T) {
	run := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "openclaw" || strings.Join(args, " ") != "models auth list --provider openai --json" {
			t.Fatalf("run %s %v", name, args)
		}
		return []byte("note: probing\n{\"profiles\":[{\"id\":\"openai:default\",\"type\":\"oauth\",\"provider\":\"openai\",\"access\":\"sk-nope\"}]}\n"), nil
	}
	profiles, err := ListOpenAIWith(context.Background(), run)
	if err != nil {
		t.Fatalf("ListOpenAIWith: %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID != "openai:default" || profiles[0].Type != "oauth" {
		t.Fatalf("profiles = %+v", profiles)
	}
}

func TestSetOpenAIAuthOrderPassesIdsInGivenOrder(t *testing.T) {
	var got []string
	run := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		return nil, nil
	}
	if err := SetOpenAIAuthOrderWith(context.Background(), run, []string{"openai:default", "openai:manual"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"openclaw", "models", "auth", "order", "set", "--provider", "openai", "openai:default", "openai:manual"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestRestartGatewayFallsBackToSv(t *testing.T) {
	var names []string
	run := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		names = append(names, name+" "+strings.Join(args, " "))
		if name == "openclaw" {
			return nil, errors.New("missing")
		}
		return []byte("ok"), nil
	}
	if err := RestartGatewayWith(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "openclaw gateway restart" || names[1] != "sv restart openclaw-gateway" {
		t.Fatalf("names = %v", names)
	}
}

func TestRedactOutputStripsSecrets(t *testing.T) {
	out := RedactOutput([]byte("access_token=sk-abc refresh_token=rt-zzz"))
	if bytes.Contains([]byte(out), []byte("sk-abc")) || strings.Contains(out, "rt-zzz") {
		t.Fatalf("leaked: %q", out)
	}
}
