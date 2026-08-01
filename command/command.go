package command

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

type Command struct {
	name string
	args []string
	cfg  *config

	// set by Start, cleared by Wait. only one process in flight at a time
	cmd *exec.Cmd
}

func New(name string, opts ...commandOpt) *Command {
	return newCommand(name, nil, opts...)
}

func newCommand(name string, args []string, opts ...commandOpt) *Command {
	c := &Command{
		name: name,
		args: args,
	}

	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	c.cfg = cfg

	return c
}

func prepCmd(c *Command) *exec.Cmd {
	cmd := exec.Command(c.name, c.args...)
	cmd.Stdin = c.cfg.stdin
	cmd.Stdout = c.cfg.stdout
	cmd.Stderr = c.cfg.stderr
	if c.cfg.cwd != "" {
		cmd.Dir = c.cfg.cwd
	}

	return cmd
}

func (c *Command) FlagShort(f rune, val any) *Command {
	// the value goes in its own arg, POSIX style. coreutils reject the
	// joined -n=5 form, and Go's flag package accepts either
	c.args = append(c.args, fmt.Sprintf("-%s", string(f)))
	if val != nil {
		// no shell involved, so no quoting: the arg is passed to the
		// process verbatim, spaces and all
		c.args = append(c.args, fmt.Sprintf("%v", val))
	}
	return c
}

func (c *Command) FlagShortIf(condition bool, f rune, val any) *Command {
	if condition {
		return c.FlagShort(f, val)
	}
	return c
}

func (c *Command) FlagLong(f string, val any) *Command {
	if val != nil {
		c.args = append(c.args, fmt.Sprintf("--%s=%v", f, val))
	} else {
		c.args = append(c.args, fmt.Sprintf("--%s", f))
	}
	return c
}

func (c *Command) FlagLongIf(condition bool, f string, val any) *Command {
	if condition {
		return c.FlagLong(f, val)
	}
	return c
}

// appends arg(s) to command
func (c *Command) Args(args ...string) *Command {
	c.args = append(c.args, args...)
	return c
}

// append arg(s) to command if condition is true
func (c *Command) ArgsIf(condition bool, args ...string) *Command {
	if condition {
		c.args = append(c.args, args...)
	}
	return c
}

// starts the command and waits for it to exit
func (c *Command) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

// starts the command without waiting for it to exit. every Start must be
// paired with a Wait, which releases the process resources
func (c *Command) Start() error {
	if c.cmd != nil {
		return fmt.Errorf("%s: already started", c.name)
	}

	cmd := prepCmd(c)
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd

	return nil
}

// blocks until the command started by Start exits. the command may be
// started again once Wait returns
func (c *Command) Wait() error {
	if c.cmd == nil {
		return fmt.Errorf("%s: not started", c.name)
	}

	err := c.cmd.Wait()
	c.cmd = nil

	return err
}

type config struct {
	stdin          io.Reader
	stdout, stderr io.Writer

	cwd string
}

func defaultConfig() *config {
	return &config{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

type commandOpt func(c *config)

// set a different std[in/out/err] than default
func WithIo(in io.Reader, out, err io.Writer) commandOpt {
	return func(c *config) {
		// we only overwrite if values are non-nil

		if in != nil {
			c.stdin = in
		}
		if out != nil {
			c.stdout = out
		}
		if err != nil {
			c.stderr = err
		}
	}
}

func WithCwd(cwd string) commandOpt {
	return func(c *config) {
		c.cwd = cwd
	}
}
