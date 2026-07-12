package contract

import (
	"encoding/json"
	"errors"
	"hash"
	"sync"
	"time"
)

const (
	ProtocolMajor           = 1
	ProtocolMinor           = 0
	MaxJSONFrameBytes       = 256 * 1024
	MaxAttachmentBytes      = 20 * 1024 * 1024
	MaxAttachmentFrameBytes = MaxAttachmentBytes + 4096 + 12 + 32
	MaxDeviceUploads        = 2
	MaxGlobalUploads        = 4
	MaxTemporaryBytes       = 100 * 1024 * 1024
	UploadExpirySeconds     = 15 * 60
)

var (
	ErrColdResumeCursor   = errors.New("cold resume cannot include a cursor")
	ErrDuplicateAction    = errors.New("action ID was already used")
	ErrFrameTooLarge      = errors.New("mobile protocol frame is too large")
	ErrInvalidAck         = errors.New("mobile protocol acknowledgement is invalid")
	ErrInvalidAction      = errors.New("mobile protocol action is invalid")
	ErrInvalidActionState = errors.New("mobile protocol action state is invalid")
	ErrInvalidAttachment  = errors.New("mobile protocol attachment is invalid")
	ErrAttachmentQuota    = errors.New("mobile protocol attachment quota exceeded")
	ErrInvalidEnvelope    = errors.New("mobile protocol envelope is invalid")
	ErrSequenceGap        = errors.New("mobile protocol sequence has a gap")
	ErrSessionClosed      = errors.New("mobile protocol session is closed")
	ErrUnsupportedVersion = errors.New("mobile protocol version is unsupported")
)

type Version struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

type Message struct {
	Version   Version         `json:"version"`
	MessageID string          `json:"messageId"`
	Sender    string          `json:"sender"`
	Type      string          `json:"type"`
	Sequence  *uint64         `json:"seq,omitempty"`
	Body      json.RawMessage `json:"body"`
}

type AttachmentChunk struct {
	SessionID     string
	UploadID      string
	Chunk         uint32
	Offset        int64
	DeclaredTotal int64
	Final         bool
	Payload       []byte
}

type AttachmentOffer struct {
	UploadID      string
	DeclaredTotal int64
	SHA256        string
}

type AttachmentAck struct {
	UploadID      string
	ReceivedBytes int64
	SHA256        string
}

// RetainedAttachment is companion-owned upload state restored from authenticated
// temporary storage. A phone resume claim is accepted only when it matches this state.
type RetainedAttachment struct {
	Offer     AttachmentOffer
	Received  []byte
	NextChunk uint32
	ExpiresAt time.Time
}

type AttachmentLimits struct {
	MaxAttachmentBytes int64
	MaxDeviceUploads   int
	MaxGlobalUploads   int
	MaxTemporaryBytes  int64
}

func DefaultAttachmentLimits() AttachmentLimits {
	return AttachmentLimits{
		MaxAttachmentBytes: MaxAttachmentBytes,
		MaxDeviceUploads:   MaxDeviceUploads,
		MaxGlobalUploads:   MaxGlobalUploads,
		MaxTemporaryBytes:  MaxTemporaryBytes,
	}
}

type AttachmentQuota struct {
	mu           sync.Mutex
	limits       AttachmentLimits
	active       int
	reserved     int64
	byDevice     map[string]int
	nextToken    uint64
	reservations map[uint64]quotaReservation
}

func NewAttachmentQuota(limits AttachmentLimits) *AttachmentQuota {
	return &AttachmentQuota{limits: limits, byDevice: make(map[string]int), reservations: make(map[uint64]quotaReservation)}
}

type quotaReservation struct {
	deviceID  string
	size      int64
	expiresAt time.Time
}

type uploadState struct {
	offer      AttachmentOffer
	next       uint32
	received   int64
	final      bool
	hash       hash.Hash
	expiresAt  time.Time
	quotaToken uint64
}
