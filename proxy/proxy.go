package proxy

import (
	"log/slog"
	"maps"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/axent-pl/axproxy/module"
	s "github.com/axent-pl/axproxy/state"
)

type AuthProxy struct {
	Address     string              `yaml:"listen"`
	Prefix      string              `yaml:"special_prefix"`
	Upstreams   []Upstream          `yaml:"upstreams"`
	upstreamMap map[string]*url.URL `yaml:"-"`
	Chain       []Step              `yaml:"chain"`

	specialMux *http.ServeMux
}

type Step struct {
	ModuleRef ModuleRef     `yaml:"moduleRef"`
	module    module.Module `yaml:"-"`
}

type ModuleRef struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`
}

type Upstream struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

func (p *AuthProxy) ListenAndServe() error {
	p.specialMux = http.NewServeMux()

	if err := p.initUpstreamMap(); err != nil {
		return err
	}

	if err := p.initModules(); err != nil {
		return err
	}

	if err := p.registerSpecialRoutes(); err != nil {
		return err
	}

	proxy := httputil.ReverseProxy{}

	// --------------------
	// Director
	// --------------------
	directorHandler := module.ProxyDirectorHandlerFunc(func(r *http.Request, st *s.State) {
		p.proxyDirector(r)
	})
	for i := len(p.Chain) - 1; i >= 0; i-- {
		step := p.Chain[i]
		handlerWrapped := step.module.ProxyDirectorMiddleware(directorHandler)
		if handlerWrapped != nil {
			directorHandler = handlerWrapped
		}
	}
	proxy.Director = func(r *http.Request) {
		st := s.GetState(r.Context())
		directorHandler(r, st)
	}
	// --------------------

	// --------------------
	// ModifyResponse
	// --------------------
	modifyResponseHandler := module.ProxyModifyResponseHandlerFunc(func(*http.Response, *s.State) error {
		return nil
	})
	for i := len(p.Chain) - 1; i >= 0; i-- {
		step := p.Chain[i]
		handlerWrapped := step.module.ProxyModifyResponseMiddleware(modifyResponseHandler)
		if handlerWrapped != nil {
			modifyResponseHandler = handlerWrapped
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		st := s.GetState(resp.Request.Context())
		return modifyResponseHandler(resp, st)
	}
	// --------------------

	// ErrorHandler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy error", "error", err)
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}

	rootMux := http.NewServeMux()
	rootMux.Handle(p.Prefix+"/", http.StripPrefix(p.Prefix, p.specialMux))

	handler := module.ProxyHandlerFunc(func(w http.ResponseWriter, r *http.Request, st *s.State) {
		proxy.ServeHTTP(w, r)
	})
	for i := len(p.Chain) - 1; i >= 0; i-- {
		step := p.Chain[i]
		handlerWrapped := step.module.ProxyMiddleware(handler)
		if handlerWrapped != nil {
			handler = handlerWrapped
		}
	}

	rootMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		st := s.NewState()
		r = r.WithContext(s.WithState(r.Context(), st))
		handler(w, r, st)
	})

	return http.ListenAndServe(p.Address, rootMux)
}

func (p *AuthProxy) registerSpecialRoutes() error {
	specialRoutes := make(map[string]http.HandlerFunc)
	for i := len(p.Chain) - 1; i >= 0; i-- {
		step := p.Chain[i]
		if moduleSpecialRoutes := step.module.SpecialRoutes(); moduleSpecialRoutes != nil {
			maps.Copy(specialRoutes, moduleSpecialRoutes)
		}
	}
	for i := len(p.Chain) - 1; i >= 0; i-- {
		step := p.Chain[i]
		for r, h := range specialRoutes {
			if wrapped := step.module.Middleware(h); wrapped != nil {
				specialRoutes[r] = step.module.Middleware(h)
			}
		}
	}
	for r, h := range specialRoutes {
		p.specialMux.HandleFunc(r, func(w http.ResponseWriter, r *http.Request) {
			st := s.NewState()
			r = r.WithContext(s.WithState(r.Context(), st))
			h.ServeHTTP(w, r)
		})
	}
	return nil
}

func (p *AuthProxy) proxyDirector(req *http.Request) {
	scheme := "http"
	if req.TLS != nil || strings.EqualFold(req.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}

	key := strings.ToLower(scheme + "://" + req.Host)
	target, ok := p.upstreamMap[key]
	if !ok {
		return
	}

	targetQuery := target.RawQuery
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	req.URL.Path, req.URL.RawPath = joinURLPath(target, req.URL)
	if targetQuery == "" || req.URL.RawQuery == "" {
		req.URL.RawQuery = targetQuery + req.URL.RawQuery
	} else {
		req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
	}
	req.Host = target.Host
}

func (p *AuthProxy) initUpstreamMap() error {
	p.upstreamMap = map[string]*url.URL{}
	for _, u := range p.Upstreams {
		target, err := url.Parse(u.Target)
		if err != nil {
			return err
		}
		p.upstreamMap[u.Source] = target
	}
	return nil
}

func (p *AuthProxy) initModules() error {
	for idx, step := range p.Chain {
		mod, err := module.Get(step.ModuleRef.Kind, step.ModuleRef.Name)
		if err != nil {
			return err
		}
		p.Chain[idx].module = mod
	}
	return nil
}

func singleJoiningSlash(a, b string) string {
	aHas := strings.HasSuffix(a, "/")
	bHas := strings.HasPrefix(b, "/")
	switch {
	case aHas && bHas:
		return a + b[1:]
	case !aHas && !bHas:
		return a + "/" + b
	}
	return a + b
}

func joinURLPath(a, b *url.URL) (path, rawpath string) {
	if a.RawPath == "" && b.RawPath == "" {
		return singleJoiningSlash(a.Path, b.Path), ""
	}
	// Same as singleJoiningSlash, but uses EscapedPath to determine
	// whether a slash should be added
	apath := a.EscapedPath()
	bpath := b.EscapedPath()

	aslash := strings.HasSuffix(apath, "/")
	bslash := strings.HasPrefix(bpath, "/")

	switch {
	case aslash && bslash:
		return a.Path + b.Path[1:], apath + bpath[1:]
	case !aslash && !bslash:
		return a.Path + "/" + b.Path, apath + "/" + bpath
	}
	return a.Path + b.Path, apath + bpath
}
