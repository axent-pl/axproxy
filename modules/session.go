package modules

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"github.com/axent-pl/axproxy/state"
	"github.com/axent-pl/axproxy/utils"
)

const KIND_SESSION string = "Session"

type SessionModule struct {
	module.NoopModule
	Metadata manifest.ObjectMeta `yaml:"metadata"`

	CookieName     string `yaml:"cookie_name"`
	CookiePath     string `yaml:"cookie_path"`
	CookieDomain   string `yaml:"cookie_domain"`
	CookieSecure   *bool  `yaml:"cookie_secure"`
	CookieHTTPOnly *bool  `yaml:"cookie_http_only"`
	CookieSameSite string `yaml:"cookie_same_site"`
	MaxAgeSeconds  int    `yaml:"max_age_seconds"`

	storeOnce sync.Once                 `yaml:"-"`
	storeMu   sync.RWMutex              `yaml:"-"`
	store     map[string]*state.Session `yaml:"-"`
}

func (m *SessionModule) Kind() string {
	return KIND_SESSION
}

func (m *SessionModule) Name() string {
	return m.Metadata.Name
}

func (m *SessionModule) ProxyMiddleware(next module.ProxyHandlerFunc) module.ProxyHandlerFunc {
	return module.ProxyHandlerFunc(func(w http.ResponseWriter, r *http.Request, st *state.State) {
		if r == nil || st == nil {
			next(w, r, st)
			return
		}

		sess, isNew := m.getOrCreateSession(r)
		st.Session = sess
		if isNew {
			http.SetCookie(w, m.buildCookie(r, sess))
		}

		next(w, r, st)

		m.saveSession(sess)
	})
}

func (m *SessionModule) getOrCreateSession(r *http.Request) (*state.Session, bool) {
	m.initStore()

	name := m.cookieName()
	if name != "" {
		if c, err := r.Cookie(name); err == nil && c.Value != "" {
			if sess, ok := m.getSessionByID(c.Value); ok {
				if sess.IsExpired() {
					m.deleteSession(c.Value)
				} else {
					sess.UpdatedAt = time.Now().UTC()
					return sess, false
				}
			}
		}
	}

	id, err := newSessionID()
	if err != nil {
		slog.Error("failed to generate session id", "error", err)
		return state.NewSession("invalid", m.MaxAgeSeconds), false
	}
	sess := state.NewSession(id, m.MaxAgeSeconds)
	m.saveSession(sess)
	return sess, true
}

func (m *SessionModule) getSessionByID(id string) (*state.Session, bool) {
	m.storeMu.RLock()
	defer m.storeMu.RUnlock()
	sess, ok := m.store[id]
	return sess, ok
}

func (m *SessionModule) saveSession(sess *state.Session) {
	m.storeMu.Lock()
	defer m.storeMu.Unlock()
	m.store[sess.ID] = sess
}

func (m *SessionModule) deleteSession(id string) {
	m.storeMu.Lock()
	defer m.storeMu.Unlock()
	delete(m.store, id)
}

func (m *SessionModule) initStore() {
	m.storeOnce.Do(func() {
		m.store = map[string]*state.Session{}
	})
}

func (m *SessionModule) buildCookie(r *http.Request, sess *state.Session) *http.Cookie {
	cookie := &http.Cookie{
		Name:     m.cookieName(),
		Value:    sess.ID,
		Path:     m.cookiePath(),
		Domain:   m.CookieDomain,
		HttpOnly: m.cookieHTTPOnly(),
		Secure:   m.cookieSecure(r),
		SameSite: m.cookieSameSite(),
	}

	if m.MaxAgeSeconds > 0 {
		cookie.MaxAge = m.MaxAgeSeconds
		exp := time.Now().UTC().Add(time.Duration(m.MaxAgeSeconds) * time.Second)
		cookie.Expires = exp
	}

	return cookie
}

func (m *SessionModule) cookieName() string {
	if m.CookieName != "" {
		return m.CookieName
	}
	return "axproxy_session"
}

func (m *SessionModule) cookiePath() string {
	if m.CookiePath != "" {
		return m.CookiePath
	}
	return "/"
}

func (m *SessionModule) cookieHTTPOnly() bool {
	if m.CookieHTTPOnly != nil {
		return *m.CookieHTTPOnly
	}
	return true
}

func (m *SessionModule) cookieSecure(r *http.Request) bool {
	if m.CookieSecure != nil {
		return *m.CookieSecure
	}
	return utils.RequestScheme(r) == "https"
}

func (m *SessionModule) cookieSameSite() http.SameSite {
	switch strings.ToLower(strings.TrimSpace(m.CookieSameSite)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	case "lax", "":
		return http.SameSiteLaxMode
	default:
		return http.SameSiteLaxMode
	}
}

func newSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
