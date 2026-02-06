package mapper

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/axent-pl/axproxy/state"
)

// BuildSourceMap builds a mapper-compatible source map from env, session, request and response.
// Supported paths:
//   - session.<KEY>
//   - request.host|path|method|headers.<Header>[idx]
//   - response.status|host|path|method|headers.<Header>[idx]
func BuildSourceMap(sess *state.Session, req *http.Request, resp *http.Response) map[string]any {
	src := map[string]any{}

	src["env"] = envToAnyMap()

	if sess != nil {
		src["session"] = sess.GetValues()
	}
	if req != nil {
		src["request"] = requestToMap(req)
	}
	if resp != nil {
		src["response"] = responseToMap(resp)
	}

	return src
}

// ApplyRules applies mapper rules using env/session/request/response as source,
// then writes mapped values back into env/session/request/response.
func ApplyRules(sess *state.Session, req *http.Request, resp *http.Response, rules map[string]string) error {
	src := BuildSourceMap(sess, req, resp)
	dst := map[string]any{}
	if err := Apply(dst, src, rules); err != nil {
		return err
	}
	return ApplyToTargets(dst, sess, req, resp)
}

// ApplyToTargets writes mapped values into env/session/request/response.
// It expects the dst map to contain optional top-level keys: env, session, request, response.
func ApplyToTargets(dst map[string]any, sess *state.Session, req *http.Request, resp *http.Response) error {
	if dst == nil {
		return nil
	}

	if sessDst, ok := dst["session"].(map[string]any); ok && sess != nil {
		sess.SetValues(sessDst)
	}
	if reqDst, ok := dst["request"].(map[string]any); ok && req != nil {
		if err := applyRequest(req, reqDst); err != nil {
			return err
		}
	}
	if respDst, ok := dst["response"].(map[string]any); ok && resp != nil {
		if err := applyResponse(resp, respDst); err != nil {
			return err
		}
	}

	return nil
}

func envToAnyMap() map[string]any {
	out := map[string]any{}
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		key := parts[0]
		value := ""
		if len(parts) > 1 {
			value = parts[1]
		}
		out[key] = value
	}
	return out
}

func requestToMap(r *http.Request) map[string]any {
	out := map[string]any{
		"host":    r.Host,
		"method":  r.Method,
		"headers": headersToAnyMap(r.Header),
	}
	if r.URL != nil {
		out["path"] = r.URL.Path
	}
	return out
}

func responseToMap(resp *http.Response) map[string]any {
	out := map[string]any{
		"status":  resp.StatusCode,
		"headers": headersToAnyMap(resp.Header),
	}
	if resp.Request != nil {
		out["host"] = resp.Request.Host
		out["method"] = resp.Request.Method
		if resp.Request.URL != nil {
			out["path"] = resp.Request.URL.Path
		}
	}
	return out
}

func headersToAnyMap(h http.Header) map[string]any {
	out := map[string]any{}
	for k, vs := range h {
		if len(vs) == 0 {
			continue
		}
		values := make([]any, 0, len(vs))
		for _, v := range vs {
			values = append(values, v)
		}
		out[k] = values
	}
	return out
}

func applyRequest(req *http.Request, m map[string]any) error {
	if host, ok := m["host"]; ok {
		req.Host = fmt.Sprint(host)
		if req.URL != nil {
			req.URL.Host = req.Host
		}
	}
	if path, ok := m["path"]; ok {
		if req.URL == nil {
			req.URL = &url.URL{}
		}
		req.URL.Path = fmt.Sprint(path)
	}
	if method, ok := m["method"]; ok {
		req.Method = fmt.Sprint(method)
	}
	if hdr, ok := m["headers"]; ok {
		header, err := anyToHeader(hdr)
		if err != nil {
			return err
		}
		if req.Header == nil {
			req.Header = http.Header{}
		}
		for k, vs := range header {
			if len(vs) == 0 {
				req.Header.Del(k)
				continue
			}
			req.Header[k] = vs
		}
	}
	return nil
}

func applyResponse(resp *http.Response, m map[string]any) error {
	if status, ok := m["status"]; ok {
		code, err := anyToInt(status)
		if err != nil {
			return fmt.Errorf("invalid response status: %w", err)
		}
		resp.StatusCode = code
		resp.Status = fmt.Sprintf("%d %s", code, http.StatusText(code))
	}
	if hdr, ok := m["headers"]; ok {
		header, err := anyToHeader(hdr)
		if err != nil {
			return err
		}
		if resp.Header == nil {
			resp.Header = http.Header{}
		}
		for k, vs := range header {
			if len(vs) == 0 {
				resp.Header.Del(k)
				continue
			}
			resp.Header[k] = vs
		}
	}
	if resp.Request != nil {
		if host, ok := m["host"]; ok {
			resp.Request.Host = fmt.Sprint(host)
			if resp.Request.URL != nil {
				resp.Request.URL.Host = resp.Request.Host
			}
		}
		if path, ok := m["path"]; ok {
			if resp.Request.URL == nil {
				resp.Request.URL = &url.URL{}
			}
			resp.Request.URL.Path = fmt.Sprint(path)
		}
		if method, ok := m["method"]; ok {
			resp.Request.Method = fmt.Sprint(method)
		}
	}
	return nil
}

func anyToHeader(v any) (http.Header, error) {
	switch tv := v.(type) {
	case http.Header:
		return cloneHeader(tv), nil
	case map[string]string:
		h := http.Header{}
		for k, v := range tv {
			if v == "" {
				continue
			}
			h[k] = []string{v}
		}
		return h, nil
	case map[string]any:
		h := http.Header{}
		for k, raw := range tv {
			values, err := anyToStrings(raw)
			if err != nil {
				return nil, fmt.Errorf("header %q: %w", k, err)
			}
			if len(values) == 0 {
				continue
			}
			h[k] = values
		}
		return h, nil
	default:
		return nil, fmt.Errorf("invalid headers type %T", v)
	}
}

func anyToStrings(v any) ([]string, error) {
	switch tv := v.(type) {
	case nil:
		return nil, nil
	case string:
		if tv == "" {
			return nil, nil
		}
		return []string{tv}, nil
	case []string:
		out := make([]string, 0, len(tv))
		for _, s := range tv {
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(tv))
		for _, item := range tv {
			if item == nil {
				continue
			}
			s := fmt.Sprint(item)
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid header value type %T", v)
	}
}

func cloneHeader(h http.Header) http.Header {
	out := http.Header{}
	for k, vs := range h {
		if len(vs) == 0 {
			continue
		}
		cp := make([]string, len(vs))
		copy(cp, vs)
		out[k] = cp
	}
	return out
}

func anyToInt(v any) (int, error) {
	switch tv := v.(type) {
	case int:
		return tv, nil
	case int8:
		return int(tv), nil
	case int16:
		return int(tv), nil
	case int32:
		return int(tv), nil
	case int64:
		return int(tv), nil
	case uint:
		return int(tv), nil
	case uint8:
		return int(tv), nil
	case uint16:
		return int(tv), nil
	case uint32:
		return int(tv), nil
	case uint64:
		return int(tv), nil
	case float32:
		return int(tv), nil
	case float64:
		return int(tv), nil
	case string:
		return strconvAtoi(tv)
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

func strconvAtoi(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return n, nil
}
