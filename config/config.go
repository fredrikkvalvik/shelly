// Package config loads tool definitions from TOML.
//
// A file defines exactly one tool. Dotted table headers are command paths, so
// [cmd.db.shell] declares the command "db shell". A command table holds the
// reserved keys doc/arg/flag/exec; every other key in it is a subcommand.
package config

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// reserved keys inside a command table. every other key is a subcommand
var reserved = []string{"doc", "arg", "flag", "exec"}

type Tool struct {
	Name string
	Doc  string
	File string // absolute path to the config file
	Root *Cmd   // synthetic node; its Sub holds the top level commands
}

type Cmd struct {
	Name  string
	Path  []string // full path from the tool root, e.g. ["db", "shell"]
	Doc   string
	Args  []*Arg
	Flags []*Flag
	Exec  []*Stage
	Sub   []*Cmd
}

// a command with no exec only prints help
func (c *Cmd) IsGroup() bool { return len(c.Exec) == 0 }

func (c *Cmd) Lookup(name string) *Cmd {
	for _, s := range c.Sub {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (c *Cmd) Arg(name string) *Arg {
	for _, a := range c.Args {
		if a.Name == name {
			return a
		}
	}
	return nil
}

func (c *Cmd) Flag(name string) *Flag {
	for _, f := range c.Flags {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func (c *Cmd) FlagShort(short string) *Flag {
	for _, f := range c.Flags {
		if f.Short != "" && f.Short == short {
			return f
		}
	}
	return nil
}

type Arg struct {
	Name     string
	Doc      string
	Default  string
	Required bool
	Variadic bool
	Complete Complete
}

type Flag struct {
	Name     string
	Short    string
	Doc      string
	Type     FlagType
	Pass     string // literal token emitted ahead of the value
	Default  string
	Complete Complete
}

func (f *Flag) IsBool() bool { return f.Type == FlagBool }

// one program in a pipeline
type Stage struct {
	Cmd  string
	Argv []string
}

// where completion candidates come from: a builtin or a program to run
type Complete struct {
	Builtin Builtin
	Argv    []string // program plus arguments, one candidate per output line
}

func (c Complete) Empty() bool { return c.Builtin == "" && len(c.Argv) == 0 }

// Load reads and validates a tool definition.
func Load(path string) (*Tool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	md, err := toml.DecodeFile(abs, &raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	l := &loader{order: make(map[string]int, len(md.Keys()))}
	for i, k := range md.Keys() {
		l.order[strings.Join(k, "\x00")] = i
	}

	t, err := l.tool(abs, raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := validate(t); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}

type loader struct {
	order map[string]int // document order, keyed by NUL joined key path
}

func (l *loader) tool(abs string, raw map[string]any) (*Tool, error) {
	base := filepath.Base(abs)
	fromFile := strings.TrimSuffix(base, filepath.Ext(base))
	t := &Tool{File: abs, Name: fromFile}

	var root ctx
	for _, k := range l.sorted(root, raw) {
		kc := root.at(k)
		var err error
		switch k {
		case "name":
			// the filename stays authoritative so `shelly init` can list a
			// directory instead of parsing every config at shell startup
			var name string
			name, err = asString(kc, raw[k])
			if err == nil && name != fromFile {
				err = kc.errf("is %q but the file is named %q, they must match", name, base)
			}
		case "doc":
			t.Doc, err = asString(kc, raw[k])
		case "cmd":
			// handled below, once name is known
		default:
			err = kc.errf("unknown key %q", k)
		}
		if err != nil {
			return nil, err
		}
	}

	t.Root = &Cmd{Name: t.Name, Doc: t.Doc}

	if v, ok := raw["cmd"]; ok {
		c := root.at("cmd")
		tbl, err := asTable(c, v)
		if err != nil {
			return nil, err
		}
		for _, name := range l.sorted(c, tbl) {
			sub, err := l.cmd(c.at(name), name, []string{name}, tbl[name])
			if err != nil {
				return nil, err
			}
			t.Root.Sub = append(t.Root.Sub, sub)
		}
	}

	return t, nil
}

func (l *loader) cmd(c ctx, name string, path []string, v any) (*Cmd, error) {
	tbl, err := asTable(c, v)
	if err != nil {
		return nil, err
	}

	cmd := &Cmd{Name: name, Path: path}
	for _, k := range l.sorted(c, tbl) {
		kc := c.at(k)

		// a subcommand may not be called doc/arg/flag/exec, and the resulting
		// type error would be baffling, so say what actually went wrong
		if _, isTable := tbl[k].(map[string]any); isTable && slices.Contains(reserved, k) {
			return nil, kc.errf("%q is a reserved key and cannot name a subcommand", k)
		}

		switch k {
		case "doc":
			cmd.Doc, err = asString(kc, tbl[k])
		case "arg":
			cmd.Args, err = l.args(kc, tbl[k])
		case "flag":
			cmd.Flags, err = l.flags(kc, tbl[k])
		case "exec":
			cmd.Exec, err = l.exec(kc, tbl[k])
		default:
			// every non-reserved key is a subcommand, so a typo like "exce"
			// lands here. say that rather than complaining about its type
			if _, ok := tbl[k].(map[string]any); !ok {
				return nil, kc.errf("unknown key %q (a subcommand must be a table)", k)
			}
			var sub *Cmd
			sub, err = l.cmd(kc, k, append(slices.Clone(path), k), tbl[k])
			if err == nil {
				cmd.Sub = append(cmd.Sub, sub)
			}
		}
		if err != nil {
			return nil, err
		}
	}

	return cmd, nil
}

func (l *loader) args(c ctx, v any) ([]*Arg, error) {
	items, err := asTables(c, v)
	if err != nil {
		return nil, err
	}

	args := make([]*Arg, 0, len(items))
	for i, it := range items {
		ic := c.index(i)
		a := &Arg{}
		for _, k := range l.sorted(ic, it) {
			kc := ic.at(k)
			switch k {
			case "name":
				a.Name, err = asString(kc, it[k])
			case "doc":
				a.Doc, err = asString(kc, it[k])
			case "default":
				a.Default, err = asString(kc, it[k])
			case "required":
				a.Required, err = asBool(kc, it[k])
			case "variadic":
				a.Variadic, err = asBool(kc, it[k])
			case "complete":
				a.Complete, err = l.complete(kc, it[k])
			default:
				err = kc.errf("unknown key %q", k)
			}
			if err != nil {
				return nil, err
			}
		}
		if a.Name == "" {
			return nil, ic.errf(`missing "name"`)
		}
		args = append(args, a)
	}
	return args, nil
}

func (l *loader) flags(c ctx, v any) ([]*Flag, error) {
	items, err := asTables(c, v)
	if err != nil {
		return nil, err
	}

	flags := make([]*Flag, 0, len(items))
	for i, it := range items {
		ic := c.index(i)
		f := &Flag{Type: FlagString}
		for _, k := range l.sorted(ic, it) {
			kc := ic.at(k)
			switch k {
			case "name":
				f.Name, err = asString(kc, it[k])
			case "short":
				f.Short, err = asString(kc, it[k])
			case "doc":
				f.Doc, err = asString(kc, it[k])
			case "type":
				var s string
				if s, err = asString(kc, it[k]); err == nil {
					if f.Type, err = NewFlagType(s); err != nil {
						err = kc.errf("%s", err)
					}
				}
			case "pass":
				f.Pass, err = asString(kc, it[k])
			case "default":
				f.Default, err = asString(kc, it[k])
			case "complete":
				f.Complete, err = l.complete(kc, it[k])
			default:
				err = kc.errf("unknown key %q", k)
			}
			if err != nil {
				return nil, err
			}
		}
		if f.Name == "" {
			return nil, ic.errf(`missing "name"`)
		}
		flags = append(flags, f)
	}
	return flags, nil
}

func (l *loader) exec(c ctx, v any) ([]*Stage, error) {
	items, err := asTables(c, v)
	if err != nil {
		return nil, err
	}

	stages := make([]*Stage, 0, len(items))
	for i, it := range items {
		ic := c.index(i)
		s := &Stage{}
		for _, k := range l.sorted(ic, it) {
			kc := ic.at(k)
			switch k {
			case "cmd":
				s.Cmd, err = asString(kc, it[k])
			case "argv":
				s.Argv, err = asStrings(kc, it[k])
			default:
				err = kc.errf("unknown key %q", k)
			}
			if err != nil {
				return nil, err
			}
		}
		if s.Cmd == "" {
			return nil, ic.errf(`missing "cmd"`)
		}
		stages = append(stages, s)
	}
	return stages, nil
}

// complete is either a builtin name or an argv list
func (l *loader) complete(c ctx, v any) (Complete, error) {
	if s, ok := v.(string); ok {
		b, err := NewBuiltin(s)
		if err != nil {
			return Complete{}, c.errf("%s", err)
		}
		return Complete{Builtin: b}, nil
	}

	argv, err := asStrings(c, v)
	if err != nil {
		return Complete{}, c.errf(`want a builtin name or an argv list, got %s`, kindOf(v))
	}
	if len(argv) == 0 {
		return Complete{}, c.errf("argv list is empty")
	}
	return Complete{Argv: argv}, nil
}

// sorted returns a table's keys in the order they appear in the document
func (l *loader) sorted(c ctx, tbl map[string]any) []string {
	keys := make([]string, 0, len(tbl))
	for k := range tbl {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		ra, rb := l.rank(c, a), l.rank(c, b)
		if ra != rb {
			return ra - rb
		}
		return strings.Compare(a, b)
	})
	return keys
}

func (l *loader) rank(c ctx, key string) int {
	if i, ok := l.order[strings.Join(append(slices.Clone(c.key), key), "\x00")]; ok {
		return i
	}
	return math.MaxInt
}

// ctx tracks the current key path so errors can point at it
type ctx struct{ key []string }

func (c ctx) at(k string) ctx { return ctx{key: append(slices.Clone(c.key), k)} }

// index turns cmd.search.arg into cmd.search.arg[0]
func (c ctx) index(i int) ctx {
	k := slices.Clone(c.key)
	if len(k) > 0 {
		k[len(k)-1] = fmt.Sprintf("%s[%d]", k[len(k)-1], i)
	}
	return ctx{key: k}
}

func (c ctx) String() string { return strings.Join(c.key, ".") }

func (c ctx) errf(format string, a ...any) error {
	msg := fmt.Sprintf(format, a...)
	if len(c.key) == 0 {
		return fmt.Errorf("%s", msg)
	}
	return fmt.Errorf("%s: %s", c, msg)
}

func asString(c ctx, v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", c.errf("want a string, got %s", kindOf(v))
	}
	return s, nil
}

func asBool(c ctx, v any) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, c.errf("want a boolean, got %s", kindOf(v))
	}
	return b, nil
}

func asTable(c ctx, v any) (map[string]any, error) {
	t, ok := v.(map[string]any)
	if !ok {
		return nil, c.errf("want a table, got %s", kindOf(v))
	}
	return t, nil
}

func asStrings(c ctx, v any) ([]string, error) {
	items, ok := v.([]any)
	if !ok {
		return nil, c.errf("want a list of strings, got %s", kindOf(v))
	}
	out := make([]string, 0, len(items))
	for i, it := range items {
		s, err := asString(c.index(i), it)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func asTables(c ctx, v any) ([]map[string]any, error) {
	switch items := v.(type) {
	case []map[string]any:
		return items, nil
	case []any:
		out := make([]map[string]any, 0, len(items))
		for i, it := range items {
			t, err := asTable(c.index(i), it)
			if err != nil {
				return nil, err
			}
			out = append(out, t)
		}
		return out, nil
	default:
		return nil, c.errf("want a list of tables, got %s", kindOf(v))
	}
}

func kindOf(v any) string {
	switch v.(type) {
	case nil:
		return "nothing"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case int64, float64:
		return "a number"
	case map[string]any:
		return "a table"
	case []any, []map[string]any:
		return "a list"
	default:
		return fmt.Sprintf("%T", v)
	}
}
