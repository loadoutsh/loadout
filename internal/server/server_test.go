package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loadoutsh/loadout/internal/catalog"
)

// start brings up a real server on a free loopback port against a temporary
// config directory, so nothing touches the developer's own setup.
func start(t *testing.T) (*Server, string) {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))

	cfgDir := filepath.Join(dir, "loadout")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `version: 1
paths:
  project_root: ~/Projects
  venv_root: ~/.venv
editors:
  - name: code
    command: code
credentials:
  npm_token:
    active: personal
    entries: [personal, work]
packs: [flutter]
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	cat, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(cat, filepath.Join(dir, "bin"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ready := make(chan string, 1)
	go func() {
		_ = srv.Serve(ctx, 0, func(url string) { ready <- url })
	}()

	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	return srv, "http://127.0.0.1:" + srv.port
}

func do(t *testing.T, method, url string, mut func(*http.Request)) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mut != nil {
		mut(req)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

// Without the token, nothing is served. Binding to loopback is not on its own
// a defence: any page in the user's browser can reach 127.0.0.1.
func TestRequiresToken(t *testing.T) {
	_, base := start(t)

	res := do(t, "GET", base+"/api/state", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", res.StatusCode)
	}

	res = do(t, "GET", base+"/api/state", func(r *http.Request) {
		r.Header.Set("X-Loadout-Token", "wrong-token")
	})
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", res.StatusCode)
	}
}

func TestTokenGrantsAccess(t *testing.T) {
	srv, base := start(t)
	res := do(t, "GET", base+"/api/state", func(r *http.Request) {
		r.Header.Set("X-Loadout-Token", srv.token)
	})
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, body = %s", res.StatusCode, body)
	}
	var state stateResponse
	if err := json.NewDecoder(res.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if len(state.Aliases) == 0 {
		t.Error("state should include aliases")
	}
	if state.ConfigError != "" {
		t.Errorf("unexpected config error: %s", state.ConfigError)
	}
}

// A page on another site must not be able to drive this API even if it
// somehow learned the token.
func TestRejectsForeignOrigin(t *testing.T) {
	srv, base := start(t)
	res := do(t, "GET", base+"/api/state", func(r *http.Request) {
		r.Header.Set("X-Loadout-Token", srv.token)
		r.Header.Set("Origin", "https://evil.example.com")
	})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

// DNS rebinding: an attacker resolves their own hostname to 127.0.0.1 so the
// browser treats it as same-origin. Requiring a loopback Host defeats it.
func TestRejectsRebindingHost(t *testing.T) {
	srv, base := start(t)
	res := do(t, "GET", base+"/api/state", func(r *http.Request) {
		r.Header.Set("X-Loadout-Token", srv.token)
		r.Host = "attacker.example.com:" + srv.port
	})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestAllowedHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"localhost:8080", true},
		{"[::1]:8080", true},
		{"attacker.com:8080", false},
		{"127.0.0.1:9999", false}, // right host, wrong port
		{"127.0.0.1", false},      // no port
		{"", false},
	}
	for _, c := range cases {
		if got := allowedHost(c.host, "8080"); got != c.want {
			t.Errorf("allowedHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestAllowedOrigin(t *testing.T) {
	cases := []struct {
		origin string
		want   bool
	}{
		{"", true}, // same-origin requests may omit it
		{"http://127.0.0.1:8080", true},
		{"http://localhost:8080", true},
		{"https://evil.com", false},
		{"http://127.0.0.1:9999", false},
		{"http://127.0.0.1.evil.com:8080", false},
	}
	for _, c := range cases {
		if got := allowedOrigin(c.origin, "8080"); got != c.want {
			t.Errorf("allowedOrigin(%q) = %v, want %v", c.origin, got, c.want)
		}
	}
}

// The page load carries the token in the query string, because a navigation
// cannot set a header.
func TestTokenInQueryServesThePage(t *testing.T) {
	srv, base := start(t)
	res := do(t, "GET", base+"/?token="+srv.token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "<title>loadout</title>") {
		t.Error("index.html was not served")
	}
}

// A browser fetches app.css and app.js from <link> and <script> tags, which
// cannot set a header and whose relative URLs carry no query string. Requiring
// a token on static assets left the page unstyled and inert. Fetch them the
// way a browser does: no token, no query string, just a Referer.
func TestStaticAssetsLoadWithoutAToken(t *testing.T) {
	_, base := start(t)
	for _, asset := range []string{"/app.css", "/app.js", "/favicon.svg", "/"} {
		res := do(t, "GET", base+asset, func(r *http.Request) {
			r.Header.Set("Referer", base+"/")
		})
		if res.StatusCode != http.StatusOK {
			t.Errorf("GET %s without a token: status = %d, want 200 "+
				"(a <script> tag cannot present one)", asset, res.StatusCode)
		}
		body, _ := io.ReadAll(res.Body)
		if len(body) == 0 {
			t.Errorf("GET %s returned an empty body", asset)
		}
	}
}

// Dropping the token on static assets must not weaken the API.
func TestAPIStillRequiresATokenAfterStaticIsOpen(t *testing.T) {
	_, base := start(t)
	for _, path := range []string{"/api/state", "/api/diff"} {
		res := do(t, "GET", base+path, nil)
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s without a token: status = %d, want 401", path, res.StatusCode)
		}
	}
}

// The loopback checks must still apply to static assets, so a rebound
// hostname cannot even load the page.
func TestStaticAssetsStillCheckHost(t *testing.T) {
	srv, base := start(t)
	res := do(t, "GET", base+"/app.js", func(r *http.Request) {
		r.Host = "attacker.example.com:" + srv.port
	})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestToggleAndDiffRoundTrip(t *testing.T) {
	srv, base := start(t)

	post := func(path, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest("POST", base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Loadout-Token", srv.token)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}

	// Disabling an alias must be reflected in the next render.
	res := post("/api/toggle", `{"kind":"alias","name":"gpush","enabled":false}`)
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("toggle: status = %d, body = %s", res.StatusCode, body)
	}

	res = do(t, "GET", base+"/api/diff", func(r *http.Request) {
		r.Header.Set("X-Loadout-Token", srv.token)
	})
	var diff struct {
		Next    string `json:"next"`
		Changed bool   `json:"changed"`
		Path    string `json:"path"`
	}
	if err := json.NewDecoder(res.Body).Decode(&diff); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(diff.Next, "alias gpush=") {
		t.Error("gpush should be absent after being disabled")
	}
	if !diff.Changed {
		t.Error("diff should report a change against a nonexistent output file")
	}

	// Apply must write exactly what the diff previewed. Anything else makes
	// the preview a lie.
	res = post("/api/apply", `{}`)
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("apply: status = %d, body = %s", res.StatusCode, body)
	}
	written, err := os.ReadFile(diff.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != diff.Next {
		t.Error("the applied file does not match what the diff previewed")
	}

	// Re-enabling must restore it.
	post("/api/toggle", `{"kind":"alias","name":"gpush","enabled":true}`)
	res = do(t, "GET", base+"/api/diff", func(r *http.Request) {
		r.Header.Set("X-Loadout-Token", srv.token)
	})
	json.NewDecoder(res.Body).Decode(&diff)
	if !strings.Contains(diff.Next, "alias gpush=") {
		t.Error("gpush should be back after being re-enabled")
	}
}

func TestToggleRejectsUnknownNames(t *testing.T) {
	srv, base := start(t)
	for _, body := range []string{
		`{"kind":"alias","name":"not-a-real-alias","enabled":false}`,
		`{"kind":"pack","name":"not-a-real-pack","enabled":true}`,
		`{"kind":"nonsense","name":"x","enabled":true}`,
	} {
		req, _ := http.NewRequest("POST", base+"/api/toggle", strings.NewReader(body))
		req.Header.Set("X-Loadout-Token", srv.token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", body, res.StatusCode)
		}
	}
}

// The UI is told whether a value exists, never what it is.
func TestCredentialValuesNeverLeaveTheKeychain(t *testing.T) {
	srv, base := start(t)
	res := do(t, "GET", base+"/api/state", func(r *http.Request) {
		r.Header.Set("X-Loadout-Token", srv.token)
	})
	body, _ := io.ReadAll(res.Body)

	var state stateResponse
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Credentials) == 0 {
		t.Fatal("expected the fixture's credential to appear")
	}
	// entryView carries only a name and a boolean; assert the shape rather
	// than trusting that no value was populated.
	raw, _ := json.Marshal(state.Credentials[0].Entries[0])
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	for k := range fields {
		if k != "name" && k != "has_value" {
			t.Errorf("credential entry exposes unexpected field %q", k)
		}
	}
}

// A GET must not be able to mutate anything.
func TestMutatingEndpointsRequirePOST(t *testing.T) {
	srv, base := start(t)
	for _, path := range []string{"/api/toggle", "/api/apply", "/api/cred/use", "/api/cred/set"} {
		res := do(t, "GET", base+path, func(r *http.Request) {
			r.Header.Set("X-Loadout-Token", srv.token)
		})
		if res.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: status = %d, want 405", path, res.StatusCode)
		}
	}
}

func TestTokenIsUnguessable(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		tok, err := newToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) < 40 {
			t.Fatalf("token %q is only %d chars", tok, len(tok))
		}
		if seen[tok] {
			t.Fatal("newToken returned a duplicate")
		}
		seen[tok] = true
	}
}
