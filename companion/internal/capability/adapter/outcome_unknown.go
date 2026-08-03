package adapter

import "fmt"

// OutcomeUnknownError marks a capability action where the request
// demonstrably left the machine but the reply demonstrably went missing, so
// the caller cannot say whether the action happened or not.
//
// It mirrors appserver.OutcomeUnknownError (internal/codex/appserver/client.go
// :131-139), which is this project's existing answer to the same question on
// the action path. Keeping the shape the same means there is one idea here,
// not two.
//
// Use it only when all three hold: the request demonstrably left the
// machine, the reply demonstrably went missing, and the operation changes
// something in the world. A read that fails, a non-2xx response, a decoded
// API-level error, missing credentials, or a request that failed to build
// are never unknown outcomes — they are ordinary failures.
type OutcomeUnknownError struct {
	AdapterID string
	Verb      string
	Cause     error
}

func (err *OutcomeUnknownError) Error() string {
	return fmt.Sprintf("%s %s outcome is unknown: %v", err.AdapterID, err.Verb, err.Cause)
}

func (err *OutcomeUnknownError) Unwrap() error { return err.Cause }
