package main

import "testing"

func TestEnvName(t *testing.T) {
	cases := map[string]string{
		"npm_token": "NPM_TOKEN",
		"gh_token":  "GH_TOKEN",
		"my-token":  "MY_TOKEN",
		"a.b":       "A_B",
		"a/b":       "A_B",
	}
	for in, want := range cases {
		if got := envName(in); got != want {
			t.Errorf("envName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The output of `loadout env` is eval'd, so a value containing shell
// metacharacters must not be able to execute anything.
func TestShellQuoteIsInjectionSafe(t *testing.T) {
	cases := map[string]string{
		"plain":            `'plain'`,
		"with space":       `'with space'`,
		"it's":             `'it'\''s'`,
		"$(rm -rf /)":      `'$(rm -rf /)'`,
		"`whoami`":         "'`whoami`'",
		"a\nb":             "'a\nb'",
		`"double"`:         `'"double"'`,
		"semi; echo pwned": `'semi; echo pwned'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}
