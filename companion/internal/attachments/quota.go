package attachments

import (
	"errors"
	"time"
)

var (
	ErrInvalidAttachment     = errors.New("attachment is invalid")
	ErrAttachmentTooLarge    = errors.New("attachment exceeds the file size limit")
	ErrAttachmentQuota       = errors.New("attachment storage quota exceeded")
	ErrAttachmentUnavailable = errors.New("attachment is unavailable")
	ErrAttachmentCorrupt     = errors.New("attachment digest does not match")
	ErrAttachmentClaimed     = errors.New("attachment belongs to another durable action")
	ErrStorageUnavailable    = errors.New("attachment storage is unavailable")
)

type Limits struct {
	MaxFileBytes      int64
	MaxDeviceUploads  int
	MaxGlobalUploads  int
	MaxTemporaryBytes int64
	Expiry            time.Duration
}

func DefaultLimits() Limits {
	return Limits{
		MaxFileBytes:      20 * 1024 * 1024,
		MaxDeviceUploads:  2,
		MaxGlobalUploads:  4,
		MaxTemporaryBytes: 100 * 1024 * 1024,
		Expiry:            15 * time.Minute,
	}
}

func (limits Limits) valid() bool {
	return limits.MaxFileBytes > 0 && limits.MaxDeviceUploads > 0 && limits.MaxGlobalUploads > 0 &&
		limits.MaxTemporaryBytes >= limits.MaxFileBytes && limits.Expiry > 0
}

func withinQuota(records map[string]*record, untrackedBytes int64, deviceID string, declaredTotal int64, limits Limits) bool {
	activeGlobal, activeDevice := 0, 0
	var reserved int64
	for _, upload := range records {
		reserved += upload.DeclaredTotal
		if upload.Complete {
			continue
		}
		activeGlobal++
		if upload.DeviceID == deviceID {
			activeDevice++
		}
	}
	reserved += untrackedBytes
	return activeGlobal < limits.MaxGlobalUploads && activeDevice < limits.MaxDeviceUploads &&
		reserved <= limits.MaxTemporaryBytes && declaredTotal <= limits.MaxTemporaryBytes-reserved
}
