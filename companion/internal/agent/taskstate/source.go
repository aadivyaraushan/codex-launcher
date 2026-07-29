package taskstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
)

var (
	ErrAdapterSourceMismatch = errors.New("Codex adapter source does not match its route")
	ErrUnresolvedTaskSource  = errors.New("Codex task owner is not verified")
	ErrUnknownTaskSource     = errors.New("Codex task source is unknown")
)

type TaskAdapter interface {
	TaskSource() Source
}

type AdapterRouter struct {
	desktop   TaskAdapter
	appServer TaskAdapter
}

func NewAdapterRouter(desktop, appServer TaskAdapter) (AdapterRouter, error) {
	if desktop == nil || desktop.TaskSource() != SourceDesktop || appServer == nil || appServer.TaskSource() != SourceAppServer {
		return AdapterRouter{}, ErrAdapterSourceMismatch
	}
	return AdapterRouter{desktop: desktop, appServer: appServer}, nil
}

func (router AdapterRouter) For(task Task) (TaskAdapter, error) {
	switch task.Source {
	case SourceDesktop:
		return router.desktop, nil
	case SourceAppServer:
		return router.appServer, nil
	case SourceCatalog:
		return nil, ErrUnresolvedTaskSource
	default:
		return nil, ErrUnknownTaskSource
	}
}

func PermissionSubset(granted, requested json.RawMessage) bool {
	decode := func(raw json.RawMessage) (any, bool) {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		return value, true
	}
	grantedValue, grantedOK := decode(granted)
	requestedValue, requestedOK := decode(requested)
	if !grantedOK || !requestedOK {
		return false
	}
	var subset func(any, any) bool
	subset = func(candidate, maximum any) bool {
		switch value := candidate.(type) {
		case map[string]any:
			container, ok := maximum.(map[string]any)
			if !ok {
				return false
			}
			for key, child := range value {
				allowed, exists := container[key]
				if !exists || !subset(child, allowed) {
					return false
				}
			}
			return true
		case []any:
			container, ok := maximum.([]any)
			if !ok {
				return false
			}
			for _, child := range value {
				found := false
				for _, allowed := range container {
					found = found || subset(child, allowed)
				}
				if !found {
					return false
				}
			}
			return true
		default:
			return reflect.DeepEqual(candidate, maximum)
		}
	}
	return subset(grantedValue, requestedValue)
}
