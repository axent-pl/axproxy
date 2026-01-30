package modules

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"github.com/axent-pl/axproxy/state"
)

const KIND_CUSTOMHEADERS string = "CustomHeaders"

type CustomHeadersModule struct {
	module.NoopModule
	Metadata        manifest.ObjectMeta     `yaml:"metadata"`
	RequestHeaders  []CustomHeaderOpertaion `yaml:"request"`
	ResponseHeaders []CustomHeaderOpertaion `yaml:"response"`
}

type CustomHeaderOpertaion struct {
	Operation string `yaml:"op"`
	Header    string `yaml:"header"`
	Value     string `yaml:"value"`
}

func (m *CustomHeadersModule) Kind() string {
	return KIND_CUSTOMHEADERS
}

func (m *CustomHeadersModule) Name() string {
	return m.Metadata.Name
}

func (m *CustomHeadersModule) ProxyModifyResponseMiddleware(next module.ProxyModifyResponseHandlerFunc) module.ProxyModifyResponseHandlerFunc {
	return module.ProxyModifyResponseHandlerFunc(func(resp *http.Response, st *state.State) error {
		for _, ho := range m.ResponseHeaders {
			switch ho.Operation {
			case "set":
				resp.Header.Set(ho.Header, ho.Value)
			case "del":
				resp.Header.Del(ho.Header)
			default:
				err := fmt.Errorf("undefined response header operation, want: get|set, got %s", ho.Operation)
				slog.Error("failed to set response headers", "error", err)
				return err
			}
		}
		return next(resp, st)
	})
}

func (m *CustomHeadersModule) ProxyDirectorMiddleware(next module.ProxyDirectorHandlerFunc) module.ProxyDirectorHandlerFunc {
	return module.ProxyDirectorHandlerFunc(func(r *http.Request, st *state.State) {
		for _, ho := range m.RequestHeaders {
			switch ho.Operation {
			case "set":
				r.Header.Set(ho.Header, ho.Value)
			case "del":
				r.Header.Del(ho.Header)
			default:
				err := fmt.Errorf("undefined request header operation, want: get|set, got %s", ho.Operation)
				slog.Error("failed to set request headers", "error", err)
			}
		}
		next(r, st)
	})
}
