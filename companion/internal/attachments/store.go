package attachments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/attachments/privatefiles"
)

const metadataVersion = 1

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,256}$`)

type Progress struct {
	NextChunk     uint32
	ReceivedBytes int64
	ExpiresAt     time.Time
}

type Attachment struct {
	ID        string
	Path      string
	SHA256    string
	MediaType string
}

type record struct {
	Version        int       `json:"version"`
	UploadID       string    `json:"uploadId"`
	DeviceID       string    `json:"deviceId"`
	DeclaredTotal  int64     `json:"declaredTotal"`
	SHA256         string    `json:"sha256"`
	ReceivedBytes  int64     `json:"receivedBytes"`
	NextChunk      uint32    `json:"nextChunk"`
	Final          bool      `json:"final"`
	Complete       bool      `json:"complete"`
	MediaType      string    `json:"mediaType,omitempty"`
	DataName       string    `json:"dataName"`
	PublishingName string    `json:"publishingName,omitempty"`
	MetadataName   string    `json:"-"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type Store struct {
	mu                 sync.Mutex
	root               string
	activeRoot         string
	completeRoot       string
	limits             Limits
	records            map[string]*record
	untrackedBytes     int64
	logger             *slog.Logger
	writeChunk         func(*os.File, []byte) (int, error)
	removePath         func(string) error
	inspect            func(string, int64) (string, string, error)
	afterPublishRename func() error
}

func Open(root string, limits Limits, logger *slog.Logger) (*Store, error) {
	if !filepath.IsAbs(root) || !limits.valid() {
		return nil, ErrInvalidAttachment
	}
	if logger == nil {
		logger = slog.Default()
	}
	store := &Store{
		root: root, activeRoot: filepath.Join(root, "active"), completeRoot: filepath.Join(root, "complete"),
		limits: limits, records: make(map[string]*record), logger: logger,
		writeChunk: func(file *os.File, data []byte) (int, error) { return file.Write(data) },
		removePath: os.Remove, inspect: inspectFile,
	}
	for _, directory := range []string{store.root, store.activeRoot, store.completeRoot} {
		if err := privatefiles.PrepareDirectory(directory); err != nil {
			store.logger.Error("[attachments] storage operation failed", "operation", "prepare_directory", "error_class", fmt.Sprintf("%T", err), "decision", "fail_closed")
			return nil, fmt.Errorf("%w: prepare private directory", ErrStorageUnavailable)
		}
	}
	if err := store.restore(time.Now()); err != nil {
		return nil, err
	}
	store.logger.Info("[attachments] store opened", "input_shape", "private_directory", "retained_count", len(store.records))
	return store, nil
}

func (store *Store) Begin(deviceID, uploadID string, declaredTotal int64, expectedSHA string, now time.Time) (Progress, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.cleanupLocked(now)
	if !validIdentifier(deviceID) || !validIdentifier(uploadID) || declaredTotal <= 0 || !validSHA(expectedSHA) {
		return Progress{}, ErrInvalidAttachment
	}
	if declaredTotal > store.limits.MaxFileBytes {
		return Progress{}, ErrAttachmentTooLarge
	}
	key := recordKey(deviceID, uploadID)
	if existing := store.records[key]; existing != nil {
		if existing.DeclaredTotal == declaredTotal && strings.EqualFold(existing.SHA256, expectedSHA) {
			store.logger.Info("[attachments] repeated offer accepted", "device_id", deviceID, "upload_id", uploadID, "received_bytes", existing.ReceivedBytes, "decision", "return_durable_progress")
			return progressOf(existing), nil
		}
		return Progress{}, ErrAttachmentQuota
	}
	if !withinQuota(store.records, store.untrackedBytes, deviceID, declaredTotal, store.limits) {
		return Progress{}, ErrAttachmentQuota
	}
	data, err := os.CreateTemp(store.activeRoot, "data-*.part")
	if err != nil {
		store.logFailure("begin_create", deviceID, uploadID, err, "reject_without_allocation")
		return Progress{}, ErrStorageUnavailable
	}
	dataName := filepath.Base(data.Name())
	if hardenErr := privatefiles.HardenFile(data); hardenErr != nil {
		_ = data.Close()
		_ = os.Remove(data.Name())
		store.logFailure("begin_harden", deviceID, uploadID, hardenErr, "delete_partial")
		return Progress{}, ErrStorageUnavailable
	}
	if closeErr := data.Close(); closeErr != nil {
		_ = os.Remove(data.Name())
		store.logFailure("begin_close", deviceID, uploadID, closeErr, "delete_partial")
		return Progress{}, ErrStorageUnavailable
	}
	metadataName := strings.TrimSuffix(dataName, ".part") + ".json"
	upload := &record{
		Version: metadataVersion, UploadID: uploadID, DeviceID: deviceID, DeclaredTotal: declaredTotal,
		SHA256: strings.ToLower(expectedSHA), DataName: dataName, MetadataName: metadataName, ExpiresAt: now.Add(store.limits.Expiry),
	}
	if err := store.writeMetadata(store.activeRoot, upload); err != nil {
		_ = os.Remove(data.Name())
		store.logFailure("begin_metadata", deviceID, uploadID, err, "delete_partial")
		return Progress{}, ErrStorageUnavailable
	}
	store.records[key] = upload
	store.logger.Info("[attachments] upload accepted", "device_id", deviceID, "upload_id", uploadID, "declared_bytes", declaredTotal, "decision", "quota_reserved")
	return progressOf(upload), nil
}

func (store *Store) Append(deviceID, uploadID string, chunk uint32, offset int64, final bool, payload []byte, now time.Time) (Progress, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.cleanupLocked(now)
	upload := store.records[recordKey(deviceID, uploadID)]
	if upload == nil || upload.Complete || upload.Final && len(payload) != 0 {
		return Progress{}, ErrAttachmentUnavailable
	}
	payloadBytes := int64(len(payload))
	if chunk != upload.NextChunk || offset != upload.ReceivedBytes || payloadBytes == 0 ||
		upload.ReceivedBytes > upload.DeclaredTotal || payloadBytes > upload.DeclaredTotal-upload.ReceivedBytes ||
		final != (payloadBytes == upload.DeclaredTotal-upload.ReceivedBytes) {
		return Progress{}, ErrInvalidAttachment
	}
	path := filepath.Join(store.activeRoot, upload.DataName)
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return Progress{}, ErrStorageUnavailable
	}
	oldBytes, oldChunk, oldFinal := upload.ReceivedBytes, upload.NextChunk, upload.Final
	if _, err = file.Seek(oldBytes, io.SeekStart); err == nil {
		var written int
		written, err = store.writeChunk(file, payload)
		if err == nil && written != len(payload) {
			err = io.ErrShortWrite
		}
	}
	if err == nil {
		err = file.Sync()
	}
	if err != nil {
		_ = file.Truncate(oldBytes)
		_ = file.Sync()
		_ = file.Close()
		store.logger.Error("[attachments] storage operation failed", "operation", "append_chunk", "device_id", deviceID, "upload_id", uploadID, "chunk", chunk, "error_class", fmt.Sprintf("%T", err), "decision", "rollback_partial_write")
		return Progress{}, ErrStorageUnavailable
	}
	if err = file.Close(); err != nil {
		_ = os.Truncate(path, oldBytes)
		store.logFailure("append_close", deviceID, uploadID, err, "rollback_partial_write")
		return Progress{}, ErrStorageUnavailable
	}
	upload.ReceivedBytes += payloadBytes
	upload.NextChunk++
	upload.Final = final
	if err := store.writeMetadata(store.activeRoot, upload); err != nil {
		upload.ReceivedBytes, upload.NextChunk, upload.Final = oldBytes, oldChunk, oldFinal
		_ = os.Truncate(path, oldBytes)
		store.logFailure("append_metadata", deviceID, uploadID, err, "rollback_partial_write")
		return Progress{}, ErrStorageUnavailable
	}
	store.logger.Debug("[attachments] chunk stored", "device_id", deviceID, "upload_id", uploadID, "chunk", chunk, "received_bytes", upload.ReceivedBytes, "final", final)
	return progressOf(upload), nil
}

func (store *Store) Resume(deviceID, uploadID string, now time.Time) (Progress, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.cleanupLocked(now)
	upload := store.records[recordKey(deviceID, uploadID)]
	if upload == nil || upload.Complete || upload.PublishingName != "" {
		return Progress{}, ErrAttachmentUnavailable
	}
	return progressOf(upload), nil
}

func (store *Store) Complete(deviceID, uploadID string, now time.Time) (Attachment, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.cleanupLocked(now)
	key := recordKey(deviceID, uploadID)
	upload := store.records[key]
	if upload == nil {
		return Attachment{}, ErrAttachmentUnavailable
	}
	if upload.Complete {
		return attachmentOf(store.completeRoot, upload), nil
	}
	if !upload.Final || upload.ReceivedBytes != upload.DeclaredTotal {
		return Attachment{}, ErrAttachmentUnavailable
	}
	activePath := filepath.Join(store.activeRoot, upload.DataName)
	digest, mediaType, err := store.inspect(activePath, upload.DeclaredTotal)
	if err != nil {
		if errors.Is(err, ErrAttachmentCorrupt) {
			store.rejectCorruptLocked(key, deviceID, uploadID, "size_mismatch")
			return Attachment{}, ErrAttachmentCorrupt
		}
		store.logFailure("complete_inspect", deviceID, uploadID, err, "retain_for_retry")
		return Attachment{}, ErrStorageUnavailable
	}
	if !strings.EqualFold(digest, upload.SHA256) {
		store.rejectCorruptLocked(key, deviceID, uploadID, "digest_mismatch")
		return Attachment{}, ErrAttachmentCorrupt
	}
	completed, err := os.CreateTemp(store.completeRoot, "attachment-*")
	if err != nil {
		store.logFailure("complete_allocate", deviceID, uploadID, err, "retain_for_retry")
		return Attachment{}, ErrStorageUnavailable
	}
	completedPath := completed.Name()
	if err := privatefiles.HardenFile(completed); err != nil {
		_ = completed.Close()
		_ = store.removePath(completedPath)
		store.logFailure("complete_harden", deviceID, uploadID, err, "retain_for_retry")
		return Attachment{}, ErrStorageUnavailable
	}
	if err := completed.Close(); err != nil {
		_ = store.removePath(completedPath)
		store.logFailure("complete_close", deviceID, uploadID, err, "retain_for_retry")
		return Attachment{}, ErrStorageUnavailable
	}
	if err := store.removePath(completedPath); err != nil {
		store.logFailure("complete_prepare_publish", deviceID, uploadID, err, "retain_for_retry")
		return Attachment{}, ErrStorageUnavailable
	}
	upload.PublishingName = filepath.Base(completedPath)
	if err := store.writeMetadata(store.activeRoot, upload); err != nil {
		upload.PublishingName = ""
		store.logFailure("complete_mark_publishing", deviceID, uploadID, err, "retain_for_retry")
		return Attachment{}, ErrStorageUnavailable
	}
	if err := os.Rename(activePath, completedPath); err != nil {
		upload.PublishingName = ""
		_ = store.writeMetadata(store.activeRoot, upload)
		store.logFailure("complete_publish", deviceID, uploadID, err, "retain_for_retry")
		return Attachment{}, ErrStorageUnavailable
	}
	if err := privatefiles.SyncDirectory(store.completeRoot); err != nil {
		store.logFailure("complete_sync_publish", deviceID, uploadID, err, "recover_on_restart")
		return Attachment{}, ErrStorageUnavailable
	}
	if store.afterPublishRename != nil {
		if err := store.afterPublishRename(); err != nil {
			store.logFailure("complete_after_publish", deviceID, uploadID, err, "recover_on_restart")
			return Attachment{}, ErrStorageUnavailable
		}
	}
	oldDataName := upload.DataName
	upload.DataName = upload.PublishingName
	upload.PublishingName = ""
	upload.Complete = true
	upload.MediaType = mediaType
	oldMetadataName := upload.MetadataName
	upload.MetadataName = strings.TrimSuffix(upload.DataName, filepath.Ext(upload.DataName)) + ".json"
	if err := store.writeMetadata(store.completeRoot, upload); err != nil {
		upload.PublishingName = upload.DataName
		upload.DataName, upload.MetadataName, upload.Complete, upload.MediaType = oldDataName, oldMetadataName, false, ""
		store.logFailure("complete_metadata", deviceID, uploadID, err, "recover_on_restart")
		return Attachment{}, ErrStorageUnavailable
	}
	if err := store.removePath(filepath.Join(store.activeRoot, oldMetadataName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		store.logger.Warn("[attachments] storage cleanup deferred", "operation", "complete_remove_active_metadata", "device_id", deviceID, "upload_id", uploadID, "error_class", fmt.Sprintf("%T", err), "decision", "completed_copy_remains_authoritative")
	}
	_ = privatefiles.SyncDirectory(store.activeRoot)
	store.logger.Info("[attachments] upload completed", "device_id", deviceID, "upload_id", uploadID, "received_bytes", upload.ReceivedBytes, "media_type", mediaType, "output_shape", "verified_local_file")
	return attachmentOf(store.completeRoot, upload), nil
}

func (store *Store) rejectCorruptLocked(key, deviceID, uploadID, reason string) {
	if removeErr := store.removeLocked(key); removeErr != nil {
		store.logFailure("complete_corrupt_cleanup", deviceID, uploadID, removeErr, "retain_quota_reservation")
	}
	store.logger.Warn("[attachments] completion rejected", "device_id", deviceID, "upload_id", uploadID, "branch_reason", reason, "decision", "delete_partial")
}

func (store *Store) Resolve(deviceID string, uploadIDs []string, now time.Time) ([]Attachment, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.cleanupLocked(now)
	if !validIdentifier(deviceID) || len(uploadIDs) == 0 {
		return nil, ErrAttachmentUnavailable
	}
	resolved := make([]Attachment, 0, len(uploadIDs))
	seen := make(map[string]struct{}, len(uploadIDs))
	for _, uploadID := range uploadIDs {
		if _, duplicate := seen[uploadID]; duplicate {
			return nil, ErrInvalidAttachment
		}
		seen[uploadID] = struct{}{}
		upload := store.records[recordKey(deviceID, uploadID)]
		if upload == nil || !upload.Complete {
			return nil, ErrAttachmentUnavailable
		}
		resolved = append(resolved, attachmentOf(store.completeRoot, upload))
	}
	return resolved, nil
}

func (store *Store) Cancel(deviceID, uploadID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	key := recordKey(deviceID, uploadID)
	if upload := store.records[key]; upload != nil && !upload.Complete {
		if err := store.removeLocked(key); err != nil {
			store.logFailure("cancel_cleanup", deviceID, uploadID, err, "retain_quota_reservation")
			return ErrStorageUnavailable
		}
		store.logger.Info("[attachments] upload cancelled", "device_id", deviceID, "upload_id", uploadID, "decision", "delete_partial")
	}
	return nil
}

func (store *Store) Release(deviceID string, uploadIDs []string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, uploadID := range uploadIDs {
		key := recordKey(deviceID, uploadID)
		upload := store.records[key]
		if upload == nil || !upload.Complete {
			return ErrAttachmentUnavailable
		}
	}
	for _, uploadID := range uploadIDs {
		if err := store.removeLocked(recordKey(deviceID, uploadID)); err != nil {
			store.logFailure("release_cleanup", deviceID, uploadID, err, "retain_quota_reservation")
			return ErrStorageUnavailable
		}
	}
	store.logger.Info("[attachments] completed files released", "device_id", deviceID, "attachment_count", len(uploadIDs), "decision", "delete_after_handoff")
	return nil
}

func (store *Store) restore(now time.Time) error {
	referenced := make(map[string]struct{})
	for _, source := range []struct {
		root     string
		complete bool
	}{{store.activeRoot, false}, {store.completeRoot, true}} {
		entries, err := os.ReadDir(source.root)
		if err != nil {
			return fmt.Errorf("%w: read metadata", ErrStorageUnavailable)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			metadataPath := filepath.Join(source.root, entry.Name())
			upload, err := readMetadata(metadataPath)
			if err != nil {
				store.logger.Warn("[attachments] startup cleanup", "operation", "restore_metadata", "error_class", fmt.Sprintf("%T", err), "decision", "delete_invalid_metadata")
				_ = store.removePath(metadataPath)
				continue
			}
			if !source.complete && upload.PublishingName != "" {
				recoveredRoot, recoverErr := store.recoverPublishing(upload, metadataPath, now)
				if recoverErr != nil {
					store.logger.Error("[attachments] startup recovery failed", "operation", "recover_publishing", "device_id", safeLogID(upload.DeviceID), "upload_id", safeLogID(upload.UploadID), "error_class", fmt.Sprintf("%T", recoverErr), "decision", "retain_for_next_start")
					if validRestoredFields(upload, now, store.limits) && filepath.Base(upload.PublishingName) == upload.PublishingName && upload.PublishingName != "." {
						upload.MetadataName = entry.Name()
						store.records[recordKey(upload.DeviceID, upload.UploadID)] = upload
						referenced[metadataPath] = struct{}{}
						referenced[filepath.Join(store.activeRoot, upload.DataName)] = struct{}{}
						referenced[filepath.Join(store.completeRoot, upload.PublishingName)] = struct{}{}
					}
					continue
				}
				source.root, source.complete = recoveredRoot, upload.Complete
				metadataPath = filepath.Join(source.root, upload.MetadataName)
			}
			if upload.Complete != source.complete || !store.validRestored(upload, source.root, now) {
				store.logger.Warn("[attachments] startup cleanup", "operation", "validate_restored_upload", "device_id", safeLogID(upload.DeviceID), "upload_id", safeLogID(upload.UploadID), "decision", "delete_invalid_upload")
				_ = store.removePath(metadataPath)
				_ = store.removePath(filepath.Join(source.root, upload.DataName))
				continue
			}
			upload.MetadataName = entry.Name()
			if upload.Complete {
				upload.MetadataName = filepath.Base(metadataPath)
			}
			key := recordKey(upload.DeviceID, upload.UploadID)
			if existing := store.records[key]; existing != nil {
				if existing.Complete == upload.Complete && existing.DataName == upload.DataName {
					referenced[filepath.Join(source.root, upload.DataName)] = struct{}{}
					referenced[metadataPath] = struct{}{}
					continue
				}
				store.logger.Warn("[attachments] startup cleanup", "operation", "restore_duplicate", "device_id", safeLogID(upload.DeviceID), "upload_id", safeLogID(upload.UploadID), "decision", "delete_duplicate")
				_ = store.removePath(metadataPath)
				_ = store.removePath(filepath.Join(source.root, upload.DataName))
				continue
			}
			store.records[key] = upload
			referenced[filepath.Join(source.root, upload.DataName)] = struct{}{}
			referenced[metadataPath] = struct{}{}
		}
	}
	for _, root := range []string{store.activeRoot, store.completeRoot} {
		entries, _ := os.ReadDir(root)
		for _, entry := range entries {
			path := filepath.Join(root, entry.Name())
			if _, keep := referenced[path]; !keep {
				if err := store.removePath(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					if info, statErr := os.Lstat(path); statErr == nil && info.Mode().IsRegular() && info.Size() > 0 {
						store.untrackedBytes += info.Size()
					}
					store.logger.Error("[attachments] startup cleanup failed", "operation", "remove_orphan", "error_class", fmt.Sprintf("%T", err), "decision", "leave_untrusted_file_unreferenced")
				} else {
					store.logger.Info("[attachments] startup cleanup", "operation", "remove_orphan", "decision", "delete_unreferenced_file")
				}
			}
		}
	}
	return nil
}

func (store *Store) recoverPublishing(upload *record, activeMetadataPath string, now time.Time) (string, error) {
	if !validRestoredFields(upload, now, store.limits) || filepath.Base(upload.PublishingName) != upload.PublishingName || upload.PublishingName == "." {
		return "", ErrInvalidAttachment
	}
	activePath := filepath.Join(store.activeRoot, upload.DataName)
	publishedPath := filepath.Join(store.completeRoot, upload.PublishingName)
	activeInfo, activeErr := os.Lstat(activePath)
	publishedInfo, publishedErr := os.Lstat(publishedPath)
	activeExists := activeErr == nil && activeInfo.Mode().IsRegular()
	publishedExists := publishedErr == nil && publishedInfo.Mode().IsRegular()
	switch {
	case activeExists && !publishedExists:
		upload.PublishingName = ""
		if err := store.writeMetadata(store.activeRoot, upload); err != nil {
			return "", err
		}
		store.logger.Info("[attachments] startup recovery", "operation", "recover_publishing", "device_id", upload.DeviceID, "upload_id", upload.UploadID, "decision", "resume_active_upload")
		return store.activeRoot, nil
	case !activeExists && publishedExists:
		digest, mediaType, err := store.inspect(publishedPath, upload.DeclaredTotal)
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(digest, upload.SHA256) {
			return "", ErrAttachmentCorrupt
		}
		upload.DataName = upload.PublishingName
		upload.PublishingName = ""
		upload.Complete = true
		upload.MediaType = mediaType
		upload.MetadataName = strings.TrimSuffix(upload.DataName, filepath.Ext(upload.DataName)) + ".json"
		if err := store.writeMetadata(store.completeRoot, upload); err != nil {
			return "", err
		}
		if err := store.removePath(activeMetadataPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		store.logger.Info("[attachments] startup recovery", "operation", "recover_publishing", "device_id", upload.DeviceID, "upload_id", upload.UploadID, "decision", "finish_completed_upload")
		return store.completeRoot, nil
	default:
		return "", ErrInvalidAttachment
	}
}

func (store *Store) validRestored(upload *record, root string, now time.Time) bool {
	if !validRestoredFields(upload, now, store.limits) || upload.PublishingName != "" {
		return false
	}
	path := filepath.Join(root, upload.DataName)
	info, err := os.Lstat(path)
	if err != nil || privatefiles.ValidateFile(path, info) != nil || info.Size() < upload.ReceivedBytes {
		return false
	}
	if upload.Complete {
		digest, mediaType, err := store.inspect(path, upload.DeclaredTotal)
		return err == nil && info.Size() == upload.DeclaredTotal && strings.EqualFold(digest, upload.SHA256) && mediaType == upload.MediaType
	}
	if upload.Final != (upload.ReceivedBytes == upload.DeclaredTotal) || (upload.ReceivedBytes == 0) != (upload.NextChunk == 0) {
		return false
	}
	if info.Size() > upload.ReceivedBytes && os.Truncate(path, upload.ReceivedBytes) != nil {
		return false
	}
	return true
}

func validRestoredFields(upload *record, now time.Time, limits Limits) bool {
	return upload.Version == metadataVersion && validIdentifier(upload.DeviceID) && validIdentifier(upload.UploadID) &&
		upload.DeclaredTotal > 0 && upload.DeclaredTotal <= limits.MaxFileBytes && validSHA(upload.SHA256) &&
		upload.ReceivedBytes >= 0 && upload.ReceivedBytes <= upload.DeclaredTotal && upload.ExpiresAt.After(now) &&
		filepath.Base(upload.DataName) == upload.DataName && upload.DataName != "."
}

func (store *Store) cleanupLocked(now time.Time) {
	for key, upload := range store.records {
		if !upload.ExpiresAt.After(now) {
			if err := store.removeLocked(key); err != nil {
				store.logFailure("expiry_cleanup", upload.DeviceID, upload.UploadID, err, "retain_quota_reservation")
			}
		}
	}
}

func (store *Store) removeLocked(key string) error {
	upload := store.records[key]
	if upload == nil {
		return nil
	}
	root := store.activeRoot
	if upload.Complete {
		root = store.completeRoot
	}
	if upload.PublishingName != "" {
		if err := store.removePath(filepath.Join(store.completeRoot, upload.PublishingName)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := store.removePath(filepath.Join(root, upload.DataName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := store.removePath(filepath.Join(root, upload.MetadataName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := privatefiles.SyncDirectory(root); err != nil {
		return err
	}
	delete(store.records, key)
	return nil
}

func (store *Store) writeMetadata(root string, upload *record) error {
	temporary, err := os.CreateTemp(root, ".metadata-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer store.removePath(temporaryPath)
	if err = privatefiles.HardenFile(temporary); err == nil {
		err = json.NewEncoder(temporary).Encode(upload)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, filepath.Join(root, upload.MetadataName)); err != nil {
		return err
	}
	return privatefiles.SyncDirectory(root)
}

func readMetadata(path string) (*record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 16*1024))
	decoder.DisallowUnknownFields()
	var upload record
	if err := decoder.Decode(&upload); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidAttachment
	}
	return &upload, nil
}

func inspectFile(path string, expectedSize int64) (string, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	hasher := sha256.New()
	var header [512]byte
	headerBytes, readErr := io.ReadFull(file, header[:])
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		return "", "", readErr
	}
	if _, err := hasher.Write(header[:headerBytes]); err != nil {
		return "", "", err
	}
	written, err := io.Copy(hasher, io.LimitReader(file, expectedSize-int64(headerBytes)+1))
	if err != nil || int64(headerBytes)+written != expectedSize {
		return "", "", ErrAttachmentCorrupt
	}
	return hex.EncodeToString(hasher.Sum(nil)), http.DetectContentType(header[:headerBytes]), nil
}

func validIdentifier(value string) bool { return identifierPattern.MatchString(value) }

func validSHA(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func recordKey(deviceID, uploadID string) string { return deviceID + "\x00" + uploadID }

func progressOf(upload *record) Progress {
	return Progress{NextChunk: upload.NextChunk, ReceivedBytes: upload.ReceivedBytes, ExpiresAt: upload.ExpiresAt}
}

func attachmentOf(root string, upload *record) Attachment {
	return Attachment{ID: upload.UploadID, Path: filepath.Join(root, upload.DataName), SHA256: upload.SHA256, MediaType: upload.MediaType}
}

func (store *Store) logFailure(operation, deviceID, uploadID string, err error, decision string) {
	store.logger.Error(
		"[attachments] storage operation failed",
		"operation", operation,
		"device_id", safeLogID(deviceID),
		"upload_id", safeLogID(uploadID),
		"error_class", fmt.Sprintf("%T", err),
		"decision", decision,
	)
}

func safeLogID(value string) string {
	if validIdentifier(value) {
		return value
	}
	return "invalid"
}
