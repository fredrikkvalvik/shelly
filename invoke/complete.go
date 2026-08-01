package invoke

import (
	"bytes"
	"io"
	"strings"

	"github.com/fredrikkvalvik/shelly/command"
	"github.com/fredrikkvalvik/shelly/config"
)

type Candidate struct {
	Value string
	Doc   string
}

// Directive asks the shell to fall back to its own completion, since Go has no
// business reimplementing file completion.
type Completion struct {
	Directive  string // "files", "dirs", or empty
	Candidates []Candidate
}

// Complete returns candidates for words[cursor]. words includes the tool name
// at index 0, and cursor is a zero based index into words.
func Complete(t *config.Tool, words []string, cursor int) Completion {
	if cursor <= 0 || len(words) == 0 {
		return Completion{}
	}
	if cursor > len(words) {
		cursor = len(words)
	}

	partial := ""
	if cursor < len(words) {
		partial = words[cursor]
	}
	typed := words[1:cursor]

	cmd := t.Root
	i := 0
	for i < len(typed) && len(cmd.Sub) > 0 {
		sub := cmd.Lookup(typed[i])
		if sub == nil {
			break
		}
		cmd = sub
		i++
	}

	if len(cmd.Sub) > 0 {
		return Completion{Candidates: subCandidates(cmd, partial)}
	}

	rest := typed[i:]

	// mid flag: "--max <TAB>" completes the flag's value, not a new flag
	if len(rest) > 0 {
		if f := pendingValue(cmd, rest[len(rest)-1]); f != nil {
			return values(f.Complete, partial)
		}
	}

	if name, val, ok := strings.Cut(partial, "="); ok && strings.HasPrefix(name, "--") {
		if f := cmd.Flag(strings.TrimPrefix(name, "--")); f != nil && !f.IsBool() {
			c := values(f.Complete, val)
			for i := range c.Candidates {
				c.Candidates[i].Value = name + "=" + c.Candidates[i].Value
			}
			return c
		}
	}

	if strings.HasPrefix(partial, "-") {
		return Completion{Candidates: flagCandidates(cmd, partial)}
	}

	if a := nthArg(cmd, positionals(cmd, rest)); a != nil {
		return values(a.Complete, partial)
	}
	return Completion{}
}

func subCandidates(c *config.Cmd, partial string) []Candidate {
	var out []Candidate
	for _, s := range c.Sub {
		if strings.HasPrefix(s.Name, partial) {
			out = append(out, Candidate{Value: s.Name, Doc: s.Doc})
		}
	}
	return out
}

func flagCandidates(c *config.Cmd, partial string) []Candidate {
	all := make([]Candidate, 0, len(c.Flags)*2+1)
	for _, f := range c.Flags {
		all = append(all, Candidate{Value: "--" + f.Name, Doc: f.Doc})
		if f.Short != "" {
			all = append(all, Candidate{Value: "-" + f.Short, Doc: f.Doc})
		}
	}
	all = append(all, Candidate{Value: "--help", Doc: "show this help"})

	var out []Candidate
	for _, cand := range all {
		if strings.HasPrefix(cand.Value, partial) {
			out = append(out, cand)
		}
	}
	return out
}

// the flag whose value the next word supplies, if any
func pendingValue(c *config.Cmd, word string) *config.Flag {
	var f *config.Flag
	switch {
	case strings.HasPrefix(word, "--"):
		if strings.Contains(word, "=") {
			return nil
		}
		f = c.Flag(word[2:])
	case strings.HasPrefix(word, "-") && len([]rune(word)) == 2:
		f = c.FlagShort(word[1:])
	default:
		return nil
	}
	if f == nil || f.IsBool() {
		return nil
	}
	return f
}

// how many positional args have already been given
func positionals(c *config.Cmd, rest []string) int {
	n := 0
	for i := 0; i < len(rest); i++ {
		w := rest[i]
		if w == "--" {
			n += len(rest) - i - 1
			break
		}
		if strings.HasPrefix(w, "-") && w != "-" {
			if f := pendingValue(c, w); f != nil {
				i++ // it consumes the next word
			}
			continue
		}
		n++
	}
	return n
}

func nthArg(c *config.Cmd, n int) *config.Arg {
	if n < len(c.Args) {
		return c.Args[n]
	}
	if len(c.Args) > 0 {
		if last := c.Args[len(c.Args)-1]; last.Variadic {
			return last
		}
	}
	return nil
}

func values(src config.Complete, partial string) Completion {
	switch {
	case src.Builtin != "":
		return Completion{Directive: src.Builtin}
	case len(src.Argv) == 0:
		return Completion{}
	}

	var buf bytes.Buffer
	c := command.New(src.Argv[0], command.WithIo(nil, &buf, io.Discard)).Args(src.Argv[1:]...)
	if err := c.Run(); err != nil {
		return Completion{} // a broken source completes to nothing, silently
	}

	var out []Candidate
	for line := range strings.Lines(buf.String()) {
		value, doc, _ := strings.Cut(strings.TrimRight(line, "\n"), "\t")
		if value == "" || !strings.HasPrefix(value, partial) {
			continue
		}
		out = append(out, Candidate{Value: value, Doc: doc})
	}
	return Completion{Candidates: out}
}
