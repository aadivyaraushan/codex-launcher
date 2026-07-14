package testkit

import "context"

type Call struct {
	Name string
	Args []string
}

type RecordingRunner struct {
	Calls  []Call
	Output string
	Err    error
}

func (runner *RecordingRunner) Run(_ context.Context, name string, arguments ...string) (string, error) {
	copyOfArguments := append([]string(nil), arguments...)
	runner.Calls = append(runner.Calls, Call{Name: name, Args: copyOfArguments})
	return runner.Output, runner.Err
}
