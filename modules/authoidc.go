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
	"sync"

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

	Scope        string `yaml:"scope"`
	ClientId     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	TokenURL     string `yaml:"token_url"`
	AuthorizeURL string `yaml:"authorize_url"`
	JWKSURL      string `yaml:"jwks_url"`

	jwksStartOnce sync.Once
	jwksScheme    xjwt.JWKSJWTScheme `yaml:"-"`
	jwtVerifier   xjwt.JWTVerifier   `yaml:"-"`
}

func (m *AuthOIDCModule) Kind() string {
	return KIND_AUTHOIDC
}

func (m *AuthOIDCModule) Name() string {
	return m.Metadata.Name
}

func (m *AuthOIDCModule) SpecialRoutes() map[string]http.HandlerFunc {
	jwksURL, err := url.Parse(m.JWKSURL)
	if err != nil {
		slog.Error("invalid JWKS URL", "error", err)
	}
	m.jwksStartOnce.Do(func() {
		m.jwksScheme = xjwt.JWKSJWTScheme{
			JWKSURL: *jwksURL,
		}
		m.jwksScheme.Start(context.Background())
		m.jwtVerifier = xjwt.JWTVerifier{}
	})

	return map[string]http.HandlerFunc{
		"/oidc-callback": m.getCallbackHandler(),
		"/oidc-login":    m.getLoginHandler(),
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
		sess.Values["state"] = oidcState
		sess.Values["nonce"] = oidcNonce

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
		if oidc_state, ok := sess.Values["oidc_state"]; ok {
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

		slog.Info("principal", "principal", principal)

		// if nonce, ok := principal.Attributes["nonce"].(string); !ok || nonce != sess.Nonce {
		// 	slog.Error(fmt.Sprintf("nonce does not match: wat %s, got %s", sess.Nonce, nonce))
		// 	http.Error(w, "could not request token", http.StatusUnauthorized)
		// 	return
		// }

		// sess.SubjectIDs = map[session.IdentityProvider]string{session.IPD_OIDC: subject}

		if entrypoint != "" {
			http.Redirect(w, r, entrypoint, http.StatusFound)
		} else {
			http.Redirect(w, r, "/", http.StatusFound)
		}
	})
}
