package session

import (
	"errors"
	"fmt"
)

type RuntimeState string

const (
	RuntimeStateInactive RuntimeState = "INACTIVE"
	RuntimeStateReady    RuntimeState = "READY"
	RuntimeStateRunning  RuntimeState = "RUNNING"
)

type RuntimeTransition string

const (
	RuntimeTransitionSend     RuntimeTransition = "send"
	RuntimeTransitionComplete RuntimeTransition = "complete"
	RuntimeTransitionArchive  RuntimeTransition = "archive"
)

var ErrInvalidRuntimeTransition = errors.New("invalid runtime transition")

func NextRuntimeState(current RuntimeState, action RuntimeTransition) (RuntimeState, error) {
	switch action {
	case RuntimeTransitionSend:
		if current == RuntimeStateReady {
			return RuntimeStateRunning, nil
		}
	case RuntimeTransitionComplete:
		if current == RuntimeStateRunning {
			return RuntimeStateReady, nil
		}
	case RuntimeTransitionArchive:
		if current == RuntimeStateReady || current == RuntimeStateRunning {
			return RuntimeStateInactive, nil
		}
	}
	return current, fmt.Errorf("%w: %s -> %s", ErrInvalidRuntimeTransition, current, action)
}
