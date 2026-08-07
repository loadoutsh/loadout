package sync

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
)

// Finding is one suspected secret in a file about to be pushed.
type Finding struct {
	Line int
	Rule string
	Hint string
}

func (f Finding) String() string {
	return fmt.Sprintf("line %d: %s (%s)", f.Line, f.Rule, f.Hint)
}

// secretRule matches something that should never be committed.
type secretRule struct {
	name    string
	pattern *regexp.Regexp
	hint    string
}

// rules catch the recognisable shapes of live credentials.
//
// The config is supposed to contain only names and which entry is active, so
// anything matching here means a real value has been pasted in and would be
// pushed to a remote. High-signal prefixes are preferred over generic entropy
// checks, which would fire constantly on hostnames and paths.
var rules = []secretRule{
	{"GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{16,}`), "starts with ghp_/gho_/ghu_/ghs_/ghr_"},
	{"GitHub fine-grained token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), "starts with github_pat_"},
	{"npm token", regexp.MustCompile(`\bnpm_[A-Za-z0-9]{30,}`), "starts with npm_"},
	{"Slack token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`), "starts with xox"},
	{"AWS access key", regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`), "AWS key id"},
	{"OpenAI key", regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`), "starts with sk-"},
	{"Anthropic key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`), "starts with sk-ant-"},
	{"private key block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), "PEM private key"},
	{"JSON web token", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`), "JWT"},
	{"generic password field", regexp.MustCompile(`(?i)^\s*(password|passwd|secret|token|api[_-]?key)\s*:\s*\S`), "a literal value where a reference belongs"},
}

// credReference matches the only form a secret may legitimately take in the
// config: a pointer to a keychain entry.
var credReference = regexp.MustCompile(`^\s*\w+\s*:\s*["']?cred:`)

// Scan reports anything in the content that looks like a live credential.
func Scan(content string) []Finding {
	var out []Finding
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for n := 1; sc.Scan(); n++ {
		line := sc.Text()

		// A comment is still a leak if it holds a real token, so comments are
		// not skipped wholesale; only the empty case is.
		if strings.TrimSpace(line) == "" {
			continue
		}
		// "password: cred:host_password" is the correct form, not a finding.
		if credReference.MatchString(line) {
			continue
		}

		for _, r := range rules {
			if r.pattern.MatchString(line) {
				out = append(out, Finding{Line: n, Rule: r.name, Hint: r.hint})
				break // one finding per line is enough to stop the push
			}
		}
	}
	return out
}
