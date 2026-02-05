package enrichment

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"
)

type LdapEnrichmentSourceConfig struct {
	Addr         string        `yaml:"addr"`
	BindDN       string        `yaml:"bind_dn"`
	BindPassword string        `yaml:"bind_password"`
	BaseDN       string        `yaml:"base_dn"`
	Timeout      time.Duration `yaml:"timeout"`

	TLSEnabled            bool   `yaml:"tls_enabled"`
	TLSServerName         string `yaml:"tls_server_name"`
	TLSInsecureSkipVerify bool   `yaml:"tls_insecure_skip_verify"`
	TLSCAFile             string `yaml:"tls_ca_file"`
	TLSClientCertFile     string `yaml:"tls_client_cert_file"`
	TLSClientKeyFile      string `yaml:"tls_client_key_file"`
}

type LdapEnrichmentSource struct {
	mu     sync.RWMutex
	conn   *ldap.Conn
	cfg    *LdapEnrichmentSourceConfig
	BaseDN string
}

// helper: establish a new connection + bind using cfg
func dialAndBind(cfg *LdapEnrichmentSourceConfig) (*ldap.Conn, error) {
	// TCP keep-alive to prevent idle disconnects at the TCP layer
	dialer := &net.Dialer{
		Timeout:   10 * time.Second, // connection timeout
		KeepAlive: 30 * time.Second, // enable keepalive probes
	}

	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	dialOpts := []ldap.DialOpt{ldap.DialWithDialer(dialer)}
	if tlsCfg != nil && strings.HasPrefix(strings.ToLower(cfg.Addr), "ldaps://") {
		dialOpts = append(dialOpts, ldap.DialWithTLSConfig(tlsCfg))
	}

	c, err := ldap.DialURL(cfg.Addr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("ldap dial: %w", err)
	}

	if cfg.TLSEnabled {
		if !strings.HasPrefix(strings.ToLower(cfg.Addr), "ldap://") {
			c.Close()
			return nil, fmt.Errorf("starttls requires ldap:// address")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			c.Close()
			return nil, fmt.Errorf("starttls: %w", err)
		}
	}

	// Per-operation I/O timeout (Search/Bind/etc.)
	if cfg.Timeout > 0 {
		c.SetTimeout(cfg.Timeout)
	}

	// Initial bind
	if err := c.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
		c.Close()
		return nil, fmt.Errorf("bind failed: %w", err)
	}

	return c, nil
}

func buildTLSConfig(cfg *LdapEnrichmentSourceConfig) (*tls.Config, error) {
	if cfg == nil {
		return nil, nil
	}

	needsTLS := cfg.TLSEnabled ||
		cfg.TLSServerName != "" ||
		cfg.TLSInsecureSkipVerify ||
		cfg.TLSCAFile != "" ||
		cfg.TLSClientCertFile != "" ||
		cfg.TLSClientKeyFile != ""
	if !needsTLS {
		return nil, nil
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
	}

	serverName := cfg.TLSServerName
	if serverName == "" {
		serverName = hostFromAddr(cfg.Addr)
	}
	if serverName != "" {
		tlsCfg.ServerName = serverName
	}

	if cfg.TLSCAFile != "" {
		caData, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read tls ca file: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(caData) {
			return nil, fmt.Errorf("append tls ca file: no certs found")
		}
		tlsCfg.RootCAs = pool
	}

	if cfg.TLSClientCertFile != "" || cfg.TLSClientKeyFile != "" {
		if cfg.TLSClientCertFile == "" || cfg.TLSClientKeyFile == "" {
			return nil, fmt.Errorf("tls client cert and key files must both be set")
		}
		cert, err := tls.LoadX509KeyPair(cfg.TLSClientCertFile, cfg.TLSClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load tls client cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

func hostFromAddr(addr string) string {
	if addr == "" {
		return ""
	}
	u, err := url.Parse(addr)
	if err != nil {
		return ""
	}
	host := u.Host
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func NewLdapEnrichmentSource(cfg *LdapEnrichmentSourceConfig) (*LdapEnrichmentSource, error) {
	c, err := dialAndBind(cfg)
	if err != nil {
		return nil, err
	}
	return &LdapEnrichmentSource{conn: c, cfg: cfg, BaseDN: cfg.BaseDN}, nil
}

// Close closes the underlying connection.
func (lc *LdapEnrichmentSource) Close() error {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.conn != nil {
		lc.conn.Close()
		lc.conn = nil
	}
	return nil
}

// ensureConn makes sure we have a live, bound connection.
// It performs a very cheap RootDSE base search as a ping when possible.
// If the conn is dead, it reconnects.
func (lc *LdapEnrichmentSource) ensureConn() error {
	lc.mu.RLock()
	c := lc.conn
	lc.mu.RUnlock()

	if c == nil {
		// reconnect
		return lc.reconnect()
	}

	// If the underlying Conn reports closing, reconnect
	if c.IsClosing() {
		return lc.reconnect()
	}

	// Lightweight ping to catch half-closed sockets:
	pingReq := ldap.NewSearchRequest(
		"", // RootDSE
		ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1,
		5, false,
		"(objectClass=*)",
		[]string{"supportedLDAPVersion"},
		nil,
	)
	if _, err := c.Search(pingReq); err != nil {
		// If it's a network error, reconnect.
		if isNetworkError(err) {
			return lc.reconnect()
		}
		// Otherwise bubble up (server-side auth/ACL issues, etc.)
		return fmt.Errorf("ldap ping failed: %w", err)
	}

	return nil
}

func (lc *LdapEnrichmentSource) reconnect() error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	// Double-check in case another goroutine already reconnected
	if lc.conn != nil && !lc.conn.IsClosing() {
		return nil
	}

	newConn, err := dialAndBind(lc.cfg)
	if err != nil {
		return err
	}
	// swap
	if lc.conn != nil {
		lc.conn.Close()
	}
	lc.conn = newConn
	return nil
}

// doSearch executes a search, retrying once if we hit a network/closed-conn error.
func (lc *LdapEnrichmentSource) doSearch(req *ldap.SearchRequest) (*ldap.SearchResult, error) {
	// ensure we have a live connection first
	if err := lc.ensureConn(); err != nil {
		return nil, err
	}

	// read lock for using the connection
	lc.mu.RLock()
	c := lc.conn
	lc.mu.RUnlock()

	res, err := c.Search(req)
	if err == nil {
		return res, nil
	}
	if !isNetworkError(err) {
		return nil, fmt.Errorf("ldap search error: %w", err)
	}

	// network/closed-conn: reconnect and retry once
	if rerr := lc.reconnect(); rerr != nil {
		return nil, fmt.Errorf("reconnect failed after network error: %v (orig: %w)", rerr, err)
	}

	lc.mu.RLock()
	c = lc.conn
	lc.mu.RUnlock()

	res, err = c.Search(req)
	if err != nil {
		return nil, fmt.Errorf("ldap search error after reconnect: %w", err)
	}
	return res, nil
}

// helper: detect go-ldap network error or closed connection
func isNetworkError(err error) bool {
	// go-ldap wraps network errors in *ldap.Error with ResultCode=ErrorNetwork (200)
	// plus keep a string check as a fallback.
	if ldap.IsErrorWithCode(err, ldap.ErrorNetwork) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "network error")
}

// allowedAttributeName ensures attribute field itself cannot inject filter syntax.
var allowedAttributeName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func (lc *LdapEnrichmentSource) Lookup(ctx context.Context, inputs map[string]string, outputs []string) (map[string]any, error) {
	if lc == nil {
		return nil, errors.New("ldap client not initialized")
	}

	filters := make([]string, len(inputs))
	filterIdx := 0
	for attributeName, attributeValue := range inputs {
		if !allowedAttributeName.MatchString(attributeName) {
			return nil, fmt.Errorf("invalid input name: %q", attributeName)
		}
		safeValue := ldap.EscapeFilter(attributeValue)
		filters[filterIdx] = fmt.Sprintf("(%s=%s)", attributeName, safeValue)
		filterIdx++
	}

	filter := "(&" + strings.Join(filters, "") + ")"

	req := ldap.NewSearchRequest(
		lc.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		2,     // SizeLimit=2 to detect >1 match
		10,    // TimeLimit=10s (server-side)
		false, // typesOnly
		filter,
		outputs,
		nil,
	)

	res, err := lc.doSearch(req)
	if err != nil {
		return nil, err
	}

	if len(res.Entries) != 1 {
		if len(res.Entries) == 0 {
			return nil, errors.New("no records found")
		}
		return nil, fmt.Errorf("expected exactly 1 record, got %d", len(res.Entries))
	}

	results := make(map[string]any)
	for _, outName := range outputs {
		results[outName] = res.Entries[0].GetAttributeValue(outName)
	}
	return results, nil
}
