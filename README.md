# gh-auto-switch

Automatically switch GitHub CLI (`gh`) authentication user based on your current directory, so you don’t have to run `gh auth switch` manually.

## Design Options

### Option 1: Shell Hook (zsh `chpwd` / bash prompt hook)

Pros:

- No extra dependencies.
- Works everywhere you use a shell.
- Fast when implemented as `decide` (no `gh` call) + conditional `switch`.

Cons:

- Needs a small snippet in your shell rc.
- If you manually switch accounts, auto-correction depends on when the hook runs.

## Assumptions

- You are already logged into both accounts on `github.com` using `gh auth login`.
- `gh auth switch --hostname ... --user ...` is available (fallback to `-h`/`-u` is implemented).
- You have created a config file with your path rules (see **Config** below).

## Project Structure

```
cmd/gh-auto-switch/main.go
internal/cli/        CLI subcommands
internal/config/     config loading (yaml/json/toml)
internal/gh/         gh command execution + output parsing
internal/rules/      rule matching
```

## Install (macOS)

### Simple install to `~/bin`

```sh
make install PREFIX="$HOME/bin"
```

Ensure `~/bin` is on your `PATH`.

## Shell Integration

### zsh (`~/.zshrc`)

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

### bash (`~/.bashrc`)

This runs on prompt render and only triggers when `$PWD` changed.

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

## Usage

```sh
gh-auto-switch config init
gh-auto-switch status
gh-auto-switch switch
gh-auto-switch doctor
```

## Config

Config is required to define your path prefixes and the GitHub usernames to switch to.
If no config file is present, the shell hook will no-op (and `status/switch/doctor` will ask you to create one).

Supported locations:

- `$XDG_CONFIG_HOME/gh-auto-switch/config.yaml` (or `~/.config/gh-auto-switch/config.yaml` if `XDG_CONFIG_HOME` is not set; also `.yml` / `.json` / `.toml`)
- `~/.gh-auto-switch.yaml` (also `.yml` / `.json` / `.toml`)

### Create a config file

```sh
gh-auto-switch config init
```

You can also start from the repo template: `config.example.yaml`.

Other formats:

```sh
gh-auto-switch config init --format json
gh-auto-switch config init --format toml
```

Show which config is being used:

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

You can also set `GH_AUTO_SWITCH_CONFIG=/path/to/config.yaml`.

Notes:

- YAML/TOML support is intentionally minimal (just enough for the simple `rules` schema shown above).

## Notes / Edge Cases

- Outside all rule prefixes: no-op.
- Non-git directories: still switches (useful for `gh repo create`, etc.).
- GitHub Enterprise repos: set `host` per rule (for example `ghe.company.com`). This tool does not inspect git remotes.
- No secrets are printed; this tool only calls `gh auth status` and `gh auth switch`.

## Testing

```sh
make test
```

Unit tests cover:

- Path rule matching
- `gh auth list/status` parsing

Integration tests can be added by setting `GH_AUTO_SWITCH_GH` to a stub `gh` script in a temp directory (no real credentials required).
This repo already includes an integration-style test that does exactly this.
