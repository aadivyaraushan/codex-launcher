package attachments

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreCompletesVerifiedUploadWithPrivateRandomPath(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_800_000_000, 0)
	payload := []byte("private attachment contents")
	store := openTestStore(t, root, testLimits())

	progress, err := store.Begin("phone-1", "upload-1", int64(len(payload)), digest(payload), now)
	if err != nil || progress.NextChunk != 0 || progress.ReceivedBytes != 0 {
		t.Fatalf("Begin() = %#v, %v", progress, err)
	}
	progress, err = store.Append("phone-1", "upload-1", 0, 0, true, payload, now)
	if err != nil || progress.NextChunk != 1 || progress.ReceivedBytes != int64(len(payload)) {
		t.Fatalf("Append() = %#v, %v", progress, err)
	}
	completed, err := store.Complete("phone-1", "upload-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if completed.ID != "upload-1" || completed.SHA256 != digest(payload) || completed.MediaType != "text/plain; charset=utf-8" {
		t.Fatalf("completed attachment = %#v", completed)
	}
	if strings.Contains(completed.Path, "upload-1") || filepath.Dir(completed.Path) != filepath.Join(root, "complete") {
		t.Fatalf("completed path is not random and confined: %q", completed.Path)
	}
	assertPrivateRegularFile(t, completed.Path, payload)
	resolved, err := store.Resolve("phone-1", []string{"upload-1"}, now)
	if err != nil || len(resolved) != 1 || resolved[0].Path != completed.Path {
		t.Fatalf("Resolve() = %#v, %v", resolved, err)
	}
	if _, err := store.Resolve("phone-2", []string{"upload-1"}, now); !errors.Is(err, ErrAttachmentUnavailable) {
		t.Fatalf("other device Resolve() error = %v", err)
	}
}

func TestStoreRejectsInvalidOffersAndQuotasBeforeAllocation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limits := Limits{MaxFileBytes: 8, MaxDeviceUploads: 2, MaxGlobalUploads: 3, MaxTemporaryBytes: 20, Expiry: time.Minute}
	store := openTestStore(t, t.TempDir(), limits)

	invalid := []struct {
		id    string
		size  int64
		hash  string
		error error
	}{
		{"bad id!", 1, digest([]byte("a")), ErrInvalidAttachment},
		{"valid", 0, digest(nil), ErrInvalidAttachment},
		{"valid", 9, digest([]byte("123456789")), ErrAttachmentTooLarge},
		{"valid", 1, "not-a-hash", ErrInvalidAttachment},
	}
	for _, test := range invalid {
		if _, err := store.Begin("phone-1", test.id, test.size, test.hash, now); !errors.Is(err, test.error) {
			t.Fatalf("Begin(%q, %d) error = %v, want %v", test.id, test.size, err, test.error)
		}
	}
	if entries := dataEntries(t, store.root); len(entries) != 0 {
		t.Fatalf("invalid offers allocated files: %v", entries)
	}

	mustBegin(t, store, "phone-1", "upload-1", []byte("12345678"), now)
	mustBegin(t, store, "phone-1", "upload-2", []byte("12345678"), now)
	if _, err := store.Begin("phone-1", "upload-3", 1, digest([]byte("x")), now); !errors.Is(err, ErrAttachmentQuota) {
		t.Fatalf("per-device quota error = %v", err)
	}
	mustBegin(t, store, "phone-2", "upload-3", []byte("1234"), now)
	if _, err := store.Begin("phone-3", "upload-4", 1, digest([]byte("x")), now); !errors.Is(err, ErrAttachmentQuota) {
		t.Fatalf("global concurrency quota error = %v", err)
	}
	if err := store.Cancel("phone-2", "upload-3"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin("phone-3", "upload-4", 5, digest([]byte("12345")), now); !errors.Is(err, ErrAttachmentQuota) {
		t.Fatalf("temporary-byte quota error = %v", err)
	}
}

func TestStoreRejectsMissingOutOfOrderOversizedAndCorruptChunks(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	payload := []byte("abcdef")
	store := openTestStore(t, t.TempDir(), testLimits())
	mustBegin(t, store, "phone-1", "upload-1", payload, now)

	badChunks := []struct {
		chunk  uint32
		offset int64
		final  bool
		data   []byte
	}{
		{1, 0, false, []byte("abc")},
		{0, 1, false, []byte("abc")},
		{0, 0, false, []byte("abcdefg")},
		{0, 0, true, []byte("abc")},
	}
	for _, test := range badChunks {
		if _, err := store.Append("phone-1", "upload-1", test.chunk, test.offset, test.final, test.data, now); !errors.Is(err, ErrInvalidAttachment) {
			t.Fatalf("Append(%d, %d, final=%v) error = %v", test.chunk, test.offset, test.final, err)
		}
	}
	progress, err := store.Resume("phone-1", "upload-1", now)
	if err != nil || progress.NextChunk != 0 || progress.ReceivedBytes != 0 {
		t.Fatalf("invalid chunks changed progress: %#v, %v", progress, err)
	}

	if _, err := store.Append("phone-1", "missing", 0, 0, true, payload, now); !errors.Is(err, ErrAttachmentUnavailable) {
		t.Fatalf("missing upload error = %v", err)
	}
	if _, err := store.Append("phone-2", "upload-1", 0, 0, true, payload, now); !errors.Is(err, ErrAttachmentUnavailable) {
		t.Fatalf("wrong owner error = %v", err)
	}
	if _, err := store.Append("phone-1", "upload-1", 0, 0, true, []byte("abcdeg"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Complete("phone-1", "upload-1", now); !errors.Is(err, ErrAttachmentCorrupt) {
		t.Fatalf("corrupt completion error = %v", err)
	}
	if _, err := store.Resume("phone-1", "upload-1", now); !errors.Is(err, ErrAttachmentUnavailable) {
		t.Fatalf("corrupt upload retained: %v", err)
	}
}

func TestStoreResumesAcrossRestartAndCleansExpiredAndOrphanFiles(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_800_000_000, 0)
	payload := []byte("six bytes")
	store := openTestStore(t, root, testLimits())
	mustBegin(t, store, "phone-1", "upload-1", payload, now)
	if _, err := store.Append("phone-1", "upload-1", 0, 0, false, payload[:3], now); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(root, "active", "orphan.part")
	if err := os.WriteFile(orphan, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}

	restarted := openTestStore(t, root, testLimits())
	progress, err := restarted.Resume("phone-1", "upload-1", now.Add(30*time.Second))
	if err != nil || progress.NextChunk != 1 || progress.ReceivedBytes != 3 {
		t.Fatalf("restart Resume() = %#v, %v", progress, err)
	}
	if _, err := os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan survived startup cleanup: %v", err)
	}
	if _, err := restarted.Append("phone-1", "upload-1", 1, 3, true, payload[3:], now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Complete("phone-1", "upload-1", now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}

	expired := openTestStore(t, root, testLimits())
	if _, err := expired.Resolve("phone-1", []string{"upload-1"}, now.Add(2*time.Minute)); !errors.Is(err, ErrAttachmentUnavailable) {
		t.Fatalf("expired completed attachment error = %v", err)
	}
	if entries := dataEntries(t, root); len(entries) != 0 {
		t.Fatalf("expired files survived cleanup: %v", entries)
	}
}

func TestStoreCancelAndDiskFailureRemoveOrRollBackPartialState(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	payload := []byte("abcdef")
	store := openTestStore(t, t.TempDir(), testLimits())
	mustBegin(t, store, "phone-1", "cancelled", payload, now)
	if err := store.Cancel("phone-1", "cancelled"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resume("phone-1", "cancelled", now); !errors.Is(err, ErrAttachmentUnavailable) {
		t.Fatalf("cancelled upload remained: %v", err)
	}

	mustBegin(t, store, "phone-1", "disk-full", payload, now)
	store.writeChunk = func(file *os.File, data []byte) (int, error) {
		if _, err := file.Write(data[:2]); err != nil {
			return 0, err
		}
		return 2, io.ErrShortWrite
	}
	if _, err := store.Append("phone-1", "disk-full", 0, 0, true, payload, now); !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("disk failure error = %v", err)
	}
	progress, err := store.Resume("phone-1", "disk-full", now)
	if err != nil || progress.NextChunk != 0 || progress.ReceivedBytes != 0 {
		t.Fatalf("disk failure changed durable progress: %#v, %v", progress, err)
	}
}

func TestStoreRecoversCompletionInterruptedAfterPublishRename(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_800_000_000, 0)
	payload := []byte("fully uploaded")
	store := openTestStore(t, root, testLimits())
	mustBegin(t, store, "phone-1", "upload-1", payload, now)
	if _, err := store.Append("phone-1", "upload-1", 0, 0, true, payload, now); err != nil {
		t.Fatal(err)
	}
	store.afterPublishRename = func() error { return errors.New("simulated crash after rename") }
	if _, err := store.Complete("phone-1", "upload-1", now); !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("interrupted Complete() error = %v", err)
	}

	restarted := openTestStore(t, root, testLimits())
	resolved, err := restarted.Resolve("phone-1", []string{"upload-1"}, now)
	if err != nil || len(resolved) != 1 {
		t.Fatalf("recovered Resolve() = %#v, %v", resolved, err)
	}
	assertPrivateRegularFile(t, resolved[0].Path, payload)
}

func TestStoreCompletionIsIdempotentAfterRestartAndLostAck(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_800_000_000, 0)
	payload := []byte("completed before ack")
	store := openTestStore(t, root, testLimits())
	mustBegin(t, store, "phone-1", "upload-1", payload, now)
	if _, err := store.Append("phone-1", "upload-1", 0, 0, true, payload, now); err != nil {
		t.Fatal(err)
	}
	first, err := store.Complete("phone-1", "upload-1", now)
	if err != nil {
		t.Fatal(err)
	}

	restarted := openTestStore(t, root, testLimits())
	second, err := restarted.Complete("phone-1", "upload-1", now.Add(time.Second))
	if err != nil || second != first {
		t.Fatalf("retried Complete() = %#v, %v; want %#v", second, err, first)
	}
}

func TestStoreKeepsFailedCleanupCountedAgainstQuota(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limits := Limits{MaxFileBytes: 6, MaxDeviceUploads: 2, MaxGlobalUploads: 4, MaxTemporaryBytes: 6, Expiry: time.Minute}
	store := openTestStore(t, t.TempDir(), limits)
	payload := []byte("123456")
	mustBegin(t, store, "phone-1", "upload-1", payload, now)
	realRemove := store.removePath
	store.removePath = func(path string) error {
		if strings.HasSuffix(path, ".part") {
			return os.ErrPermission
		}
		return realRemove(path)
	}
	if err := store.Cancel("phone-1", "upload-1"); !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("Cancel() error = %v", err)
	}
	if _, err := store.Begin("phone-2", "upload-2", 1, digest([]byte("x")), now); !errors.Is(err, ErrAttachmentQuota) {
		t.Fatalf("failed cleanup no longer counted: %v", err)
	}
	if _, err := store.Begin("phone-2", "upload-3", 1, digest([]byte("x")), now.Add(2*time.Minute)); !errors.Is(err, ErrAttachmentQuota) {
		t.Fatalf("failed expiry cleanup no longer counted: %v", err)
	}
}

func TestStoreKeepsUploadWhenInspectionHasTransientIOFailure(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	payload := []byte("attachment")
	store := openTestStore(t, t.TempDir(), testLimits())
	mustBegin(t, store, "phone-1", "upload-1", payload, now)
	if _, err := store.Append("phone-1", "upload-1", 0, 0, true, payload, now); err != nil {
		t.Fatal(err)
	}
	store.inspect = func(string, int64) (string, string, error) { return "", "", os.ErrPermission }
	if _, err := store.Complete("phone-1", "upload-1", now); !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("transient inspection error = %v", err)
	}
	progress, err := store.Resume("phone-1", "upload-1", now)
	if err != nil || progress.ReceivedBytes != int64(len(payload)) || progress.NextChunk != 1 {
		t.Fatalf("upload lost after inspection failure: %#v, %v", progress, err)
	}
}

func TestStoreDeletesFilesWithTruncatedOrExtraBytesAsCorrupt(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	payload := []byte("attachment")
	for _, test := range []struct {
		name   string
		mutate func(string) error
	}{
		{name: "truncated", mutate: func(path string) error { return os.Truncate(path, int64(len(payload)-1)) }},
		{name: "extra bytes", mutate: func(path string) error {
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			if _, err = file.Write([]byte("x")); err == nil {
				err = file.Sync()
			}
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openTestStore(t, t.TempDir(), testLimits())
			mustBegin(t, store, "phone-1", "upload-1", payload, now)
			if _, err := store.Append("phone-1", "upload-1", 0, 0, true, payload, now); err != nil {
				t.Fatal(err)
			}
			upload := store.records[recordKey("phone-1", "upload-1")]
			if err := test.mutate(filepath.Join(store.activeRoot, upload.DataName)); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Complete("phone-1", "upload-1", now); !errors.Is(err, ErrAttachmentCorrupt) {
				t.Fatalf("Complete() error = %v", err)
			}
			if _, err := store.Resume("phone-1", "upload-1", now); !errors.Is(err, ErrAttachmentUnavailable) {
				t.Fatalf("corrupt file retained: %v", err)
			}
		})
	}
}

func TestStoreLogsCaughtFailuresWithoutPrivateValues(t *testing.T) {
	root := t.TempDir()
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	store, err := Open(root, testLimits(), logger)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	payload := []byte("secret attachment body")
	mustBegin(t, store, "phone-1", "upload-1", payload, now)
	store.writeChunk = func(*os.File, []byte) (int, error) { return 0, os.ErrPermission }
	if _, err := store.Append("phone-1", "upload-1", 0, 0, true, payload, now); !errors.Is(err, ErrStorageUnavailable) {
		t.Fatal(err)
	}
	encoded := logs.String()
	for _, required := range []string{"operation=append_chunk", "decision=rollback_partial_write", "device_id=phone-1", "upload_id=upload-1", "error_class"} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("safe diagnostic %q missing from logs: %s", required, encoded)
		}
	}
	for _, private := range []string{root, string(payload), digest(payload)} {
		if strings.Contains(encoded, private) {
			t.Fatalf("private value leaked to logs: %q", private)
		}
	}
}

func TestStoreReleaseDeletesCompletedFilesAndRejectsDuplicateIds(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	payload := []byte("attachment")
	store := openTestStore(t, t.TempDir(), testLimits())
	mustBegin(t, store, "phone-1", "upload-1", payload, now)
	progress, err := store.Begin("phone-1", "upload-1", int64(len(payload)), digest(payload), now)
	if err != nil || progress.NextChunk != 0 || progress.ReceivedBytes != 0 {
		t.Fatalf("retried active offer = %#v, %v", progress, err)
	}
	if _, err := store.Begin("phone-1", "upload-1", int64(len(payload))+1, digest(append(payload, 'x')), now); !errors.Is(err, ErrAttachmentQuota) {
		t.Fatalf("mismatched duplicate active upload error = %v", err)
	}
	if _, err := store.Append("phone-1", "upload-1", 0, 0, true, payload, now); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Complete("phone-1", "upload-1", now)
	if err != nil {
		t.Fatal(err)
	}
	progress, err = store.Begin("phone-1", "upload-1", int64(len(payload)), digest(payload), now)
	if err != nil || progress.ReceivedBytes != int64(len(payload)) || progress.NextChunk != 1 {
		t.Fatalf("retried completed offer = %#v, %v", progress, err)
	}
	if err := store.Release("phone-1", []string{"upload-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(completed.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("released attachment file remains: %v", err)
	}
}

func openTestStore(t *testing.T, root string, limits Limits) *Store {
	t.Helper()
	store, err := Open(root, limits, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func testLimits() Limits {
	return Limits{MaxFileBytes: 1024, MaxDeviceUploads: 2, MaxGlobalUploads: 4, MaxTemporaryBytes: 4096, Expiry: time.Minute}
}

func mustBegin(t *testing.T, store *Store, deviceID, uploadID string, payload []byte, now time.Time) {
	t.Helper()
	if _, err := store.Begin(deviceID, uploadID, int64(len(payload)), digest(payload), now); err != nil {
		t.Fatal(err)
	}
}

func digest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func assertPrivateRegularFile(t *testing.T, path string, want []byte) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v", info.Mode())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("file contents = %q", got)
	}
}

func dataEntries(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	for _, folder := range []string{"active", "complete"} {
		children, err := os.ReadDir(filepath.Join(root, folder))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		for _, child := range children {
			entries = append(entries, filepath.Join(folder, child.Name()))
		}
	}
	return entries
}
