package modules

import (
	"bufio"
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"github.com/axent-pl/axproxy/state"
	"github.com/axent-pl/axproxy/utils"
)

const KIND_AUDIT string = "Audit"

type AuditModule struct {
	module.NoopModule
	Metadata manifest.ObjectMeta `yaml:"metadata"`

	RequestIDHeader string               `yaml:"request_id_header"`
	MaxBodyBytes    int                  `yaml:"max_body_bytes"`
	Request         AuditRequestLogging  `yaml:"request"`
	Response        AuditResponseLogging `yaml:"response"`
}

type AuditRequestLogging struct {
	Info  AuditRequestFields `yaml:"info"`
	Debug AuditRequestFields `yaml:"debug"`
}

type AuditResponseLogging struct {
	Info  AuditResponseFields `yaml:"info"`
	Debug AuditResponseFields `yaml:"debug"`
}

type AuditRequestFields struct {
	Method     bool `yaml:"method"`
	Path       bool `yaml:"path"`
	Query      bool `yaml:"query"`
	Headers    bool `yaml:"headers"`
	Body       bool `yaml:"body"`
	Host       bool `yaml:"host"`
	Origin     bool `yaml:"origin"`
	RemoteAddr bool `yaml:"remote_addr"`
}

type AuditResponseFields struct {
	Status       bool `yaml:"status"`
	Headers      bool `yaml:"headers"`
	Body         bool `yaml:"body"`
	Size         bool `yaml:"size"`
	Duration     bool `yaml:"duration"`
	TargetOrigin bool `yaml:"target_origin"`
}

const (
	defaultRequestIDHeader = "X-Request-Id"
	defaultMaxBodyBytes    = 64 * 1024
	auditRequestIDKey      = "audit.request_id"
	auditTargetOriginKey   = "audit.target_origin"
)

func (m *AuditModule) Kind() string {
	return KIND_AUDIT
}

func (m *AuditModule) Name() string {
	return m.Metadata.Name
}

func (m *AuditModule) ProxyDirectorMiddleware(next module.ProxyDirectorHandlerFunc) module.ProxyDirectorHandlerFunc {
	return module.ProxyDirectorHandlerFunc(func(r *http.Request, st *state.State) {
		next(r, st)
		if r == nil || st == nil {
			return
		}
		if r.URL == nil {
			return
		}
		if r.URL.Scheme == "" || r.URL.Host == "" {
			return
		}
		st.Set(auditTargetOriginKey, r.URL.Scheme+"://"+r.URL.Host)
	})
}

func (m *AuditModule) ProxyModifyResponseMiddleware(next module.ProxyModifyResponseHandlerFunc) module.ProxyModifyResponseHandlerFunc {
	return module.ProxyModifyResponseHandlerFunc(func(resp *http.Response, st *state.State) error {
		if resp != nil && st != nil {
			if reqID, ok := st.Get(auditRequestIDKey); ok {
				if header := m.requestIDHeader(); header != "" {
					if id, ok := reqID.(string); ok && id != "" {
						resp.Header.Set(header, id)
					}
				}
			}
		}
		return next(resp, st)
	})
}

func (m *AuditModule) ProxyMiddleware(next module.ProxyHandlerFunc) module.ProxyHandlerFunc {
	return module.ProxyHandlerFunc(func(w http.ResponseWriter, r *http.Request, st *state.State) {
		if r == nil || st == nil {
			next(w, r, st)
			return
		}

		start := time.Now()
		requestID := st.RequestID
		st.Set(auditRequestIDKey, requestID)
		if header := m.requestIDHeader(); header != "" && requestID != "" {
			w.Header().Set(header, requestID)
		}

		reqBody, reqBodyTruncated, reqBodyLen := m.captureRequestBody(r)
		infoReqFields := m.requestInfoFields()
		debugReqFields := m.requestDebugFields()
		infoRespFields := m.responseInfoFields()
		debugRespFields := m.responseDebugFields()

		captureRespBody := infoRespFields.Body || debugRespFields.Body
		aw := newAuditResponseWriter(w, captureRespBody, m.maxBodyBytes())

		next(aw, r, st)

		duration := time.Since(start)
		attrs := m.buildAttrs(infoReqFields, infoRespFields, requestID, r, st, aw, duration, reqBody, reqBodyLen, reqBodyTruncated)
		if len(attrs) > 0 {
			slog.Info("AuditModule request completed", attrs...)
		}

		debugAttrs := m.buildAttrs(debugReqFields, debugRespFields, requestID, r, st, aw, duration, reqBody, reqBodyLen, reqBodyTruncated)
		if len(debugAttrs) > 0 {
			slog.Debug("AuditModule request completed", debugAttrs...)
		}

	})
}

func (m *AuditModule) requestIDHeader() string {
	if m.RequestIDHeader != "" {
		return m.RequestIDHeader
	}
	return defaultRequestIDHeader
}

func (m *AuditModule) maxBodyBytes() int {
	if m.MaxBodyBytes > 0 {
		return m.MaxBodyBytes
	}
	return defaultMaxBodyBytes
}

func (m *AuditModule) captureRequestBody(r *http.Request) (string, bool, int) {
	if r == nil || r.Body == nil || r.Body == http.NoBody {
		return "", false, 0
	}
	if !m.requestInfoFields().Body && !m.requestDebugFields().Body {
		return "", false, 0
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("audit: failed to read request body", "error", err)
		return "", false, 0
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	maxBytes := m.maxBodyBytes()
	if maxBytes > 0 && len(bodyBytes) > maxBytes {
		return string(bodyBytes[:maxBytes]), true, len(bodyBytes)
	}
	return string(bodyBytes), false, len(bodyBytes)
}

func (m *AuditModule) requestInfoFields() AuditRequestFields {
	if m.Request.Info.hasAny() || m.Response.Info.hasAny() {
		return m.Request.Info
	}
	return AuditRequestFields{Method: true, Origin: true}
}

func (m *AuditModule) requestDebugFields() AuditRequestFields {
	return m.Request.Debug
}

func (m *AuditModule) responseInfoFields() AuditResponseFields {
	if m.Request.Info.hasAny() || m.Response.Info.hasAny() {
		return m.Response.Info
	}
	return AuditResponseFields{Status: true, Duration: true, TargetOrigin: true}
}

func (m *AuditModule) responseDebugFields() AuditResponseFields {
	return m.Response.Debug
}

func (m *AuditModule) buildAttrs(reqFields AuditRequestFields, respFields AuditResponseFields, requestID string, r *http.Request, st *state.State, aw *auditResponseWriter, duration time.Duration, reqBody string, reqBodyLen int, reqBodyTruncated bool) []any {
	if r == nil || aw == nil {
		return nil
	}
	attrs := []any{
		"request_id", requestID,
	}

	if reqFields.Method {
		attrs = append(attrs, "method", r.Method)
	}
	if reqFields.Path {
		if r.URL != nil {
			attrs = append(attrs, "path", r.URL.Path)
		}
	}
	if reqFields.Query {
		if r.URL != nil {
			attrs = append(attrs, "query", r.URL.RawQuery)
		}
	}
	if reqFields.Headers {
		attrs = append(attrs, "request_headers", r.Header)
	}
	if reqFields.Body {
		attrs = append(attrs, "request_body", reqBody, "request_body_bytes", reqBodyLen)
		if reqBodyTruncated {
			attrs = append(attrs, "request_body_truncated", true)
		}
	}
	if reqFields.Host {
		attrs = append(attrs, "host", r.Host)
	}
	if reqFields.Origin {
		attrs = append(attrs, "source_origin", utils.RequestScheme(r)+"://"+r.Host)
	}
	if reqFields.RemoteAddr {
		attrs = append(attrs, "remote_addr", r.RemoteAddr)
	}

	if respFields.Status {
		attrs = append(attrs, "status", aw.status)
	}
	if respFields.Headers {
		attrs = append(attrs, "response_headers", aw.Header())
	}
	if respFields.Body {
		attrs = append(attrs, "response_body", aw.bodyString(), "response_body_bytes", aw.bodyBytes())
		if aw.bodyTruncated {
			attrs = append(attrs, "response_body_truncated", true)
		}
	}
	if respFields.Size {
		attrs = append(attrs, "response_bytes", aw.bytes)
	}
	if respFields.Duration {
		attrs = append(attrs, "duration_ms", duration.Milliseconds())
	}
	if respFields.TargetOrigin {
		if st != nil {
			if v, ok := st.Get(auditTargetOriginKey); ok {
				if origin, ok := v.(string); ok && origin != "" {
					attrs = append(attrs, "target_origin", origin)
				}
			}
		}
	}

	return attrs
}

func (f AuditRequestFields) hasAny() bool {
	return f.Method || f.Path || f.Query || f.Headers || f.Body || f.Host || f.Origin || f.RemoteAddr
}

func (f AuditResponseFields) hasAny() bool {
	return f.Status || f.Headers || f.Body || f.Size || f.Duration || f.TargetOrigin
}

type auditResponseWriter struct {
	http.ResponseWriter
	status        int
	bytes         int64
	captureBody   bool
	maxBodyBytes  int
	body          bytes.Buffer
	bodyTruncated bool
}

func newAuditResponseWriter(w http.ResponseWriter, captureBody bool, maxBodyBytes int) *auditResponseWriter {
	return &auditResponseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
		captureBody:    captureBody,
		maxBodyBytes:   maxBodyBytes,
	}
}

func (w *auditResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *auditResponseWriter) Write(p []byte) (int, error) {
	if w.captureBody && len(p) > 0 {
		w.captureResponseBody(p)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

func (w *auditResponseWriter) captureResponseBody(p []byte) {
	if w.maxBodyBytes <= 0 {
		_, _ = w.body.Write(p)
		return
	}
	remaining := w.maxBodyBytes - w.body.Len()
	if remaining <= 0 {
		w.bodyTruncated = true
		return
	}
	if len(p) > remaining {
		_, _ = w.body.Write(p[:remaining])
		w.bodyTruncated = true
		return
	}
	_, _ = w.body.Write(p)
}

func (w *auditResponseWriter) bodyString() string {
	if w.body.Len() == 0 {
		return ""
	}
	return w.body.String()
}

func (w *auditResponseWriter) bodyBytes() int {
	return w.body.Len()
}

func (w *auditResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *auditResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (w *auditResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}
