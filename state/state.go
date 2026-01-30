package state

import (
	"context"
	"time"
)

type ctxKey int

const stateKey ctxKey = 1

type State struct {
	Values  map[string]any
	Session *Session
	Stop    bool
	Error   error
}

func NewState() *State {
	return &State{
		Values: map[string]any{},
		Stop:   false,
	}
}

func (s *State) Set(key string, v any) { s.Values[key] = v }
func (s *State) Get(key string) (any, bool) {
	v, ok := s.Values[key]
	return v, ok
}

type Session struct {
	ID        string
	Values    map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt *time.Time
}

func NewSession(id string, maxAgeSeconds int) *Session {
	now := time.Now().UTC()
	sess := &Session{
		ID:        id,
		Values:    map[string]any{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if maxAgeSeconds > 0 {
		exp := now.Add(time.Duration(maxAgeSeconds) * time.Second)
		sess.ExpiresAt = &exp
	}
	return sess
}

func (s *Session) IsExpired() bool {
	if s == nil || s.ExpiresAt == nil {
		return false
	}
	return time.Now().UTC().After(*s.ExpiresAt)
}

func WithState(ctx context.Context, st *State) context.Context {
	return context.WithValue(ctx, stateKey, st)
}

func GetState(ctx context.Context) *State {
	if v := ctx.Value(stateKey); v != nil {
		if st, ok := v.(*State); ok {
			return st
		}
	}
	return nil
}
