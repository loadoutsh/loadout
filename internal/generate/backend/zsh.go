package backend

import (
	"fmt"
	"strings"
)

func init() { register(Zsh{}) }

// Zsh emits zsh syntax.
type Zsh struct{}

func (Zsh) Name() string { return "zsh" }
func (Zsh) Ext() string  { return "zsh" }

// Alias renders `alias name='command'`.
//
// The body is always single-quoted so nothing in it is expanded at definition
// time, which is what an alias needs: expansion must happen when the alias is
// used, not when it is defined. A literal single quote is closed, escaped and
// reopened, the only way to get one inside a single-quoted string.
func (Zsh) Alias(name, command string) string {
	return fmt.Sprintf("alias %s=%s", quoteAliasName(name), singleQuote(command))
}

// quoteAliasName quotes names that are not bare words. Names like `..` and
// `ctl:status` are fine unquoted in zsh, but a name containing a quote or
// whitespace is not.
func quoteAliasName(name string) string {
	if name == "" {
		return "''"
	}
	if strings.ContainsAny(name, " \t\n'\"\\$`&|;<>()") {
		return singleQuote(name)
	}
	return name
}

func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// PathAppend appends to PATH only if the directory is not already there, so
// re-sourcing the file does not grow PATH without bound.
func (Zsh) PathAppend(dir string) string {
	return fmt.Sprintf(`if [[ ":$PATH:" != *":%s:"* ]]; then
	export PATH="$PATH:%s"
fi`, dir, dir)
}

func (Zsh) Comment(text string) string {
	if text == "" {
		return "#"
	}
	return "# " + text
}

func (Zsh) SectionHeader(title string) string {
	const width = 66
	line := "# " + title + " "
	if n := width - len(line); n > 0 {
		line += strings.Repeat("=", n)
	}
	return line
}
