package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// newToken makes a per-run secret that every API call must present.
//
// Binding to 127.0.0.1 keeps other machines out, but it does not keep other
// *pages* out: any website open in the user's browser can issue requests to
// localhost. Since this server rewrites shell configuration, a token is the
// difference between a local tool and a remote code execution primitive.
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// tokenOK compares in constant time, so the comparison cannot be used as an
// oracle to guess the token byte by byte.
func tokenOK(want, got string) bool {
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// allowedHost reports whether a Host header names this loopback server.
//
// This is the defence against DNS rebinding: an attacker points a hostname
// they control at 127.0.0.1, so the browser treats their origin as same-site
// and sends requests here. Requiring the Host to literally be loopback breaks
// that, because their hostname will not match.
func allowedHost(host, port string) bool {
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		// No port in the header; a bare hostname is not something we serve.
		return false
	}
	if p != port {
		return false
	}
	switch h {
	case "127.0.0.1", "localhost", "[::1]", "::1":
		return true
	}
	return false
}

// allowedOrigin reports whether a cross-origin request may proceed.
//
// A same-origin request from our own page carries either no Origin or our own.
// Anything else is another site talking to us and is refused outright, before
// the token is even considered.
func allowedOrigin(origin, port string) bool {
	if origin == "" {
		// Non-CORS requests (the initial page load, same-origin fetches in
		// some browsers) send no Origin at all.
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return allowedHost(u.Host, port)
}

// guard wraps a handler with the origin, host and token checks.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost(r.Host, s.port) {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
		if !allowedOrigin(r.Header.Get("Origin"), s.port) {
			http.Error(w, "bad origin", http.StatusForbidden)
			return
		}

		// The page itself is fetched with the token in the query string,
		// because a browser navigation cannot set a header. Everything the
		// page then calls uses the header.
		got := r.Header.Get("X-Loadout-Token")
		if got == "" {
			got = r.URL.Query().Get("token")
		}
		if !tokenOK(s.token, got) {
			http.Error(w, "bad or missing token", http.StatusUnauthorized)
			return
		}

		// Nothing here should ever be cached or embedded elsewhere.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")

		next.ServeHTTP(w, r)
	})
}

// URL is the address to open, token included.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%s/?token=%s", s.port, url.QueryEscape(s.token))
}

// listenLoopback binds to 127.0.0.1 only. Binding to all interfaces would put
// a config-rewriting API on the local network.
func listenLoopback(preferred int) (net.Listener, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", preferred)
	ln, err := net.Listen("tcp", addr)
	if err != nil && preferred != 0 {
		return nil, fmt.Errorf("cannot listen on %s: %w", addr, err)
	}
	if err != nil {
		return nil, err
	}
	return ln, nil
}

// portOf extracts the port from a listener address.
func portOf(ln net.Listener) string {
	_, p, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(p)
}
