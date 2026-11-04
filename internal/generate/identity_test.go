package generate

import (
	"testing"

	"github.com/loadoutsh/loadout/internal/config"
)

// TestSwitchIdentityAgent runs the generated `s` with git, ssh-add and
// loadout stubbed, and checks what it does rather than what it looks like.
//
// An identity with an ssh_key must reset the agent and load that key. One
// without must leave the agent alone: its key is chosen by ~/.ssh/config,
// often from a different agent, and `ssh-add -D` would empty whichever agent
// $SSH_AUTH_SOCK happens to name.
func TestSwitchIdentityAgent(t *testing.T) {
	cfg := loadFixture(t)
	keyed := findIdentity(t, cfg, "default")
	agentless := findIdentity(t, cfg, "agentless")
	if keyed.SSHKey == "" || agentless.SSHKey != "" {
		t.Fatal("fixture no longer has one identity with an ssh_key and one without")
	}
	stubs := []string{"git", "ssh-add", "loadout"}

	for _, shell := range testShells {
		t.Run(shell, func(t *testing.T) {
			script := writeScript(t, cfg, shell)

			r := runFunction(t, shell, script, stubs, nil, "s", keyed.ID)
			if r.err != nil {
				t.Fatalf("s %s: %v\n%s", keyed.ID, r.err, r.out)
			}
			if !hasLine(r.calls, "ssh-add -D") {
				t.Errorf("s %s did not reset the agent; calls:\n%s", keyed.ID, r.calls)
			}
			if !hasLine(r.calls, "ssh-add "+r.home+"/.ssh/id_rsa") {
				t.Errorf("s %s did not load its key; calls:\n%s", keyed.ID, r.calls)
			}

			r = runFunction(t, shell, script, stubs, nil, "s", agentless.ID)
			if r.err != nil {
				t.Fatalf("s %s: %v\n%s", agentless.ID, r.err, r.out)
			}
			if got := r.callsTo("ssh-add"); len(got) > 0 {
				t.Errorf("s %s touched the agent: %q", agentless.ID, got)
			}
			if !hasLine(r.calls, "git config --global user.email "+agentless.Email) {
				t.Errorf("s %s did not set the git author; calls:\n%s", agentless.ID, r.calls)
			}
		})
	}
}

func findIdentity(t *testing.T, cfg *config.Config, id string) config.Identity {
	t.Helper()
	for _, ident := range cfg.Identities {
		if ident.ID == id {
			return ident
		}
	}
	t.Fatalf("fixture has no identity %q", id)
	return config.Identity{}
}
