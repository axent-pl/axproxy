package modules

import (
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"sync"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"github.com/axent-pl/axproxy/state"
)

const KIND_COOKIE string = "Cookie"

type CookieModule struct {
	module.NoopModule
	Metadata manifest.ObjectMeta `yaml:"metadata"`

	jar     *cookiejar.Jar `yaml:"-"`
	jarOnce sync.Once      `yaml:"-"`
	jarErr  error          `yaml:"-"`
}

func (m *CookieModule) Kind() string {
	return KIND_COOKIE
}

func (m *CookieModule) Name() string {
	return m.Metadata.Name
}

func (m *CookieModule) ProxyDirectorMiddleware(next module.ProxyDirectorHandlerFunc) module.ProxyDirectorHandlerFunc {
	return module.ProxyDirectorHandlerFunc(func(r *http.Request, st *state.State) {
		if r == nil || r.URL == nil {
			next(r, st)
		}

		jar, err := m.getJar()
		if err != nil {
			slog.Error("could not get upstream cookie JAR", "error", err)
			return
		}

		jarCookies := jar.Cookies(r.URL)
		clientCookies := r.Cookies()
		if len(jarCookies) == 0 && len(clientCookies) == 0 {
			next(r, st)
		}

		combined := map[string]*http.Cookie{}
		for _, c := range jarCookies {
			combined[c.Name] = c
		}
		for _, c := range clientCookies {
			combined[c.Name] = c
		}

		r.Header.Del("Cookie")
		for _, c := range combined {
			r.AddCookie(c)
		}
		next(r, st)
	})
}

func (m *CookieModule) ProxyModifyResponseMiddleware(next module.ProxyModifyResponseHandlerFunc) module.ProxyModifyResponseHandlerFunc {
	return module.ProxyModifyResponseHandlerFunc(func(resp *http.Response, st *state.State) error {
		if resp == nil || resp.Request == nil || resp.Request.URL == nil {
			return next(resp, st)
		}

		cookies := resp.Cookies()
		if len(cookies) == 0 {
			return next(resp, st)
		}

		jar, err := m.getJar()
		if err != nil {
			slog.Error("failed to get get COOKIE jar", "error", err)
			return err
		}

		jar.SetCookies(resp.Request.URL, cookies)
		return next(resp, st)
	})
}

func (m *CookieModule) getJar() (*cookiejar.Jar, error) {
	m.jarOnce.Do(func() {
		m.jar, m.jarErr = cookiejar.New(nil)
	})
	return m.jar, m.jarErr
}
