package state

import (
	"context"
	"crypto/rand"
	"encoding/base64"
)

type ctxKey int

const stateKey ctxKey = 1

type State struct {
	RequestID string
	Values    map[string]any
	Session   *Session
	Error     error
}

func NewState() *State {
	return &State{
		RequestID: newRequestID(),
		Values:    map[string]any{},
	}
}

func (s *State) Set(key string, v any) { s.Values[key] = v }
func (s *State) Get(key string) (any, bool) {
	v, ok := s.Values[key]
	return v, ok
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

func newRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
