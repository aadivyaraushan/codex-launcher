package credentialstore

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestKeychainRoundTripDoesNotNeedAnInteractivePrompt(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS Keychain is only available on Darwin")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	name := fmt.Sprintf("integration_%d", time.Now().UnixNano())
	store := NewKeychain(nil)
	t.Cleanup(func() { _ = store.Delete(context.Background(), name) })

	want := []byte(`{"access_token":"round-trip-secret"}`)
	if err := store.Put(ctx, name, want); err != nil {
		t.Fatalf("Put without an interactive prompt: %v", err)
	}
	got, err := store.Get(ctx, name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("Get = %q, want %q", got, want)
	}
	if err := store.Delete(ctx, name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete error = %v, want ErrNotFound", err)
	}
}

func TestKeychainUpdateDoesNotNeedAnInteractivePrompt(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS Keychain is only available on Darwin")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	name := fmt.Sprintf("integration_update_%d", time.Now().UnixNano())
	store := NewKeychain(nil)
	t.Cleanup(func() { _ = store.Delete(context.Background(), name) })

	if err := store.Put(ctx, name, []byte("first-secret")); err != nil {
		t.Fatalf("initial Put: %v", err)
	}
	if err := store.Put(ctx, name, []byte("replacement-secret")); err != nil {
		t.Fatalf("replacement Put without an interactive prompt: %v", err)
	}
	got, err := store.Get(ctx, name)
	if err != nil {
		t.Fatalf("Get replacement: %v", err)
	}
	if string(got) != "replacement-secret" {
		t.Fatalf("Get replacement = %q", got)
	}
}

func TestNewKeychainUsesNativeBackendOnMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS Keychain is only available on Darwin")
	}
	got := fmt.Sprintf("%T", NewKeychain(nil).command)
	if got != "macos.Command" {
		t.Fatalf("NewKeychain backend = %s, want native macos.Command", got)
	}
}

type commandCall struct {
	input string
	args  []string
}

type fakeCommand struct {
	calls  []commandCall
	output []byte
	err    error
}

func (f *fakeCommand) Run(_ context.Context, input string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, commandCall{input: input, args: append([]string(nil), args...)})
	return append([]byte(nil), f.output...), f.err
}

func TestPutKeepsSecretOutOfCommandArguments(t *testing.T) {
	command := &fakeCommand{}
	store := newKeychain("com.operator.test", command)
	secret := []byte(`{"access_token":"very-secret"}`)

	if err := store.Put(context.Background(), "google", secret); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if len(command.calls) != 1 {
		t.Fatalf("command calls = %d, want 1", len(command.calls))
	}
	call := command.calls[0]
	if call.input != string(secret) {
		t.Fatalf("native Keychain backend did not receive the exact secret")
	}
	if strings.Contains(strings.Join(call.args, " "), "very-secret") {
		t.Fatalf("secret leaked into command arguments: %v", call.args)
	}
}

func TestGetReturnsSecretWithoutWhitespaceAddedBySecurity(t *testing.T) {
	command := &fakeCommand{output: []byte("stored-secret\n")}
	store := newKeychain("com.operator.test", command)

	secret, err := store.Get(context.Background(), "youtube")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(secret) != "stored-secret" {
		t.Fatalf("secret = %q", secret)
	}
}

func TestGetMapsMissingKeychainItemToErrNotFound(t *testing.T) {
	command := &fakeCommand{err: errors.New("security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain.")}
	store := newKeychain("com.operator.test", command)

	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get error = %v, want ErrNotFound", err)
	}
}
