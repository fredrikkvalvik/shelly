// shelly turns tools already on PATH into command hierarchies.
//
// A TOML file declares one tool: dotted table headers are command paths, and
// each command names the programs it runs and how its args and flags map onto
// their argv. Nothing is handed to a shell, so values reach the wrapped
// program verbatim.
package main

import (
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fredrikkvalvik/shelly/command"
	"github.com/fredrikkvalvik/shelly/config"
	"github.com/fredrikkvalvik/shelly/invoke"
	"github.com/fredrikkvalvik/shelly/project"
	"github.com/fredrikkvalvik/shelly/shellgen"
)

const usage = `shelly turns tools on PATH into command hierarchies.

usage:
  shelly run --config FILE -- ARGS...   run a tool (what the shell functions call)
  shelly init [zsh|bash]                emit shell code for every configured tool
  shelly load [--shell SH] FILE...      emit shell code for specific configs
  shelly complete --config FILE ...     completion candidates, called on TAB
  shelly check [FILE...]                validate configs and report name collisions
  shelly hook [zsh|bash]                emit the prompt hook for project tools
  shelly export [zsh|bash] --pid N      what the shell must load or drop here

put this in your shell rc file:
  eval "$(shelly init zsh)"

or define a tool for the current shell only:
  eval "$(shelly load ./tools/ffzf.toml)"

configs live in %s
`

// carries a wrapped program's exit status without printing anything, since it
// has already written its own diagnostics to stderr
type exitStatus int

func (e exitStatus) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

func main() {
	if err := run(os.Args[1:]); err != nil {
		if status, ok := errors.AsType[exitStatus](err); ok {
			os.Exit(int(status))
		}
		fmt.Fprintln(os.Stderr, "shelly: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Printf(usage, configDir())
		return nil
	}

	switch args[0] {
	case "run":
		return cmdRun(args[1:])
	case "init":
		return cmdInit(args[1:])
	case "load":
		return cmdLoad(args[1:])
	case "complete":
		return cmdComplete(args[1:])
	case "check":
		return cmdCheck(args[1:])
	case "hook":
		return cmdHook(args[1:])
	case "export":
		return cmdExport(args[1:])
	case "help", "-h", "--help":
		fmt.Printf(usage, configDir())
		return nil
	default:
		return fmt.Errorf("unknown command %q, see `shelly help`", args[0])
	}
}

func cmdRun(args []string) error {
	fs := newFlagSet("run")
	cfg := fs.String("config", "", "path to the tool's config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cfg == "" {
		return errors.New("run: --config is required")
	}

	tool, err := config.Load(*cfg)
	if err != nil {
		return err
	}

	inv, err := invoke.Parse(tool, fs.Args())
	if err != nil {
		return err
	}
	if inv.Help {
		fmt.Print(invoke.Help(tool, inv.Cmd))
		return nil
	}

	stages, err := invoke.Resolve(inv)
	if err != nil {
		return err
	}

	cmds := make([]*command.Command, 0, len(stages))
	for _, s := range stages {
		cmds = append(cmds, command.New(s.Cmd).Args(s.Argv...))
	}

	err = command.Pipe(cmds[0], cmds[1:]...)
	if pe, ok := errors.AsType[*command.PipeError](err); ok {
		// a shell takes the pipeline's status from its last stage, so an
		// upstream program dying on a broken pipe is not a failure
		return statusOf(pe.Last())
	}
	return err
}

func cmdInit(args []string) error {
	shell, err := shellArg(args)
	if err != nil {
		return err
	}

	dir := configDir()
	paths, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil {
		return err
	}

	// only the filenames are read here. parsing every config would put the
	// cost of the whole config directory on every shell startup
	tools := make([]shellgen.Tool, 0, len(paths))
	for _, p := range paths {
		base := filepath.Base(p)
		tools = append(tools, shellgen.Tool{
			Name: strings.TrimSuffix(base, filepath.Ext(base)),
			File: p,
		})
	}

	return emit(shell, tools)
}

func cmdLoad(args []string) error {
	fs := newFlagSet("load")
	name := fs.String("shell", "", "zsh or bash, defaults to $SHELL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("load: need at least one config file")
	}

	shell := detectShell()
	if *name != "" {
		var err error
		if shell, err = shellgen.NewShell(*name); err != nil {
			return err
		}
	}

	tools := make([]shellgen.Tool, 0, fs.NArg())
	for _, p := range fs.Args() {
		// unlike init this parses, because a session local config is usually
		// one the user is still editing and wants to hear about
		tool, err := config.Load(p)
		if err != nil {
			return err
		}
		tools = append(tools, shellgen.Tool{Name: tool.Name, File: tool.File})
	}

	return emit(shell, tools)
}

func cmdComplete(args []string) error {
	fs := newFlagSet("complete")
	cfg := fs.String("config", "", "path to the tool's config")
	cursor := fs.String("cursor", "0", "index of the word being completed")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// completion runs on every TAB, so it never reports errors: a broken
	// config simply completes to nothing
	tool, err := config.Load(*cfg)
	if err != nil {
		return nil
	}
	at, err := strconv.Atoi(*cursor)
	if err != nil {
		return nil
	}

	c := invoke.Complete(tool, fs.Args(), at)
	if c.Directive != "" {
		fmt.Println(":" + c.Directive)
		return nil
	}
	for _, cand := range c.Candidates {
		if cand.Doc == "" {
			fmt.Println(cand.Value)
			continue
		}
		fmt.Printf("%s\t%s\n", cand.Value, cand.Doc)
	}
	return nil
}

func cmdCheck(args []string) error {
	paths := args
	if len(paths) == 0 {
		var err error
		if paths, err = filepath.Glob(filepath.Join(configDir(), "*.toml")); err != nil {
			return err
		}
	}
	if len(paths) == 0 {
		fmt.Printf("no configs in %s\n", configDir())
		return nil
	}

	bad := 0
	for _, p := range paths {
		tool, err := config.Load(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			bad++
			continue
		}

		fmt.Printf("ok  %s (%d commands)\n", tool.Name, count(tool.Root))

		// shadowing is allowed, but silently redefining rg is worth knowing
		if p, err := exec.LookPath(tool.Name); err == nil {
			fmt.Printf("    note: shadows %s\n", p)
		}
		for _, s := range missing(tool.Root) {
			fmt.Printf("    note: %s is not on PATH\n", s)
		}
	}

	if bad > 0 {
		return fmt.Errorf("%d of %d configs failed to load", bad, len(paths))
	}
	return nil
}

// the shell carries what it has loaded, so shelly stays stateless per prompt
const (
	envLoaded = "SHELLY_LOADED"     // newline separated absolute config paths
	envPID    = "SHELLY_LOADED_PID" // the shell those definitions live in
	envSaid   = "SHELLY_SAID"       // digest of the last notices, so they are not repeated
)

// a short digest, only ever compared against itself
func digest(lines []string) string {
	h := fnv.New32a()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{0})
	}
	return strconv.FormatUint(uint64(h.Sum32()), 16)
}

func cmdHook(args []string) error {
	shell, err := shellArg(args)
	if err != nil {
		return err
	}
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate the shelly binary: %w", err)
	}

	out, err := shellgen.Hook(shell, bin)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

func cmdExport(args []string) error {
	// the shell name comes out first: flag.Parse stops at the first non-flag
	// argument, and the hook emits `export zsh --pid "$$"`
	name, rest := splitShell(args)

	fs := newFlagSet("export")
	pid := fs.String("pid", "", "the calling shell's $$, so a nested shell is not believed")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if name == "" {
		name = fs.Arg(0) // also accept `export --pid N zsh`
	}
	shell, err := shellArg([]string{name})
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// a nested shell inherits the variable but not the functions, so anything
	// it claims to have loaded is a lie
	var loaded []string
	if *pid != "" && os.Getenv(envPID) == *pid {
		loaded = strings.Split(os.Getenv(envLoaded), "\n")
	}

	plan := project.Build(
		globalTools(),
		project.Discover(cwd, home),
		loaded,
		// the trust store lands next; until then nothing discovered may run
		func(d project.Dir) (bool, string) {
			return false, fmt.Sprintf("shelly: %s is not trusted, %s not loaded",
				d.Path, plural(len(d.Tools), "tool"))
		},
	)

	var b strings.Builder

	// a notice is worth saying when it changes, not on every prompt: standing
	// in an untrusted directory must not scold you once per command
	said := digest(plan.Notices)
	newNotice := *pid == "" || os.Getenv(envSaid) != said
	if newNotice {
		b.WriteString(shellgen.Notice(plan.Notices))
	}

	if len(plan.Unload) > 0 {
		out, err := shellgen.Unload(shell, plan.Unload)
		if err != nil {
			return err
		}
		b.WriteString(out)
	}
	if len(plan.Load) > 0 {
		bin, err := os.Executable()
		if err != nil {
			return err
		}
		tools := make([]shellgen.Tool, 0, len(plan.Load))
		for _, t := range plan.Load {
			tools = append(tools, shellgen.Tool{Name: t.Name, File: t.File})
		}
		out, err := shellgen.Emit(shell, bin, tools)
		if err != nil {
			return err
		}
		b.WriteString(out)
	}

	// only touch the environment when something actually moved, so standing
	// still costs the shell nothing
	if len(plan.Unload) > 0 || len(plan.Load) > 0 || newNotice {
		b.WriteString(shellgen.Setenv(envLoaded, strings.Join(plan.Loaded, "\n")))
		b.WriteString(shellgen.Setenv(envSaid, said))
		if *pid != "" {
			b.WriteString(shellgen.Setenv(envPID, *pid))
		}
	}

	fmt.Print(b.String())
	return nil
}

// the tools in the config directory, which are always available and never
// need trusting: they are your own, not something a directory brought with it
func globalTools() []project.Tool {
	paths, err := filepath.Glob(filepath.Join(configDir(), "*.toml"))
	if err != nil {
		return nil
	}
	tools := make([]project.Tool, 0, len(paths))
	for _, p := range paths {
		base := filepath.Base(p)
		tools = append(tools, project.Tool{
			Name: strings.TrimSuffix(base, filepath.Ext(base)),
			File: p,
		})
	}
	return tools
}

// splitShell takes a leading shell name off an argument list, so that flags
// after it still reach flag.Parse
func splitShell(args []string) (string, []string) {
	if len(args) > 0 && args[0] != "" && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func shellArg(args []string) (shellgen.Shell, error) {
	if len(args) > 0 && args[0] != "" {
		return shellgen.NewShell(args[0])
	}
	return detectShell(), nil
}

func emit(shell shellgen.Shell, tools []shellgen.Tool) error {
	bin, err := os.Executable()
	if err != nil {
		// without an absolute path the emitted functions would depend on PATH
		return fmt.Errorf("cannot locate the shelly binary: %w", err)
	}

	out, err := shellgen.Emit(shell, bin, tools)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func statusOf(err error) error {
	if err == nil {
		return nil
	}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitStatus(ee.ExitCode())
	}
	return err
}

func detectShell() shellgen.Shell {
	if s, err := shellgen.NewShell(filepath.Base(os.Getenv("SHELL"))); err == nil {
		return s
	}
	return shellgen.Zsh
}

func configDir() string {
	if d := os.Getenv("SHELLY_CONFIG_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "shelly")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config/shelly"
	}
	return filepath.Join(home, ".config", "shelly")
}

func count(c *config.Cmd) int {
	n := 0
	if !c.IsGroup() {
		n++
	}
	for _, s := range c.Sub {
		n += count(s)
	}
	return n
}

// every program the tool wants to run that cannot be found
func missing(c *config.Cmd) []string {
	seen := map[string]bool{}
	var out []string

	var walk func(*config.Cmd)
	walk = func(c *config.Cmd) {
		for _, s := range c.Exec {
			if seen[s.Cmd] {
				continue
			}
			seen[s.Cmd] = true
			if _, err := exec.LookPath(s.Cmd); err != nil {
				out = append(out, s.Cmd)
			}
		}
		for _, s := range c.Sub {
			walk(s)
		}
	}
	walk(c)

	return out
}
