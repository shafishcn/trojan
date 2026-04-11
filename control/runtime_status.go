package control

import "time"

type runtimeReporter interface {
	RuntimeStatus() (*RuntimeStatus, error)
}

func controlRuntimeStatus(store Store) (*RuntimeStatus, error) {
	if reporter, ok := store.(runtimeReporter); ok {
		return reporter.RuntimeStatus()
	}
	return &RuntimeStatus{
		Backend:   "unknown",
		Healthy:   true,
		CheckedAt: time.Now().UTC(),
	}, nil
}
