package modules

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"github.com/axent-pl/axproxy/state"
)

const KIND_REWRITER string = "Rewriter"

type RewriterModule struct {
	module.NoopModule
	Metadata       manifest.ObjectMeta `yaml:"metadata"`
	Rewrite        map[string]string   `yaml:"rewrite"`
	ReplaceHeaders bool                `yaml:"headers"`
	ReplaceBody    bool                `yaml:"body"`
}

func (m *RewriterModule) Kind() string {
	return KIND_REWRITER
}

func (m *RewriterModule) Name() string {
	return m.Metadata.Name
}

func (m *RewriterModule) ProxyModifyResponseMiddleware(next module.ProxyModifyResponseHandlerFunc) module.ProxyModifyResponseHandlerFunc {
	return module.ProxyModifyResponseHandlerFunc(func(resp *http.Response, st *state.State) error {
		if resp == nil || len(m.Rewrite) == 0 {
			return next(resp, st)
		}

		replacements := make([]string, 0, len(m.Rewrite)*2)
		for r, v := range m.Rewrite {
			replacements = append(replacements, r, v)
		}
		if len(replacements) == 0 {
			return next(resp, st)
		}
		replacer := strings.NewReplacer(replacements...)

		if err := m.replaceHeaders(resp, st, replacer); err != nil {
			slog.Error("failed to replace URLs in response headers", "error", err)
			return err
		}

		if err := m.replaceBody(resp, st, replacer); err != nil {
			slog.Error("failed to replace URLs in response body", "error", err)
			return err
		}

		return next(resp, st)
	})
}

func (m *RewriterModule) replaceHeaders(resp *http.Response, _ *state.State, replacer *strings.Replacer) error {
	if !m.ReplaceHeaders {
		return nil
	}
	for key, values := range resp.Header {
		changed := false
		for i, v := range values {
			if v == "" {
				continue
			}
			nv := replacer.Replace(v)
			if nv != v {
				values[i] = nv
				changed = true
			}
		}
		if changed {
			resp.Header[key] = values
		}
	}
	return nil
}

func (m *RewriterModule) replaceBody(resp *http.Response, _ *state.State, replacer *strings.Replacer) error {
	if !m.ReplaceBody {
		return nil
	}
	if resp.Body == nil || resp.Body == http.NoBody {
		return nil
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && !strings.EqualFold(enc, "identity") {
		if !strings.EqualFold(strings.TrimSpace(enc), "gzip") {
			return nil
		}

		zr, err := gzip.NewReader(resp.Body)
		if err != nil && err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("could not open gzipped body reader: %v", err)
		}
		bodyBytes, err := io.ReadAll(zr)
		if err != nil {
			_ = zr.Close()
			return fmt.Errorf("could not read gzipped body: %v", err)
		}
		if err := zr.Close(); err != nil {
			return fmt.Errorf("could not close gzipped body reader: %v", err)
		}
		_ = resp.Body.Close()

		bodyStr := string(bodyBytes)
		newBodyStr := replacer.Replace(bodyStr)

		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		if err != nil {
			return err
		}
		if _, err := zw.Write([]byte(newBodyStr)); err != nil {
			_ = zw.Close()
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}

		resp.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
		resp.ContentLength = int64(buf.Len())
		resp.Header.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		return nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if len(bodyBytes) == 0 {
		resp.Body = http.NoBody
		resp.ContentLength = 0
		resp.Header.Del("Content-Length")
		return nil
	}

	bodyStr := string(bodyBytes)
	newBodyStr := replacer.Replace(bodyStr)
	resp.Body = io.NopCloser(strings.NewReader(newBodyStr))
	resp.ContentLength = int64(len(newBodyStr))
	resp.Header.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	return nil
}
