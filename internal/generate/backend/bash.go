package backend

import (
	"fmt"
	"strings"
)

func init() { register(Bash{}) }

// Bash emits bash syntax.
//
// Almost everything loadout generates is identical between bash and zsh:
// alias definitions, function definitions, PATH manipulation and comments all
// share a syntax. The divergence that matters is not in these primitives but
// in the function bodies, which is why the templates avoid zsh-only
// constructs rather than being written twice.
//
// What genuinely differs is captured in Preamble.
type Bash struct{}

func (Bash) Name() string { return "bash" }
func (Bash) Ext() string  { return "bash" }

// Alias renders `alias name='command'`, identical to zsh.
func (Bash) Alias(name, command string) string {
	return fmt.Sprintf("alias %s=%s", quoteAliasName(name), singleQuote(command))
}

func (Bash) PathAppend(dir string) string {
	return fmt.Sprintf(`if [[ ":$PATH:" != *":%s:"* ]]; then
	export PATH="$PATH:%s"
fi`, dir, dir)
}

func (Bash) Comment(text string) string {
	if text == "" {
		return "#"
	}
	return "# " + text
}

func (Bash) SectionHeader(title string) string {
	const width = 66
	line := "# " + title + " "
	if n := width - len(line); n > 0 {
		line += strings.Repeat("=", n)
	}
	return line
}

// Preamble is emitted once at the top of the generated file.
//
// bash only expands aliases in interactive shells unless told otherwise, so a
// script that sources this file would silently get the functions and none of
// the aliases. zsh needs no equivalent, which is why this exists on the
// interface at all.
func (Bash) Preamble() string {
	return `# bash expands aliases only in interactive shells by default, so a
# non-interactive script sourcing this file would get the functions and
# silently none of the aliases.
shopt -s expand_aliases`
}

// Preamble for zsh is empty: nothing needs enabling.
func (Zsh) Preamble() string { return "" }
