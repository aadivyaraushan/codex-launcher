package contract

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	approvedprojects "github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

const (
	attachmentHeaderMax = 4096
	attachmentTagBytes  = 32
	attachmentVersion   = 1
	maxProtocolInteger  = uint64(1<<63 - 1)
	maxProtocolChunk    = uint64(1<<31 - 1)
)

var attachmentMagic = [4]byte{'C', 'L', 'A', 'T'}
var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
var projectIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func DecodeText(frame []byte) (Message, error) {
	if len(frame) > MaxJSONFrameBytes {
		return Message{}, ErrFrameTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	var message Message
	if err := decoder.Decode(&message); err != nil {
		return Message{}, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || !validID(message.MessageID) || len(message.Body) == 0 ||
		(message.Sender != "phone" && message.Sender != "companion") {
		return Message{}, ErrInvalidEnvelope
	}
	if message.Version.Major != ProtocolMajor || message.Version.Minor < 0 || uint64(message.Version.Minor) > maxProtocolChunk {
		return Message{}, ErrUnsupportedVersion
	}
	sequenceType := message.Type == "snapshot" || message.Type == "event" || message.Type == "action_result" || message.Type == "attachment_ack"
	if sequenceType != (message.Sequence != nil) || message.Sequence != nil && (*message.Sequence == 0 || *message.Sequence > maxProtocolInteger) {
		return Message{}, ErrInvalidEnvelope
	}
	if err := validateBody(message); err != nil {
		return Message{}, err
	}
	return message, nil
}

type Session struct {
	seenActions       map[string]struct{}
	actionStates      map[string]string
	uploads           map[string]*uploadState
	attachmentKey     []byte
	expectedSessionID string
	quota             *AttachmentQuota
	requestedUploads  map[string]uint32
	completedUploads  map[string]AttachmentAck
	deviceID          string
	hasSequence       bool
	hasFreshSnapshot  bool
	lastSeq           uint64
	lastAck           uint64
	hasAck            bool
	closed            bool
}

func NewSession() *Session {
	return NewSessionWithAttachments(nil, NewAttachmentQuota(DefaultAttachmentLimits()), "", "")
}

func NewSessionWithAttachments(key []byte, quota *AttachmentQuota, sessionID, deviceID string) *Session {
	if quota == nil {
		quota = NewAttachmentQuota(DefaultAttachmentLimits())
	}
	return &Session{
		seenActions: make(map[string]struct{}), actionStates: make(map[string]string),
		uploads: make(map[string]*uploadState), attachmentKey: append([]byte(nil), key...), expectedSessionID: sessionID, quota: quota,
		requestedUploads: make(map[string]uint32), completedUploads: make(map[string]AttachmentAck), deviceID: deviceID,
	}
}

func (session *Session) AcceptText(frame []byte) (message Message, err error) {
	if session.closed {
		return Message{}, ErrSessionClosed
	}
	defer func() {
		if err != nil {
			session.closed = true
		}
	}()
	message, err = DecodeText(frame)
	if err != nil {
		return Message{}, err
	}
	if message.Type == "hello" {
		var body struct {
			ClientInstanceID string `json:"clientInstanceId"`
			Resume           struct {
				Mode    string            `json:"mode"`
				LastAck *uint64           `json:"lastAck,omitempty"`
				Uploads map[string]uint32 `json:"uploads,omitempty"`
			} `json:"resume"`
		}
		_ = json.Unmarshal(message.Body, &body)
		if session.deviceID == "" {
			session.deviceID = body.ClientInstanceID
		} else if session.deviceID != body.ClientInstanceID {
			return Message{}, ErrInvalidEnvelope
		}
		if body.Resume.Mode == "no_local_state" && body.Resume.LastAck != nil {
			return Message{}, ErrColdResumeCursor
		}
		if body.Resume.Mode == "no_local_state" && len(body.Resume.Uploads) != 0 {
			return Message{}, ErrColdResumeCursor
		}
		if body.Resume.Mode == "warm" && body.Resume.LastAck != nil {
			session.lastSeq = *body.Resume.LastAck
			session.hasSequence = true
			session.lastAck = *body.Resume.LastAck
			session.hasAck = true
		}
		for uploadID, nextChunk := range body.Resume.Uploads {
			session.requestedUploads[uploadID] = nextChunk
		}
	}
	if message.Sequence != nil {
		if message.Type == "snapshot" {
			if session.hasSequence && *message.Sequence <= session.lastSeq {
				return Message{}, ErrSequenceGap
			}
			session.lastSeq = *message.Sequence
			session.hasSequence = true
			session.hasFreshSnapshot = true
		} else if !session.hasSequence || *message.Sequence != session.lastSeq+1 {
			return Message{}, ErrSequenceGap
		} else {
			session.lastSeq = *message.Sequence
		}
	}
	if message.Type == "action" {
		var body struct {
			ActionID string `json:"actionId"`
		}
		_ = json.Unmarshal(message.Body, &body)
		if _, duplicate := session.seenActions[body.ActionID]; duplicate {
			return Message{}, ErrDuplicateAction
		}
		session.seenActions[body.ActionID] = struct{}{}
	}
	if message.Type == "action_result" {
		var body struct {
			ActionID string `json:"actionId"`
			State    string `json:"state"`
		}
		_ = json.Unmarshal(message.Body, &body)
		if !validActionTransition(session.actionStates[body.ActionID], body.State) {
			return Message{}, ErrInvalidActionState
		}
		session.actionStates[body.ActionID] = body.State
	}
	if message.Type == "ack" {
		var body struct {
			ThroughSeq uint64 `json:"throughSeq"`
		}
		_ = json.Unmarshal(message.Body, &body)
		if !session.hasSequence || body.ThroughSeq > session.lastSeq || session.hasAck && body.ThroughSeq < session.lastAck {
			return Message{}, ErrInvalidAck
		}
		session.lastAck = body.ThroughSeq
		session.hasAck = true
	}
	if message.Type == "attachment_offer" {
		var body struct {
			UploadID      string `json:"uploadId"`
			DeclaredTotal int64  `json:"declaredTotal"`
			SHA256        string `json:"sha256"`
		}
		_ = json.Unmarshal(message.Body, &body)
		if err := session.OfferAttachment(AttachmentOffer(body)); err != nil {
			return Message{}, err
		}
	}
	if message.Type == "attachment_cancel" {
		var body struct {
			UploadID string `json:"uploadId"`
		}
		_ = json.Unmarshal(message.Body, &body)
		session.CancelAttachment(body.UploadID)
	}
	if message.Type == "attachment_complete" {
		var body struct {
			UploadID string `json:"uploadId"`
		}
		_ = json.Unmarshal(message.Body, &body)
		ack, completeErr := session.CompleteAttachment(body.UploadID)
		if completeErr != nil {
			return Message{}, completeErr
		}
		session.completedUploads[body.UploadID] = ack
	}
	return message, nil
}

func (session *Session) CompletedAttachment(uploadID string) (AttachmentAck, bool) {
	ack, okay := session.completedUploads[uploadID]
	return ack, okay
}

func (session *Session) HasFreshSnapshot() bool { return session.hasFreshSnapshot }

func (session *Session) LastSequence() uint64 { return session.lastSeq }

func (session *Session) AcceptedResumeChunk(uploadID string) (uint32, bool) {
	session.ExpireAttachments(time.Now())
	claimed, claimedExists := session.requestedUploads[uploadID]
	retained := session.uploads[uploadID]
	if !claimedExists || retained == nil || retained.next != claimed {
		return 0, false
	}
	return retained.next, true
}

func (session *Session) RestoreAttachment(retained RetainedAttachment) error {
	offer := retained.Offer
	if !validID(offer.UploadID) || offer.DeclaredTotal <= 0 || int64(len(retained.Received)) >= offer.DeclaredTotal ||
		!isSHA256(offer.SHA256) || !retained.ExpiresAt.After(time.Now()) ||
		(len(retained.Received) == 0) != (retained.NextChunk == 0) {
		return ErrInvalidAttachment
	}
	if _, exists := session.uploads[offer.UploadID]; exists || offer.DeclaredTotal > MaxAttachmentBytes || offer.DeclaredTotal > session.quota.limits.MaxAttachmentBytes {
		return ErrAttachmentQuota
	}
	quotaToken, reserved := session.quota.reserve(session.deviceID, offer.DeclaredTotal, retained.ExpiresAt, time.Now())
	if !reserved {
		return ErrAttachmentQuota
	}
	digest := sha256.New()
	_, _ = digest.Write(retained.Received)
	session.uploads[offer.UploadID] = &uploadState{
		offer: offer, next: retained.NextChunk, received: int64(len(retained.Received)), hash: digest, expiresAt: retained.ExpiresAt, quotaToken: quotaToken,
	}
	return nil
}

func (session *Session) OfferAttachment(offer AttachmentOffer) error {
	return session.OfferAttachmentAt(offer, time.Now())
}

func (session *Session) OfferAttachmentAt(offer AttachmentOffer, now time.Time) error {
	session.ExpireAttachments(now)
	if !validID(offer.UploadID) || offer.DeclaredTotal <= 0 || !isSHA256(offer.SHA256) {
		return ErrInvalidAttachment
	}
	expiresAt := now.Add(time.Duration(UploadExpirySeconds) * time.Second)
	if _, exists := session.uploads[offer.UploadID]; exists || offer.DeclaredTotal > MaxAttachmentBytes || offer.DeclaredTotal > session.quota.limits.MaxAttachmentBytes {
		return ErrAttachmentQuota
	}
	quotaToken, reserved := session.quota.reserve(session.deviceID, offer.DeclaredTotal, expiresAt, now)
	if !reserved {
		return ErrAttachmentQuota
	}
	session.uploads[offer.UploadID] = &uploadState{
		offer: offer, hash: sha256.New(), expiresAt: expiresAt, quotaToken: quotaToken,
	}
	return nil
}

func (session *Session) ExpireAttachments(now time.Time) {
	for uploadID, upload := range session.uploads {
		if now.After(upload.expiresAt) {
			session.releaseAttachment(uploadID)
		}
	}
}

func (session *Session) AcceptAttachmentFrame(frame []byte) error {
	session.ExpireAttachments(time.Now())
	chunk, err := DecodeAttachmentFrame(frame, session.attachmentKey)
	if err != nil {
		return err
	}
	if session.expectedSessionID == "" || chunk.SessionID != session.expectedSessionID {
		return ErrInvalidAttachment
	}
	upload := session.uploads[chunk.UploadID]
	payloadLength := int64(len(chunk.Payload))
	if upload == nil || upload.final || chunk.Chunk != upload.next || chunk.Offset != upload.received ||
		chunk.DeclaredTotal != upload.offer.DeclaredTotal || upload.received > upload.offer.DeclaredTotal ||
		payloadLength > upload.offer.DeclaredTotal-upload.received {
		return ErrInvalidAttachment
	}
	if chunk.Final && payloadLength != upload.offer.DeclaredTotal-upload.received {
		return ErrInvalidAttachment
	}
	_, _ = upload.hash.Write(chunk.Payload)
	upload.received += int64(len(chunk.Payload))
	upload.next++
	upload.final = chunk.Final
	return nil
}

func (session *Session) CompleteAttachment(uploadID string) (AttachmentAck, error) {
	session.ExpireAttachments(time.Now())
	upload := session.uploads[uploadID]
	if upload == nil || !upload.final || upload.received != upload.offer.DeclaredTotal {
		return AttachmentAck{}, ErrInvalidAttachment
	}
	digest := hex.EncodeToString(upload.hash.Sum(nil))
	if !strings.EqualFold(digest, upload.offer.SHA256) {
		session.releaseAttachment(uploadID)
		return AttachmentAck{}, ErrInvalidAttachment
	}
	ack := AttachmentAck{UploadID: uploadID, ReceivedBytes: upload.received, SHA256: digest}
	session.releaseAttachment(uploadID)
	return ack, nil
}

func (session *Session) CancelAttachment(uploadID string) {
	session.releaseAttachment(uploadID)
}

func (session *Session) releaseAttachment(uploadID string) {
	upload := session.uploads[uploadID]
	if upload == nil {
		return
	}
	delete(session.uploads, uploadID)
	session.quota.release(upload.quotaToken)
}

func (quota *AttachmentQuota) reserve(deviceID string, size int64, expiresAt, now time.Time) (uint64, bool) {
	quota.mu.Lock()
	defer quota.mu.Unlock()
	quota.pruneExpiredLocked(now)
	if deviceID == "" || quota.byDevice[deviceID] >= MaxDeviceUploads || quota.byDevice[deviceID] >= quota.limits.MaxDeviceUploads ||
		quota.active >= MaxGlobalUploads || quota.active >= quota.limits.MaxGlobalUploads ||
		quota.reserved > MaxTemporaryBytes || size > MaxTemporaryBytes-quota.reserved ||
		quota.reserved > quota.limits.MaxTemporaryBytes || size > quota.limits.MaxTemporaryBytes-quota.reserved {
		return 0, false
	}
	quota.nextToken++
	token := quota.nextToken
	quota.byDevice[deviceID]++
	quota.active++
	quota.reserved += size
	quota.reservations[token] = quotaReservation{deviceID: deviceID, size: size, expiresAt: expiresAt}
	return token, true
}

func (quota *AttachmentQuota) release(token uint64) {
	quota.mu.Lock()
	defer quota.mu.Unlock()
	quota.releaseLocked(token)
}

func (quota *AttachmentQuota) pruneExpiredLocked(now time.Time) {
	for token, reservation := range quota.reservations {
		if now.After(reservation.expiresAt) {
			quota.releaseLocked(token)
		}
	}
}

func (quota *AttachmentQuota) releaseLocked(token uint64) {
	reservation, exists := quota.reservations[token]
	if !exists {
		return
	}
	delete(quota.reservations, token)
	quota.active--
	quota.reserved -= reservation.size
	quota.byDevice[reservation.deviceID]--
	if quota.byDevice[reservation.deviceID] == 0 {
		delete(quota.byDevice, reservation.deviceID)
	}
}

type attachmentHeader struct {
	SessionID     string `json:"sessionId"`
	UploadID      string `json:"uploadId"`
	Chunk         uint32 `json:"chunk"`
	Offset        int64  `json:"offset"`
	DeclaredTotal int64  `json:"declaredTotal"`
	SHA256        string `json:"sha256"`
}

func EncodeAttachmentFrame(chunk AttachmentChunk, key []byte) ([]byte, error) {
	payloadLength := int64(len(chunk.Payload))
	if len(key) < 32 || !validID(chunk.SessionID) || !validID(chunk.UploadID) || uint64(chunk.Chunk) > maxProtocolChunk || chunk.Offset < 0 ||
		chunk.DeclaredTotal <= 0 || chunk.DeclaredTotal > MaxAttachmentBytes || payloadLength > chunk.DeclaredTotal || chunk.Offset > chunk.DeclaredTotal-payloadLength {
		return nil, ErrInvalidAttachment
	}
	digest := sha256.Sum256(chunk.Payload)
	header, err := json.Marshal(attachmentHeader{
		SessionID: chunk.SessionID, UploadID: chunk.UploadID, Chunk: chunk.Chunk, Offset: chunk.Offset,
		DeclaredTotal: chunk.DeclaredTotal, SHA256: hex.EncodeToString(digest[:]),
	})
	if err != nil || len(header) > attachmentHeaderMax {
		return nil, ErrInvalidAttachment
	}
	frame := make([]byte, 12+len(header)+len(chunk.Payload)+attachmentTagBytes)
	copy(frame[:4], attachmentMagic[:])
	frame[4] = attachmentVersion
	if chunk.Final {
		frame[5] = 1
	}
	binary.BigEndian.PutUint16(frame[6:8], uint16(len(header)))
	binary.BigEndian.PutUint32(frame[8:12], uint32(len(chunk.Payload)))
	copy(frame[12:], header)
	copy(frame[12+len(header):], chunk.Payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(frame[:len(frame)-attachmentTagBytes])
	copy(frame[len(frame)-attachmentTagBytes:], mac.Sum(nil))
	return frame, nil
}

func DecodeAttachmentFrame(frame, key []byte) (AttachmentChunk, error) {
	if len(key) < 32 || len(frame) < 12+attachmentTagBytes || len(frame) > MaxAttachmentFrameBytes || !bytes.Equal(frame[:4], attachmentMagic[:]) || frame[4] != attachmentVersion || frame[5]&^byte(1) != 0 {
		return AttachmentChunk{}, ErrInvalidAttachment
	}
	headerLength := int(binary.BigEndian.Uint16(frame[6:8]))
	payloadLength := int(binary.BigEndian.Uint32(frame[8:12]))
	if headerLength == 0 || headerLength > attachmentHeaderMax || len(frame) != 12+headerLength+payloadLength+attachmentTagBytes {
		return AttachmentChunk{}, ErrInvalidAttachment
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(frame[:len(frame)-attachmentTagBytes])
	if subtle.ConstantTimeCompare(mac.Sum(nil), frame[len(frame)-attachmentTagBytes:]) != 1 {
		return AttachmentChunk{}, ErrInvalidAttachment
	}
	decoder := json.NewDecoder(bytes.NewReader(frame[12 : 12+headerLength]))
	decoder.DisallowUnknownFields()
	var header attachmentHeader
	if decoder.Decode(&header) != nil || decoder.Decode(&struct{}{}) != io.EOF || !validID(header.SessionID) || !validID(header.UploadID) ||
		uint64(header.Chunk) > maxProtocolChunk || header.Offset < 0 || header.DeclaredTotal <= 0 || header.DeclaredTotal > MaxAttachmentBytes || !isSHA256(header.SHA256) {
		return AttachmentChunk{}, ErrInvalidAttachment
	}
	payload := append([]byte(nil), frame[12+headerLength:len(frame)-attachmentTagBytes]...)
	digest := sha256.Sum256(payload)
	payloadBytes := int64(len(payload))
	if !strings.EqualFold(hex.EncodeToString(digest[:]), header.SHA256) || payloadBytes > header.DeclaredTotal || header.Offset > header.DeclaredTotal-payloadBytes {
		return AttachmentChunk{}, ErrInvalidAttachment
	}
	return AttachmentChunk{
		SessionID: header.SessionID, UploadID: header.UploadID, Chunk: header.Chunk, Offset: header.Offset,
		DeclaredTotal: header.DeclaredTotal, Final: frame[5]&1 == 1, Payload: payload,
	}, nil
}

func validateBody(message Message) error {
	var body map[string]json.RawMessage
	if json.Unmarshal(message.Body, &body) != nil {
		return ErrInvalidEnvelope
	}
	switch message.Type {
	case "hello":
		if message.Sender != "phone" || !exactKeys(body, "clientInstanceId", "supportedMajors", "resume") || !boundedString(body["clientInstanceId"], 128) {
			return ErrInvalidEnvelope
		}
		var majors []int
		if json.Unmarshal(body["supportedMajors"], &majors) != nil || len(majors) == 0 || !uniquePositiveInts(majors) || !containsInt(majors, ProtocolMajor) {
			return ErrUnsupportedVersion
		}
		var resume map[string]json.RawMessage
		mode := ""
		if json.Unmarshal(body["resume"], &resume) != nil || !onlyAllowedKeys(resume, "mode", "lastAck", "uploads") ||
			(func() bool { mode = stringValue(resume["mode"]); return mode != "warm" && mode != "no_local_state" })() || !validateResumeUploads(resume["uploads"]) {
			return ErrInvalidEnvelope
		}
		lastAck, hasLastAck := uintValueOK(resume["lastAck"])
		if mode == "no_local_state" && (resume["lastAck"] != nil || resume["uploads"] != nil) {
			return ErrColdResumeCursor
		}
		if mode == "warm" && (!hasLastAck || lastAck == 0) {
			return ErrInvalidEnvelope
		}
	case "welcome":
		if message.Sender != "companion" || !exactKeys(body, "sessionId", "capabilities", "limits") || !boundedString(body["sessionId"], 128) ||
			!validateStringArray(body["capabilities"]) || !validateLimits(body["limits"]) {
			return ErrInvalidEnvelope
		}
	case "snapshot":
		if message.Sender != "companion" || message.Sequence == nil || !exactKeys(body, "baseSeq", "computerName", "projects", "tasks") ||
			uintValue(body["baseSeq"]) != *message.Sequence || !safeDisplayString(body["computerName"], 80) ||
			!validateProjects(body["projects"]) || !validateTasks(body["tasks"]) {
			return ErrInvalidEnvelope
		}
	case "event":
		if message.Sender != "companion" || message.Sequence == nil || !exactKeys(body, "taskId", "event", "state", "summary") ||
			!validID(stringValue(body["taskId"])) || !knownEvent(stringValue(body["event"])) || !knownTaskState(stringValue(body["state"])) || !boundedString(body["summary"], 512) {
			return ErrInvalidEnvelope
		}
	case "action_result":
		state := stringValue(body["state"])
		if message.Sender != "companion" || message.Sequence == nil || !onlyAllowedKeys(body, "actionId", "state", "error") ||
			!validID(stringValue(body["actionId"])) || !knownActionState(state) || !validateOptionalError(body["error"], state == "failed" || state == "outcome_unknown") {
			return ErrInvalidActionState
		}
	case "ack":
		through, okay := uintValueOK(body["throughSeq"])
		if message.Sender != "phone" || !exactKeys(body, "throughSeq") || !okay || through == 0 || through > maxProtocolInteger {
			return ErrInvalidAck
		}
	case "action":
		return validateAction(message.Sender, body)
	case "attachment_offer":
		if message.Sender != "phone" || !exactKeys(body, "uploadId", "declaredTotal", "sha256") ||
			!validID(stringValue(body["uploadId"])) || !uintInRange(body["declaredTotal"], 1, MaxAttachmentBytes) || !isSHA256(stringValue(body["sha256"])) {
			return ErrInvalidAttachment
		}
	case "attachment_cancel", "attachment_complete":
		if message.Sender != "phone" || !exactKeys(body, "uploadId") || !validID(stringValue(body["uploadId"])) {
			return ErrInvalidAttachment
		}
	case "attachment_ack":
		state := stringValue(body["state"])
		if message.Sender != "companion" || message.Sequence == nil || !exactKeys(body, "uploadId", "state", "receivedBytes", "sha256", "nextChunk") ||
			!validID(stringValue(body["uploadId"])) || (state != "accepted" && state != "complete" && state != "cancelled") ||
			!uintInRange(body["receivedBytes"], 0, MaxAttachmentBytes) || !uintInRange64(body["nextChunk"], 0, maxProtocolChunk) || !isSHA256(stringValue(body["sha256"])) {
			return ErrInvalidAttachment
		}
	case "error":
		if !exactKeys(body, "code", "retryable") || !knownErrorCode(stringValue(body["code"])) || boolValue(body["retryable"]) == nil {
			return ErrInvalidEnvelope
		}
	default:
		return ErrInvalidEnvelope
	}
	return nil
}

func validateResumeUploads(raw json.RawMessage) bool {
	if raw == nil {
		return true
	}
	var uploads map[string]json.RawMessage
	if json.Unmarshal(raw, &uploads) != nil || len(uploads) > MaxDeviceUploads {
		return false
	}
	for uploadID, rawNextChunk := range uploads {
		value, okay := uintValueOK(rawNextChunk)
		if !validID(uploadID) || !okay || value > maxProtocolChunk {
			return false
		}
	}
	return true
}

func validateStringArray(raw json.RawMessage) bool {
	var values []string
	if json.Unmarshal(raw, &values) != nil {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validateLimits(raw json.RawMessage) bool {
	var limits map[string]json.RawMessage
	if json.Unmarshal(raw, &limits) != nil || !exactKeys(limits,
		"maxJsonBytes", "maxAttachmentBytes", "maxDeviceUploads", "maxGlobalUploads", "maxTemporaryBytes", "uploadExpirySeconds") {
		return false
	}
	return uintInRange(limits["maxJsonBytes"], 1, MaxJSONFrameBytes) && uintInRange(limits["maxAttachmentBytes"], 1, MaxAttachmentBytes) &&
		uintInRange(limits["maxDeviceUploads"], 1, MaxDeviceUploads) && uintInRange(limits["maxGlobalUploads"], 1, MaxGlobalUploads) &&
		uintInRange(limits["maxTemporaryBytes"], 1, MaxTemporaryBytes) && uintInRange(limits["uploadExpirySeconds"], 1, UploadExpirySeconds)
}

func validateTasks(raw json.RawMessage) bool {
	var tasks []map[string]json.RawMessage
	if json.Unmarshal(raw, &tasks) != nil {
		return false
	}
	for _, task := range tasks {
		if !onlyAllowedKeys(task, "taskId", "title", "projectLabel", "state", "lastActivityAt", "pendingRequest") ||
			!validID(stringValue(task["taskId"])) || !boundedString(task["title"], 256) || !boundedString(task["projectLabel"], 128) ||
			!knownTaskState(stringValue(task["state"])) || !validRFC3339(stringValue(task["lastActivityAt"])) {
			return false
		}
		if pending := task["pendingRequest"]; pending != nil {
			var request map[string]json.RawMessage
			if json.Unmarshal(pending, &request) != nil || !exactKeys(request, "requestId", "kind", "summary") ||
				!validID(stringValue(request["requestId"])) || !knownRequestKind(stringValue(request["kind"])) || !boundedString(request["summary"], 512) {
				return false
			}
		}
	}
	return true
}

func validateProjects(raw json.RawMessage) bool {
	var projects []map[string]json.RawMessage
	if json.Unmarshal(raw, &projects) != nil || len(projects) > approvedprojects.MaxChoices {
		return false
	}
	seen := make(map[string]struct{}, len(projects))
	for _, project := range projects {
		id := stringValue(project["id"])
		if !exactKeys(project, "id", "displayName") || !validProjectID(id) || !safeDisplayString(project["displayName"], 128) {
			return false
		}
		if _, duplicate := seen[id]; duplicate {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func validateOptionalError(raw json.RawMessage, required bool) bool {
	if raw == nil {
		return !required
	}
	var value map[string]json.RawMessage
	return json.Unmarshal(raw, &value) == nil && exactKeys(value, "code", "retryable") &&
		knownErrorCode(stringValue(value["code"])) && boolValue(value["retryable"]) != nil
}

func knownTaskState(state string) bool {
	switch state {
	case "working", "waiting_for_approval", "waiting_for_answer", "failed", "interrupted", "idle_after_reply":
		return true
	default:
		return false
	}
}

func knownErrorCode(code string) bool {
	switch code {
	case "computer_offline", "connection_lost", "desktop_incompatible", "owner_unavailable", "invalid_action", "outcome_unknown", "sequence_gap", "unauthorized", "quota_exceeded", "attachment_invalid", "internal":
		return true
	default:
		return false
	}
}

func knownActionState(state string) bool {
	switch state {
	case "queued", "sent", "confirmed", "outcome_unknown", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func validActionTransition(previous, next string) bool {
	if !knownActionState(next) {
		return false
	}
	switch previous {
	case "":
		return true
	case "queued":
		return next == "sent" || next == "confirmed" || next == "outcome_unknown" || next == "failed" || next == "cancelled"
	case "sent":
		return next == "confirmed" || next == "outcome_unknown" || next == "failed" || next == "cancelled"
	default:
		return false
	}
}

func validateAction(sender string, body map[string]json.RawMessage) error {
	if sender != "phone" || !validID(stringValue(body["actionId"])) {
		return ErrInvalidAction
	}
	kind := stringValue(body["kind"])
	switch kind {
	case "start_turn", "steer_turn":
		if !onlyAllowedKeys(body, "actionId", "kind", "taskId", "text", "attachmentIds") ||
			!validID(stringValue(body["taskId"])) || !boundedString(body["text"], 131072) || strings.TrimSpace(stringValue(body["text"])) == "" ||
			!validateOptionalIDs(body["attachmentIds"]) {
			return ErrInvalidAction
		}
	case "interrupt_turn":
		if !exactKeys(body, "actionId", "kind", "taskId") || !validID(stringValue(body["taskId"])) {
			return ErrInvalidAction
		}
	case "approval":
		decision := stringValue(body["decision"])
		requestKind := stringValue(body["requestKind"])
		if !exactKeys(body, "actionId", "kind", "taskId", "requestId", "requestKind", "decision") ||
			!validID(stringValue(body["taskId"])) || !validID(stringValue(body["requestId"])) ||
			(requestKind != "command" && requestKind != "file" && requestKind != "permissions") ||
			(decision != "accept" && decision != "accept_for_session" && decision != "decline" && decision != "cancel") {
			return ErrInvalidAction
		}
	case "set_project":
		if !exactKeys(body, "actionId", "kind", "projectId") || !validProjectID(stringValue(body["projectId"])) {
			return ErrInvalidAction
		}
	default:
		return ErrInvalidAction
	}
	return nil
}

func validID(value string) bool {
	return utf8.RuneCountInString(value) >= 1 && utf8.RuneCountInString(value) <= 128 && identifierPattern.MatchString(value)
}

func validProjectID(value string) bool {
	return projectIdentifierPattern.MatchString(value)
}

func boundedString(raw json.RawMessage, maximum int) bool {
	value := stringValue(raw)
	length := utf8.RuneCountInString(value)
	return length >= 1 && length <= maximum
}

func safeDisplayString(raw json.RawMessage, maximum int) bool {
	value := stringValue(raw)
	length := utf8.RuneCountInString(value)
	return length >= 1 && length <= maximum && strings.TrimSpace(value) != "" && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validateOptionalIDs(raw json.RawMessage) bool {
	if raw == nil {
		return true
	}
	var values []string
	if json.Unmarshal(raw, &values) != nil {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validID(value) {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func uintValueOK(raw json.RawMessage) (uint64, bool) {
	if raw == nil {
		return 0, false
	}
	var value uint64
	if json.Unmarshal(raw, &value) != nil {
		return 0, false
	}
	return value, true
}

func uintInRange(raw json.RawMessage, minimum, maximum int) bool {
	value, okay := uintValueOK(raw)
	return okay && value >= uint64(minimum) && value <= uint64(maximum)
}

func uintInRange64(raw json.RawMessage, minimum, maximum uint64) bool {
	value, okay := uintValueOK(raw)
	return okay && value >= minimum && value <= maximum
}

func uniquePositiveInts(values []int) bool {
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value < 1 || uint64(value) > maxProtocolChunk {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validRFC3339(value string) bool {
	normalized := value
	if len(normalized) > 10 && normalized[10] == 't' {
		normalized = normalized[:10] + "T" + normalized[11:]
	}
	if strings.HasSuffix(normalized, "z") {
		normalized = normalized[:len(normalized)-1] + "Z"
	}
	_, err := time.Parse(time.RFC3339, normalized)
	return err == nil
}

func knownEvent(event string) bool {
	switch event {
	case "activity", "reply", "approval", "answer", "failure", "interrupted", "metadata":
		return true
	default:
		return false
	}
}

func knownRequestKind(kind string) bool {
	switch kind {
	case "command", "file", "permissions", "question", "mcp_elicitation":
		return true
	default:
		return false
	}
}

func stringValue(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func uintValue(raw json.RawMessage) uint64 {
	var value uint64
	_ = json.Unmarshal(raw, &value)
	return value
}

func boolValue(raw json.RawMessage) *bool {
	var value bool
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return &value
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func isHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}

func isSHA256(value string) bool { return len(value) == 64 && isHex(value) }

func exactKeys(values map[string]json.RawMessage, keys ...string) bool {
	return len(values) == len(keys) && onlyAllowedKeys(values, keys...)
}

func onlyAllowedKeys(values map[string]json.RawMessage, keys ...string) bool {
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	for key := range values {
		if _, ok := allowed[key]; !ok {
			return false
		}
	}
	return true
}
