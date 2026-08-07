package config

import (
	"strings"
	"testing"
)

// The derived name is the default, and it is what most credentials want.
func TestDeriveEnvName(t *testing.T) {
	cases := map[string]string{
		"npm_token":     "NPM_TOKEN",
		"gh_token":      "GH_TOKEN",
		"my-token":      "MY_TOKEN",
		"a.b":           "A_B",
		"a/b":           "A_B",
		"host_password": "HOST_PASSWORD",
	}
	for in, want := range cases {
		if got := DeriveEnvName(in); got != want {
			t.Errorf("DeriveEnvName(%q) = %q, want %q", in, got, want)
		}
	}
}

// Deriving the variable name from the key was fine until a tool expected a
// name the key does not produce. gh_token cannot derive GHP_TOKEN.
func TestEnvOverride(t *testing.T) {
	plain := Credential{}
	if got := plain.EnvName("gh_token"); got != "GH_TOKEN" {
		t.Errorf("without an override = %q, want GH_TOKEN", got)
	}

	overridden := Credential{Env: "GHP_TOKEN"}
	if got := overridden.EnvName("gh_token"); got != "GHP_TOKEN" {
		t.Errorf("with an override = %q, want GHP_TOKEN", got)
	}
}

// Two credentials exporting one variable means one silently overwrites the
// other, and which one wins depends on map iteration order.
func TestClashingEnvNamesAreRejected(t *testing.T) {
	cfg := Default()
	cfg.Credentials = map[string]Credential{
		"gh_token":  {Env: "TOKEN", Entries: []string{"a"}, Active: "a"},
		"npm_token": {Env: "TOKEN", Entries: []string{"a"}, Active: "a"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("two credentials exporting TOKEN should be rejected")
	}
	// The message has to name the variable and point at the fix, or the user
	// is left guessing which of their credentials is the problem.
	for _, want := range []string{"TOKEN", "env"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

// A clash between a derived name and an explicit one must be caught too, not
// just two explicit ones.
func TestDerivedAndOverriddenClashIsRejected(t *testing.T) {
	cfg := Default()
	cfg.Credentials = map[string]Credential{
		"gh_token": {Entries: []string{"a"}, Active: "a"},
		"other":    {Env: "GH_TOKEN", Entries: []string{"a"}, Active: "a"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("a derived name clashing with an explicit one should be rejected")
	}
}

func TestDistinctEnvNamesPass(t *testing.T) {
	cfg := Default()
	cfg.Credentials = map[string]Credential{
		"gh_token":  {Env: "GHP_TOKEN", Entries: []string{"a"}, Active: "a"},
		"npm_token": {Entries: []string{"a"}, Active: "a"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("distinct names should validate, got: %v", err)
	}
}
