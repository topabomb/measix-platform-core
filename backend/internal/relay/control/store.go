package control

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"measix/platform/internal/wire/relaycontrolapi"
)

var (
	ErrInvalidControl       = errors.New("invalid runtime control")
	ErrStaleRevision        = errors.New("stale control revision")
	ErrRevisionHashConflict = errors.New("control revision hash conflict")
)

type Store struct {
	current   atomic.Pointer[State]
	mu        sync.Mutex
	now       func() time.Time
	startedAt time.Time
}

func NewStore(now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{now: now, startedAt: now().UTC()}
}

func (s *Store) Now() time.Time { return s.now().UTC() }

func (s *Store) Current() *State { return s.current.Load() }

func (s *Store) Apply(input relaycontrolapi.RuntimeControlState) (relaycontrolapi.ControlAck, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if current := s.current.Load(); current != nil {
		switch {
		case input.ControlRevision < current.ControlRevision:
			return relaycontrolapi.ControlAck{}, ErrStaleRevision
		case input.ControlRevision == current.ControlRevision && string(input.BundleHash) != current.BundleHash:
			return relaycontrolapi.ControlAck{}, ErrRevisionHashConflict
		case input.ControlRevision == current.ControlRevision:
			return ack(current), nil
		}
	}

	state, err := build(input, s.Now())
	if err != nil {
		return relaycontrolapi.ControlAck{}, err
	}
	s.current.Store(state)
	return ack(state), nil
}

func (s *Store) Status() relaycontrolapi.ControlStatus {
	current := s.current.Load()
	if current == nil {
		return relaycontrolapi.ControlStatus{
			Ready: false, AppliedControlRevision: 0, BundleHash: "", ActiveManagedGeneration: 0, StartedAt: s.startedAt,
		}
	}
	return relaycontrolapi.ControlStatus{
		Ready: true, AppliedControlRevision: current.ControlRevision, BundleHash: current.BundleHash,
		ActiveManagedGeneration: current.ActiveManagedGeneration, StartedAt: s.startedAt,
	}
}

func IsRevisionHashConflict(err error) bool { return errors.Is(err, ErrRevisionHashConflict) }

func IsRevisionStale(err error) bool { return errors.Is(err, ErrStaleRevision) }

func ack(state *State) relaycontrolapi.ControlAck {
	return relaycontrolapi.ControlAck{
		AppliedControlRevision:  state.ControlRevision,
		BundleHash:              state.BundleHash,
		ActiveManagedGeneration: state.ActiveManagedGeneration,
		AppliedAt:               state.AppliedAt,
	}
}
