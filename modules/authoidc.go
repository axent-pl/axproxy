package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"github.com/axent-pl/axproxy/state"
	"github.com/axent-pl/axproxy/utils"
	xjwt "github.com/axent-pl/credentials/jwt"
)

const KIND_AUTHOIDC string = "AuthOIDC"

type AuthOIDCModule struct {
	module.NoopModule
	Metadata manifest.ObjectMeta `yaml:"metadata"`

	SessionSubjectIDKey string `yaml:"session_subject_id_key"`
	SessionClaimsKey    string `yaml:"session_claims_key"`

	Scope        string `yaml:"scope"`
	ClientId     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	TokenURL     string `yaml:"token_url"`
	AuthorizeURL string `yaml:"authorize_url"`
	JWKSURL      string `yaml:"jwks_url"`

	jwksScheme  xjwt.JWKSJWTScheme `yaml:"-"`
	jwtVerifier xjwt.JWTVerifier   `yaml:"-"`
}

func (m *AuthOIDCModule) Kind() string {
	return KIND_AUTHOIDC
}

func (m *AuthOIDCModule) Name() string {
	return m.Metadata.Name
}

func (m *AuthOIDCModule) Start() error {
	jwksURL, err := url.Parse(m.JWKSURL)
	if err != nil {
		slog.Error("invalid JWKS URL", "error", err)
		return fmt.Errorf("invalid JWKS URL: %v", err)
	}
	m.jwksScheme = xjwt.JWKSJWTScheme{
		JWKSURL: *jwksURL,
	}
	m.jwksScheme.Start(context.Background())
	m.jwtVerifier = xjwt.JWTVerifier{}
	return nil
}

func (m *AuthOIDCModule) SpecialRoutes() map[string]http.HandlerFunc {

	return map[string]http.HandlerFunc{
		"/oidc-callback": m.getCallbackHandler(),
		"/oidc-login":    m.getLoginHandler(),
	}
}

func (m *AuthOIDCModule) ProxyMiddleware(next module.ProxyHandlerFunc) module.ProxyHandlerFunc {
	return module.ProxyHandlerFunc(func(w http.ResponseWriter, r *http.Request, st *state.State) {
		if r == nil || st == nil {
			next(w, r, st)
			return
		}
		sess := st.Session
		subjectID, err := m.readSubjectID(sess)
		if err != nil {
			scheme := utils.RequestScheme(r)
			currentURL := scheme + "://" + r.Host + r.URL.RequestURI()
			loginURL := &url.URL{
				Scheme: utils.RequestScheme(r),
				Host:   r.Host,
				Path:   "/_/oidc-login",
			}
			q := loginURL.Query()
			q.Set("entrypoint_url", currentURL)
			loginURL.RawQuery = q.Encode()
			http.Redirect(w, r, loginURL.String(), http.StatusFound)
			slog.Info("AuthOIDCModule redirecting to oidc-login", "request_id", st.RequestID)
			return
		}
		slog.Info("AuthOIDCModule authentication completed", "request_id", st.RequestID, "subjectID", subjectID)
		next(w, r, st)
	})
}

func (m *AuthOIDCModule) readSubjectID(session *state.Session) (subjectID string, err error) {
	key := m.SessionSubjectIDKey
	if key == "" {
		key = "oidc_subject_id"
	}

	sub, err := session.GetValue(key)
	if err != nil {
		return "", fmt.Errorf("could not read subject_id from session (key:%s): %v", key, err)
	}
	subStr, ok := sub.(string)
	if !ok {
		return "", fmt.Errorf("could not read subject_id from session (key:%s): invalid type", key)
	}
	return subStr, nil

}

func (m *AuthOIDCModule) storePrincipal(session *state.Session, subjectID string, claims map[string]any) {
	if m.SessionSubjectIDKey != "" {
		session.SetValue(m.SessionSubjectIDKey, subjectID)
	} else {
		session.SetValue("oidc_subject_id", subjectID)
	}
	if m.SessionClaimsKey != "" {
		session.SetValue(m.SessionClaimsKey, claims)
	} else {
		session.SetValue("oidc_claims", claims)
	}
}

// special handlers

func (m *AuthOIDCModule) getLoginHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callbackURL := &url.URL{
			Scheme: utils.RequestScheme(r),
			Host:   r.Host,
			Path:   "/_/oidc-callback",
		}

		entrypoint := r.URL.Query().Get("entrypoint_url")
		if entrypoint != "" {
			q := callbackURL.Query()
			q.Set("entrypoint_url", entrypoint)
			callbackURL.RawQuery = q.Encode()
		}

		oidcState, err := utils.RandomURLSafe(32)
		if err != nil {
			http.Error(w, "could not connect to authorization server", http.StatusInternalServerError)
			return
		}
		oidcNonce, err := utils.RandomURLSafe(32)
		if err != nil {
			http.Error(w, "could not connect to authorization server", http.StatusInternalServerError)
			return
		}

		st := state.GetState(r.Context())
		sess := st.Session
		sess.SetValue("state", oidcState)
		sess.SetValue("nonce", oidcNonce)

		authURL, err := url.Parse(m.AuthorizeURL)
		if err != nil {
			http.Error(w, "could not connect to authorization server", http.StatusInternalServerError)
			return
		}
		q := authURL.Query()
		q.Set("redirect_uri", callbackURL.String())
		q.Set("response_type", "code")
		q.Set("client_id", m.ClientId)
		q.Set("scope", m.Scope)
		q.Set("state", oidcState)
		q.Set("nonce", oidcNonce)
		authURL.RawQuery = q.Encode()
		http.Redirect(w, r, authURL.String(), http.StatusFound)
		slog.Info("AuthOIDCModule redirecting to authorization server", "request_id", st.RequestID, "authorize_url", m.AuthorizeURL)
	})
}

func (m *AuthOIDCModule) getCallbackHandler() http.HandlerFunc {
	type tokenResponeStruct struct {
		TokenType          string `json:"token_type"`
		ExpiresInSeconds   int    `json:"expires_in"`
		AccessTokenEncoded string `json:"access_token"`
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization_code := r.URL.Query().Get("code")
		if authorization_code == "" {
			slog.Error("missing authorization code")
			http.Error(w, "missing authorization code", http.StatusUnauthorized)
			return
		}
		st := state.GetState(r.Context())
		sess := st.Session

		callbackURL := &url.URL{
			Scheme: utils.RequestScheme(r),
			Host:   r.Host,
			Path:   "/_/oidc-callback",
		}
		entrypoint := r.URL.Query().Get("entrypoint_url")
		if entrypoint != "" {
			q := callbackURL.Query()
			q.Set("entrypoint_url", entrypoint)
			callbackURL.RawQuery = q.Encode()
		}

		form := url.Values{}
		form.Add("grant_type", "authorization_code")
		form.Add("code", authorization_code)
		if oidc_state, err := sess.GetValue("oidc_state"); err == nil {
			form.Add("state", oidc_state.(string))
		}
		form.Add("redirect_uri", callbackURL.String())
		form.Add("client_id", m.ClientId)
		form.Add("client_secret", m.ClientSecret)
		tokenHTTPResponse, err := http.Post(m.TokenURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		if err != nil {
			slog.Error(fmt.Sprintf("could not request token: %v", err))
			http.Error(w, "could not request token", http.StatusUnauthorized)
			return
		}
		defer func() {
			err := tokenHTTPResponse.Body.Close()
			if err != nil {
				slog.Error("could not close OIDC token response body", "error", err)
			}
		}()
		if tokenHTTPResponse.StatusCode != http.StatusOK {
			slog.Error(fmt.Sprintf("could not request token: status %s", tokenHTTPResponse.Status))
			http.Error(w, "could not request token", http.StatusUnauthorized)
			return
		}
		tokenHTTPResponseBody, err := io.ReadAll(tokenHTTPResponse.Body)
		if err != nil {
			slog.Error(fmt.Sprintf("could not read request token response: %v", err))
			http.Error(w, "could not request token", http.StatusUnauthorized)
			return
		}
		tokenResponse := tokenResponeStruct{}
		if err = json.Unmarshal(tokenHTTPResponseBody, &tokenResponse); err != nil {
			slog.Error(fmt.Sprintf("could not unmarshal request token response: %v", err))
			http.Error(w, "could not request token", http.StatusUnauthorized)
			return
		}

		principal, err := m.jwtVerifier.Verify(r.Context(), xjwt.JWTCredentials{Token: tokenResponse.AccessTokenEncoded}, &m.jwksScheme)
		if err != nil {
			slog.Error("token verification failed", "error", err)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		if oidc_nonce, err := sess.GetValue("oidc_nonce"); err == nil {
			if nonce, ok := principal.Attributes["nonce"].(string); !ok || nonce != oidc_nonce {
				slog.Error(fmt.Sprintf("nonce does not match: wat %s, got %s", oidc_nonce, nonce))
				http.Error(w, "could not request token", http.StatusUnauthorized)
				return
			}
		}

		m.storePrincipal(sess, string(principal.Subject), principal.Attributes)

		if entrypoint != "" {
			http.Redirect(w, r, entrypoint, http.StatusFound)
		} else {
			http.Redirect(w, r, "/", http.StatusFound)
		}
	})
}
