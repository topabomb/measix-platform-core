package control

import (
	"context"
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
	requests  map[*activeRequest]struct{}
}

type activeRequest struct {
	valid  func(*State) bool
	cancel context.CancelFunc
}

// Register and Apply use one lock: registration after a completed revocation
// must recheck the new authority and cannot escape the cancellation sweep.
func (s *Store) Register(parent context.Context, valid func(*State) bool) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.current.Load()
	if state == nil || !valid(state) {
		return nil, nil, ErrInvalidControl
	}
	ctx, cancel := context.WithCancel(parent)
	entry := &activeRequest{valid: valid, cancel: cancel}
	if s.requests == nil {
		s.requests = make(map[*activeRequest]struct{})
	}
	s.requests[entry] = struct{}{}
	return ctx, func() { s.mu.Lock(); delete(s.requests, entry); s.mu.Unlock(); cancel() }, nil
}
func (s *Store) cancelInvalid(state *State) {
	for entry := range s.requests {
		if !entry.valid(state) {
			entry.cancel()
			delete(s.requests, entry)
		}
	}
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
			s.cancelInvalid(current)
			return ack(current), nil
		}
	}

	state, err := build(input, s.Now())
	if err != nil {
		return relaycontrolapi.ControlAck{}, err
	}
	s.current.Store(state)
	s.cancelInvalid(state)
	return ack(state), nil
}

func (s *Store) Status() relaycontrolapi.ControlStatus {
	current := s.current.Load()
	if current == nil {
		return relaycontrolapi.ControlStatus{
			ProtocolVersion: protocolStatusVersion(),
			Ready:           false, AppliedControlRevision: 0, BundleHash: "", ActiveManagedGeneration: 0, StartedAt: s.startedAt,
		}
	}
	return relaycontrolapi.ControlStatus{
		ProtocolVersion: protocolStatusVersion(),
		Ready:           true, AppliedControlRevision: current.ControlRevision, BundleHash: current.BundleHash,
		ActiveManagedGeneration: current.ActiveManagedGeneration, StartedAt: s.startedAt,
	}
}

func IsRevisionHashConflict(err error) bool { return errors.Is(err, ErrRevisionHashConflict) }

func IsRevisionStale(err error) bool { return errors.Is(err, ErrStaleRevision) }

func ack(state *State) relaycontrolapi.ControlAck {
	var protocol *relaycontrolapi.ControlAckProtocolVersion
	if state.ProtocolVersion == 2 {
		protocol = protocolAckVersion()
	}
	return relaycontrolapi.ControlAck{
		ProtocolVersion:         protocol,
		AppliedControlRevision:  state.ControlRevision,
		BundleHash:              state.BundleHash,
		ActiveManagedGeneration: state.ActiveManagedGeneration,
		AppliedAt:               state.AppliedAt,
	}
}

func protocolStatusVersion() *relaycontrolapi.ControlStatusProtocolVersion {
	v := relaycontrolapi.ControlStatusProtocolVersion(2)
	return &v
}
func protocolAckVersion() *relaycontrolapi.ControlAckProtocolVersion {
	v := relaycontrolapi.ControlAckProtocolVersion(2)
	return &v
}
