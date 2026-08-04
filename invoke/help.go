package invoke

import (
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/fredrikkvalvik/shelly/config"
)

// Help renders help for one node of the command tree.
func Help(t *config.Tool, c *config.Cmd) string {
	var b strings.Builder

	full := strings.Join(append([]string{t.Name}, c.Path...), " ")
	if c.Doc != "" {
		fmt.Fprintf(&b, "%s — %s\n\n", full, c.Doc)
	} else {
		fmt.Fprintf(&b, "%s\n\n", full)
	}

	fmt.Fprintf(&b, "usage: %s\n", usage(full, c))

	if len(c.Sub) > 0 {
		b.WriteString("\ncommands:\n")
		w := tabwriter.NewWriter(&b, 0, 0, 3, ' ', 0)
		for _, s := range c.Sub {
			fmt.Fprintf(w, "  %s\t%s\n", s.Name, summary(s.Doc))
		}
		w.Flush()
	}

	if len(c.Args) > 0 {
		b.WriteString("\narguments:\n")
		w := tabwriter.NewWriter(&b, 0, 0, 3, ' ', 0)
		for _, a := range c.Args {
			fmt.Fprintf(w, "  %s\t%s\n", a.Name, withDefault(summary(a.Doc), a.Default))
		}
		w.Flush()
	}

	if len(c.Flags) > 0 {
		b.WriteString("\nflags:\n")
		w := tabwriter.NewWriter(&b, 0, 0, 3, ' ', 0)
		for _, f := range c.Flags {
			fmt.Fprintf(w, "  %s\t%s\n", flagSpec(f), withDefault(summary(f.Doc), f.Default))
		}
		fmt.Fprintf(w, "  %s\t%s\n", "-h, --help", "show this help")
		w.Flush()
	}

	// a wrapper is only understandable if you can see what it wraps
	if len(c.Exec) > 0 {
		b.WriteString("\nruns:\n  ")
		parts := make([]string, 0, len(c.Exec))
		for _, s := range c.Exec {
			words := make([]string, 0, len(s.Argv)+1)
			words = append(words, s.Cmd)
			for _, a := range s.Argv {
				words = append(words, quoteArg(a))
			}
			parts = append(parts, strings.Join(words, " "))
		}
		b.WriteString(strings.Join(parts, " | "))
		b.WriteString("\n")
	}

	return b.String()
}

func usage(full string, c *config.Cmd) string {
	if len(c.Sub) > 0 {
		return full + " <command>"
	}

	parts := []string{full}
	if len(c.Flags) > 0 {
		parts = append(parts, "[flags]")
	}
	for _, a := range c.Args {
		switch {
		case a.Variadic && a.Required:
			parts = append(parts, "<"+a.Name+">...")
		case a.Variadic:
			parts = append(parts, "["+a.Name+"...]")
		case a.Required:
			parts = append(parts, "<"+a.Name+">")
		default:
			parts = append(parts, "["+a.Name+"]")
		}
	}
	return strings.Join(parts, " ")
}

func flagSpec(f *config.Flag) string {
	var b strings.Builder
	if f.Short != "" {
		fmt.Fprintf(&b, "-%s, ", f.Short)
	} else {
		b.WriteString("    ")
	}
	b.WriteString("--")
	b.WriteString(f.Name)
	if !f.IsBool() {
		b.WriteString(" <value>")
	}
	return b.String()
}

// a listing is one row per entry, so only the first line of a description can
// appear in it. The full text is shown in that command's own help header
func summary(doc string) string {
	line, _, _ := strings.Cut(doc, "\n")
	return strings.TrimSpace(line)
}

// an argv element can hold spaces or newlines, which would otherwise make the
// one line "runs:" summary unreadable
func quoteArg(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\n\r\"'\\") {
		return strconv.Quote(s)
	}
	return s
}

func withDefault(doc, def string) string {
	if def == "" {
		return doc
	}
	if doc == "" {
		return fmt.Sprintf("(default: %s)", def)
	}
	return fmt.Sprintf("%s (default: %s)", doc, def)
}
