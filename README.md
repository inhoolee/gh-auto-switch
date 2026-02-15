# gh-auto-switch

[![CI](https://github.com/inhoolee/gh-auto-switch/actions/workflows/ci.yml/badge.svg)](https://github.com/inhoolee/gh-auto-switch/actions/workflows/ci.yml)

Automatically switch the active GitHub CLI (`gh`) account based on your current directory.

This is useful if you have multiple GitHub accounts and want `gh` to “just work” as you move between personal and work checkouts.

## How It Works

`gh` has one active account per host (for example `github.com`). This tool:

1. Matches the current directory against configured path-prefix rules.
2. Checks the currently active account via `gh auth status`.
3. Runs `gh auth switch --hostname <host> --user <user>` when a change is needed.

No secrets are stored or printed. The tool only uses official `gh` commands.

## Quickstart (macOS)

1. Install:

```sh
go install github.com/inhoolee/gh-auto-switch/cmd/gh-auto-switch@latest
```

Or from source:

```sh
make install PREFIX="$HOME/bin"
```

2. Create a config file and edit it:

```sh
gh-auto-switch config init
$EDITOR ~/.config/gh-auto-switch/config.yaml
```

3. Add the zsh hook (recommended) to `~/.zshrc`:

```sh
autoload -U add-zsh-hook

_gh_auto_switch_tick() {
  (( $+commands[gh-auto-switch] )) || return 0

  # Run only when $PWD changes.
  if [[ "$PWD" == "$GH_AUTO_SWITCH_LAST_PWD" ]]; then
    return 0
  fi
  export GH_AUTO_SWITCH_LAST_PWD="$PWD"

  local key
  key="$(gh-auto-switch decide 2>/dev/null)" || return 0

  if [[ -z "$key" ]]; then
    unset GH_AUTO_SWITCH_LAST_KEY
    return 0
  fi
  if [[ "$key" == "$GH_AUTO_SWITCH_LAST_KEY" ]]; then
    return 0
  fi

  if gh-auto-switch switch -q >/dev/null 2>&1; then
    export GH_AUTO_SWITCH_LAST_KEY="$key"
  else
    unset GH_AUTO_SWITCH_LAST_KEY
  fi
}

add-zsh-hook chpwd _gh_auto_switch_tick
add-zsh-hook precmd _gh_auto_switch_tick
_gh_auto_switch_tick
```

4. Verify:

```sh
cd ~/Workspace/some-repo
gh auth status
```

## Commands

- `gh-auto-switch status`: show the rule match for the current directory and the active `gh` account
- `gh-auto-switch switch`: switch now based on the current directory (idempotent)
- `gh-auto-switch doctor`: verify `gh` is installed, config is readable, and required accounts exist
- `gh-auto-switch decide`: print the desired `host|user` for the current directory (used by shell hooks)
- `gh-auto-switch config path|show|init`: manage config

## Config

Config is required for `status/switch/doctor`. The shell hook uses `decide`, which no-ops when unconfigured.

Locations (first match wins):

- `$XDG_CONFIG_HOME/gh-auto-switch/config.yaml` (or `~/.config/gh-auto-switch/config.yaml` if `XDG_CONFIG_HOME` is not set; also `.yml` / `.json` / `.toml`)
- `~/.gh-auto-switch.yaml` (also `.yml` / `.json` / `.toml`)

Create a config file:

```sh
gh-auto-switch config init
```

Show which config is in use:

```sh
gh-auto-switch config path
gh-auto-switch config show
```

Example `config.yaml`:

```yaml
rules:
  - prefix: ~/Documents
    user: user-personal
    host: github.com
  - prefix: ~/Workspace
    user: user-work
    host: github.com
```

Notes:

- YAML/TOML support is intentionally minimal (a simple `rules` list as above).
- `host` is optional in config; if omitted, it defaults to `github.com`.

## Bash Integration

Bash doesn’t have a `chpwd` hook, so this uses `PROMPT_COMMAND` and only runs when `$PWD` changes.

```sh
__gh_auto_switch_prev_pwd=""
__gh_auto_switch() {
  if [[ "$PWD" == "$__gh_auto_switch_prev_pwd" ]]; then
    return 0
  fi
  __gh_auto_switch_prev_pwd="$PWD"

  local key
  key="$(gh-auto-switch decide 2>/dev/null)" || return 0
  if [[ -z "$key" ]]; then
    unset GH_AUTO_SWITCH_LAST_KEY
    return 0
  fi
  if [[ "$key" == "$GH_AUTO_SWITCH_LAST_KEY" ]]; then
    return 0
  fi
  gh-auto-switch switch -q >/dev/null 2>&1 || true
  export GH_AUTO_SWITCH_LAST_KEY="$key"
}
PROMPT_COMMAND="__gh_auto_switch${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
__gh_auto_switch
```

## GitHub Enterprise / Multiple Hosts

Set `host` per rule (for example `ghe.company.com`). This tool does not inspect git remotes; it strictly follows your configured path rules.

## Troubleshooting

- `config not found`: run `gh-auto-switch config init` and edit the generated file
- `not logged in ...`: run `gh auth login --hostname <host>` for each account
- Hook debugging:
  - Run `gh-auto-switch -v switch` manually to see the `gh` commands executed

## Development

```sh
make test
make build
```

Tests include a stubbed-`gh` integration test, so they don’t require real GitHub credentials.

## License

MIT (see `LICENSE`).

