package transaction

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/configsecurity"
)

const maxRecordBytes int64 = 1024

var (
	ErrMissing = errors.New("install transaction is missing")
	ErrInvalid = errors.New("install transaction is invalid")
)

const (
	OperationInstall     = "install"
	OperationReplace     = "replace"
	OperationRollback    = "rollback"
	OperationUninstall   = "uninstall"
	OperationReconfigure = "reconfigure"

	PhasePrepared  = "prepared"
	PhaseActivated = "activated"
	PhaseStopped   = "stopped"
	PhaseHealthy   = "healthy"
)

type Record struct {
	Version   int    `json:"version"`
	Operation string `json:"operation"`
	Phase     string `json:"phase"`
}

type Store struct {
	path string
	root string
}

func New(path string) *Store { return &Store{path: path, root: filepath.Dir(path)} }

func (store *Store) Write(record Record) error {
	if !valid(record) {
		return ErrInvalid
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return ErrInvalid
	}
	if err := configsecurity.Replace(store.root, store.path, append(encoded, '\n'), maxRecordBytes); err != nil {
		return err
	}
	return nil
}

func (store *Store) Read() (Record, error) {
	file, err := configsecurity.Open(store.root, store.path, maxRecordBytes)
	if errors.Is(err, configsecurity.ErrUnsafe) {
		if _, statErr := os.Lstat(store.path); errors.Is(statErr, os.ErrNotExist) {
			return Record{}, ErrMissing
		}
	}
	if err != nil {
		return Record{}, ErrInvalid
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, maxRecordBytes+1))
	if err != nil || int64(len(encoded)) > maxRecordBytes {
		return Record{}, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil || !valid(record) {
		return Record{}, ErrInvalid
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Record{}, ErrInvalid
	}
	return record, nil
}

func (store *Store) Clear() error {
	err := os.Remove(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func valid(record Record) bool {
	if record.Version != 1 {
		return false
	}
	switch record.Operation {
	case OperationInstall:
		return record.Phase == PhasePrepared || record.Phase == PhaseActivated || record.Phase == PhaseHealthy
	case OperationReplace, OperationRollback:
		return record.Phase == PhasePrepared || record.Phase == PhaseActivated || record.Phase == PhaseHealthy
	case OperationUninstall:
		return record.Phase == PhasePrepared || record.Phase == PhaseStopped
	case OperationReconfigure:
		return record.Phase == PhasePrepared || record.Phase == PhaseStopped || record.Phase == PhaseHealthy
	default:
		return false
	}
}
