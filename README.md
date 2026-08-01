# shelly

Turn tools already on your `PATH` into command hierarchies.

You know the incantation. You just don't remember it. `shelly` lets you write it
down once, give it a name, and get help and tab completion for free:

```console
$ ffzf search '*.go'          # -> rg --files --glob '*.go' | fzf --height 40% ...
$ ffzf db shell               # -> psql -h localhost -U dev app
```

**This is not a CLI framework.** It will not replace Cobra, and it deliberately
cannot express everything. It is for structuring the aliases and shell functions
you already have, with enough arg and flag logic to control how they are called.

## Setup

```sh
go build -o bin/shelly .          # or: mise run build
```

Add to `~/.zshrc` or `~/.bashrc`:

```sh
eval "$(shelly init zsh)"         # defines every tool in ~/.config/shelly/
```

Or define one for the current shell only, without installing it anywhere:

```sh
eval "$(shelly load ./tools/ffzf.toml)"
```

Both go through the same emitter; only the set of configs differs. Each tool
becomes a shell function with its config path baked in, so editing the TOML
takes effect on the next call with no reload.

Shell functions do not cross into child processes, so `xargs ffzf` or a
`#!/bin/bash` script will not see the name. `shelly run --config FILE -- ...`
always works and is the form scripts use.

## A first tool

`~/.config/shelly/ffzf.toml` — the filename is the tool name.

```toml
doc = "fzf wrappers"

[cmd.search]
doc  = "fuzzy-pick a file matching a glob"
arg  = [{ name = "glob", default = "*", complete = "files" }]
flag = [{ name = "hidden", short = "H", type = "bool", pass = "--hidden" }]
exec = [
  { cmd = "rg",  argv = ["--files", "$hidden", "--glob", "$glob"] },
  { cmd = "fzf", argv = ["--height", "40%"] },
]

[cmd.db]
doc = "database helpers"

[cmd.db.shell]
exec = [{ cmd = "psql", argv = ["-h", "localhost", "-U", "dev", "app"] }]
```

Every key is documented in [`docs/reference.toml`](docs/reference.toml), which
is a working config the test suite loads.

## The model

Dotted table headers are command paths, so `[cmd.db.shell]` declares
`ffzf db shell` with the whole path visible on one line.

`arg` and `flag` declare the interface users see. `exec` is a pipeline: entries
run concurrently and each pipes into the next. A command with no `exec` is a
**group** and prints help; a command with subcommands may not also have `exec`,
since `db shell` would then be ambiguous between a subcommand and a positional.

### Substitution

`arg` and `flag` share one mechanism. **Position in `argv` decides both which
stage a value lands on and where in that stage it appears**, so there is no
"which program does this flag belong to" key.

| in `argv` | when set | when unset |
|---|---|---|
| `"$glob"` — arg | `["*.go"]` | its `default`, else the element is dropped |
| `"$hidden"` — bool flag | `["--hidden"]` (its `pass`) | dropped |
| `"$max"` — string flag | `["--max-count", "5"]` (`pass`, value) | its `default`, else dropped |
| `"--sep=$sep"` — embedded | `["--sep=;"]`, one element | dropped |
| `"$files..."` — variadic | every value | dropped |
| `"$$HOME"` | a literal `$HOME` | — |

An unset value drops its whole element rather than leaving an empty string
behind. Write `${name}` when a bare `$name` would swallow what follows: names
may contain hyphens, so `$x-post` reads as one name called `x-post`.

### Nothing reaches a shell

Every element becomes one argv element, passed straight to `exec`. A value
holding a space, a glob, `$(...)` or a `;` arrives verbatim:

```console
$ ffzf search '$(rm -rf ~)'
rg --files --glob '$(rm -rf ~)'      # one argument, uninterpreted
```

There is no quoting to get right because there is no shell to quote for. This
is the property the whole design exists to preserve.

## What it deliberately cannot do

No redirection, no `&&`, no `||`, no environment prefixes, no `sh -c` escape
hatch. If a pipeline of programs cannot express it, it is out of scope — an
escape hatch would immediately become the path of least resistance and configs
would decay into a pile of shell snippets.

Also: no variables, conditionals or loops in config. Flags are `bool` or
`string` only. No inherited flags, no hooks, no passthrough — the declared
interface is the whole interface, which is what makes help and completion
exhaustive by construction.

One limit worth naming: nothing stops you writing
`{ cmd = "sh", argv = ["-c", "echo $x"] }`. shelly will not introduce a shell,
but it cannot stop you invoking one, and a `$ref` embedded in that script
string is back to being shell-interpreted.

## Commands

| | |
|---|---|
| `shelly run --config FILE -- ARGS...` | run a tool; what the shell functions call |
| `shelly init [zsh\|bash]` | emit shell code for every configured tool |
| `shelly load [--shell SH] FILE...` | emit shell code for specific configs |
| `shelly complete --config FILE ...` | completion candidates, called on TAB |
| `shelly check [FILE...]` | validate configs, report shadowing and missing programs |

Configs live in `$SHELLY_CONFIG_DIR`, else `$XDG_CONFIG_HOME/shelly`, else
`~/.config/shelly`.

Completion delegates back to `shelly` on every TAB rather than being baked into
the shell at startup. That keeps `init` to a directory listing instead of a
parse of every config, and means completions can never go stale.

## Development

```sh
mise run build     # -> bin/shelly
mise run test
mise run check     # gofmt, vet, test — the pre-commit gate
```
