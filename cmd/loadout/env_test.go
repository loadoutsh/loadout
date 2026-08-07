package main

import (
	"os"
	"testing"
)

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

// `eval "$(loadout env)"` captures stdout only. Cobra's Print helpers fall
// back to stderr when the root has no out writer, which silently sent every
// export to stderr and made the command a no-op under eval.
func TestRootWritesToStdout(t *testing.T) {
	root := newRootCmd()
	if root.OutOrStderr() != os.Stdout {
		t.Error("cmd.Print* would write to stderr; exports from `loadout env` would be lost under eval")
	}
}
