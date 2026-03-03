package session

import "sync"

type RuntimeSnapshot struct {
	State     RuntimeState `json:"state"`
	RunningAt int64        `json:"runningAt,omitempty"`
}

type RuntimeStore struct {
	mu    sync.RWMutex
	items map[string]RuntimeSnapshot
}

func NewRuntimeStore() *RuntimeStore {
	return &RuntimeStore{
		items: make(map[string]RuntimeSnapshot),
	}
}

func (s *RuntimeStore) Get(sessionID string) RuntimeSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if snap, ok := s.items[sessionID]; ok {
		return snap
	}
	return RuntimeSnapshot{State: RuntimeStateInactive}
}

func (s *RuntimeStore) Set(sessionID string, snap RuntimeSnapshot) RuntimeSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[sessionID] = normalizeRuntimeSnapshot(snap)
	return s.items[sessionID]
}

func (s *RuntimeStore) Transition(sessionID string, action RuntimeTransition, runningAt int64) (RuntimeSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := RuntimeSnapshot{State: RuntimeStateInactive}
	if snap, ok := s.items[sessionID]; ok {
		current = snap
	}

	nextState, err := NextRuntimeState(current.State, action)
	if err != nil {
		return current, err
	}

	next := current
	next.State = nextState
	if nextState == RuntimeStateRunning {
		next.RunningAt = runningAt
	} else {
		next.RunningAt = 0
	}
	next = normalizeRuntimeSnapshot(next)
	s.items[sessionID] = next
	return next, nil
}

func normalizeRuntimeSnapshot(snap RuntimeSnapshot) RuntimeSnapshot {
	switch snap.State {
	case RuntimeStateInactive, RuntimeStateReady, RuntimeStateRunning:
	default:
		snap.State = RuntimeStateInactive
	}
	if snap.State != RuntimeStateRunning {
		snap.RunningAt = 0
	}
	return snap
}
