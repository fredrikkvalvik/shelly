package shellgen

import (
	"fmt"
	"strings"
)

// Unload removes tool definitions a shell no longer wants. Both the function
// and its completion go, since a stale completion would offer subcommands for
// something that is no longer callable.
func Unload(shell Shell, names []string) (string, error) {
	var b strings.Builder

	for _, name := range names {
		fn := helper(name)
		switch shell {
		case Zsh:
			fmt.Fprintf(&b, "unset -f %s %s 2>/dev/null\n", name, fn)
			fmt.Fprintf(&b, "compdef -d %s 2>/dev/null\n", name)
		case Bash:
			fmt.Fprintf(&b, "unset -f %s %s 2>/dev/null\n", name, fn)
			fmt.Fprintf(&b, "complete -r %s 2>/dev/null\n", name)
		default:
			return "", fmt.Errorf("unsupported shell %q, want one of: %s", shell, list(shells))
		}
	}

	return b.String(), nil
}

// Notice emits a message to the user. It goes to stderr so it cannot end up
// inside a command substitution that happens to be capturing the shell.
func Notice(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "printf '%%s\\n' %s >&2\n", sq(l))
	}
	return b.String()
}

// Setenv emits an assignment the shell will carry forward.
func Setenv(name, value string) string {
	return fmt.Sprintf("export %s=%s\n", name, sq(value))
}

// Hook emits the shell code that runs Export on every prompt.
//
// It has to be idempotent: sourcing a shell rc file twice, or `exec zsh`, must
// not stack the hook up.
func Hook(shell Shell, bin string) (string, error) {
	switch shell {
	case Zsh:
		return fmt.Sprintf(`_shelly_hook() {
  eval "$(%s export zsh --pid "$$")"
}
typeset -ag precmd_functions
if [[ -z ${precmd_functions[(r)_shelly_hook]} ]]; then
  precmd_functions=(_shelly_hook $precmd_functions)
fi
`, sq(bin)), nil

	case Bash:
		// PROMPT_COMMAND is kept a plain string: the array form arrived in
		// bash 5.1 and macOS ships 3.2
		return fmt.Sprintf(`_shelly_hook() {
  eval "$(%s export bash --pid "$$")"
}
case ";$PROMPT_COMMAND;" in
  *";_shelly_hook;"*) ;;
  *) PROMPT_COMMAND="_shelly_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
esac
`, sq(bin)), nil

	default:
		return "", fmt.Errorf("unsupported shell %q, want one of: %s", shell, list(shells))
	}
}
