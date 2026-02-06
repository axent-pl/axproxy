package module

import (
	"net/http"

	s "github.com/axent-pl/axproxy/state"
)

type ProxyHandlerFunc func(w http.ResponseWriter, r *http.Request, st *s.State)
type ProxyDirectorHandlerFunc func(*http.Request, *s.State)

type ProxyModifyResponseHandlerFunc func(*http.Response, *s.State) error
type ProxyErrorHandlerFunc func(w http.ResponseWriter, r *http.Request, err error)

type Module interface {
	Kind() string

	Name() string

	Middleware(next http.HandlerFunc) http.HandlerFunc

	SpecialRoutes() map[string]http.HandlerFunc

	ProxyMiddleware(ProxyHandlerFunc) ProxyHandlerFunc

	ProxyDirectorMiddleware(ProxyDirectorHandlerFunc) ProxyDirectorHandlerFunc

	ProxyModifyResponseMiddleware(ProxyModifyResponseHandlerFunc) ProxyModifyResponseHandlerFunc
}

type NoopModule struct{}

func (NoopModule) SpecialRoutes() map[string]http.HandlerFunc        { return nil }
func (NoopModule) Middleware(next http.HandlerFunc) http.HandlerFunc { return nil }
func (NoopModule) ProxyMiddleware(ProxyHandlerFunc) ProxyHandlerFunc { return nil }
func (NoopModule) ProxyDirectorMiddleware(ProxyDirectorHandlerFunc) ProxyDirectorHandlerFunc {
	return nil
}

func (NoopModule) ProxyModifyResponseMiddleware(ProxyModifyResponseHandlerFunc) ProxyModifyResponseHandlerFunc {
	return nil
}
