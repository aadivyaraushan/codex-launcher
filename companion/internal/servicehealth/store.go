package servicehealth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service"
)

const maxHealthBytes int64 = 4096

var (
	ErrHealthMissing = errors.New("companion health record is missing")
	ErrInvalidHealth = errors.New("companion health record is invalid")
	ErrStartFailed   = errors.New("companion service reported a failed start")
	ErrStartTimeout  = errors.New("companion service did not confirm this start attempt")
)

type State string

const (
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopped  State = "stopped"
	StateFailed   State = "failed"
)

const (
	ErrorCodexUnavailable           = "codex_unavailable"
	ErrorServiceDependencies        = "service_dependencies_unavailable"
	ErrorStateUnavailable           = "state_unavailable"
	ErrorTLSIdentityUnavailable     = "tls_identity_unavailable"
	ErrorListenUnavailable          = "listen_unavailable"
	ErrorServiceStoppedUnexpectedly = "service_stopped_unexpectedly"
)

type Record struct {
	Version   int       `json:"version"`
	AttemptID string    `json:"attemptId,omitempty"`
	State     State     `json:"state"`
	LastError string    `json:"lastError,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Store struct {
	path         string
	now          func() time.Time
	random       io.Reader
	waitTimeout  time.Duration
	pollInterval time.Duration
}

func New(path string, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{path: path, now: now, random: rand.Reader, waitTimeout: 15 * time.Second, pollInterval: 50 * time.Millisecond}
}

func (store *Store) Update(state State, errorCode string) error {
	attemptID := ""
	if previous, err := store.Read(); err == nil {
		attemptID = previous.AttemptID
	} else if !errors.Is(err, ErrHealthMissing) {
		return err
	}
	return store.UpdateAttempt(attemptID, state, errorCode)
}

func (store *Store) UpdateAttempt(attemptID string, state State, errorCode string) error {
	if !validAttempt(attemptID, true) || !validState(state) || (errorCode != "" && !validError(errorCode)) || store.path == "" {
		return ErrInvalidHealth
	}
	lastError := ""
	if previous, err := store.Read(); err == nil {
		lastError = previous.LastError
	} else if !errors.Is(err, ErrHealthMissing) {
		return err
	}
	if errorCode != "" {
		lastError = errorCode
	}
	record := Record{Version: 1, AttemptID: attemptID, State: state, LastError: lastError, UpdatedAt: store.now().UTC()}
	encoded, err := json.Marshal(record)
	if err != nil || int64(len(encoded)) > maxHealthBytes {
		return ErrInvalidHealth
	}
	encoded = append(encoded, '\n')
	if err := service.WriteFileAtomic(store.path, encoded, 0o600); err != nil {
		return err
	}
	return nil
}

func (store *Store) PrepareStart() (string, error) {
	bytes := make([]byte, 16)
	if store.path == "" || store.random == nil {
		return "", ErrInvalidHealth
	}
	if _, err := io.ReadFull(store.random, bytes); err != nil {
		return "", fmt.Errorf("create service start attempt: %w", err)
	}
	attemptID := fmt.Sprintf("%x", bytes)
	request := struct {
		Version   int    `json:"version"`
		AttemptID string `json:"attemptId"`
	}{Version: 1, AttemptID: attemptID}
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", ErrInvalidHealth
	}
	if err := service.WriteFileAtomic(store.requestPath(), append(encoded, '\n'), 0o600); err != nil {
		return "", err
	}
	return attemptID, nil
}

func (store *Store) BeginAttempt() (string, error) {
	encoded, err := os.ReadFile(store.requestPath())
	if errors.Is(err, os.ErrNotExist) {
		if _, err := store.PrepareStart(); err != nil {
			return "", err
		}
		encoded, err = os.ReadFile(store.requestPath())
	}
	if err != nil || len(encoded) > 1024 {
		return "", ErrInvalidHealth
	}
	request := struct {
		Version   int    `json:"version"`
		AttemptID string `json:"attemptId"`
	}{}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.Version != 1 || !validAttempt(request.AttemptID, false) {
		return "", ErrInvalidHealth
	}
	return request.AttemptID, nil
}

func (store *Store) WaitRunning(ctx context.Context, attemptID string) error {
	if !validAttempt(attemptID, false) {
		return ErrInvalidHealth
	}
	timer := time.NewTimer(store.waitTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(store.pollInterval)
	defer ticker.Stop()
	for {
		record, err := store.Read()
		if err == nil && record.AttemptID == attemptID {
			switch record.State {
			case StateRunning:
				return nil
			case StateFailed:
				return fmt.Errorf("%w: %s", ErrStartFailed, record.LastError)
			}
		} else if err != nil && !errors.Is(err, ErrHealthMissing) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return ErrStartTimeout
		case <-ticker.C:
		}
	}
}

func (store *Store) Read() (Record, error) {
	info, err := os.Lstat(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, ErrHealthMissing
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxHealthBytes {
		return Record{}, ErrInvalidHealth
	}
	file, err := os.Open(store.path)
	if err != nil {
		return Record{}, ErrInvalidHealth
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, maxHealthBytes+1))
	if err != nil || int64(len(encoded)) > maxHealthBytes {
		return Record{}, ErrInvalidHealth
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil || record.Version != 1 || !validAttempt(record.AttemptID, true) || !validState(record.State) || (record.LastError != "" && !validError(record.LastError)) || record.UpdatedAt.IsZero() {
		return Record{}, ErrInvalidHealth
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Record{}, ErrInvalidHealth
	}
	return record, nil
}

func (store *Store) requestPath() string { return store.path + ".start-request" }

func validAttempt(attemptID string, allowEmpty bool) bool {
	if attemptID == "" {
		return allowEmpty
	}
	if len(attemptID) != 32 {
		return false
	}
	for _, character := range attemptID {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func (store *Store) LastError() (string, error) {
	record, err := store.Read()
	if errors.Is(err, ErrHealthMissing) {
		return "none recorded", nil
	}
	if err != nil {
		return "", err
	}
	if record.LastError == "" {
		return "none recorded", nil
	}
	return record.LastError, nil
}

func validState(state State) bool {
	switch state {
	case StateStarting, StateRunning, StateStopped, StateFailed:
		return true
	default:
		return false
	}
}

func validError(code string) bool {
	switch code {
	case ErrorCodexUnavailable, ErrorServiceDependencies, ErrorStateUnavailable, ErrorTLSIdentityUnavailable, ErrorListenUnavailable, ErrorServiceStoppedUnexpectedly:
		return true
	default:
		return false
	}
}
