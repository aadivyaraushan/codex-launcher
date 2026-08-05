package phoneruntime

// 1) Callers: runtime.go Open (~loadBrokerReady), Health overlay (brokerReady),
//    Serve POST /v1/credentials/broker-status, broker_status_test.go.
// 2) Grep: no broker_status.go / MarkBrokerReady before this file; only
//    android_broker_pending string remap in Health().
// 3) Data file <root>/broker-status.json example: {"todoist":"ready"} (string map).
// 4) User: "Fix android_broker_pending on phone-runtime health despite Todoist
//    token in credential_broker — bridge/import so health reflects connected Todoist."

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const brokerStatusFile = "broker-status.json"

var (
	ErrBrokerStatusInvalid = errors.New("broker status invalid")
	ErrBrokerStatusSecret  = errors.New("broker status must not include secrets")
)

type brokerStatusRequest struct {
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

func loadBrokerReady(root string) map[string]string {
	path := filepath.Join(root, brokerStatusFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]string{}
	}
	cleaned := map[string]string{}
	for k, v := range out {
		if v == "ready" && k != "" {
			cleaned[k] = "ready"
		}
	}
	return cleaned
}

func (runtime *Runtime) persistBrokerReadyLocked() error {
	if runtime == nil {
		return ErrMissingDependency
	}
	path := filepath.Join(runtime.config.Root, brokerStatusFile)
	raw, err := json.MarshalIndent(runtime.brokerReady, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// MarkBrokerReady records that Android holds a usable grant for provider.
// Never accepts or stores tokens.
func (runtime *Runtime) MarkBrokerReady(provider string) error {
	if runtime == nil {
		return ErrMissingDependency
	}
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return fmt.Errorf("%w: empty provider", ErrBrokerStatusInvalid)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.brokerReady == nil {
		runtime.brokerReady = map[string]string{}
	}
	runtime.brokerReady[provider] = "ready"
	if err := runtime.persistBrokerReadyLocked(); err != nil {
		return err
	}
	runtime.logger.Info("[phone-runtime] broker status ready", "provider", provider)
	return nil
}

func (runtime *Runtime) applyBrokerStatusRequest(raw map[string]json.RawMessage) error {
	if raw == nil {
		return ErrBrokerStatusInvalid
	}
	for key := range raw {
		switch strings.ToLower(key) {
		case "provider", "status":
			continue
		default:
			return fmt.Errorf("%w: unexpected field %q", ErrBrokerStatusSecret, key)
		}
	}
	var req brokerStatusRequest
	b, err := json.Marshal(raw)
	if err != nil {
		return ErrBrokerStatusInvalid
	}
	if err := json.Unmarshal(b, &req); err != nil {
		return ErrBrokerStatusInvalid
	}
	if strings.TrimSpace(strings.ToLower(req.Status)) != "ready" {
		return fmt.Errorf("%w: status must be ready", ErrBrokerStatusInvalid)
	}
	return runtime.MarkBrokerReady(req.Provider)
}
