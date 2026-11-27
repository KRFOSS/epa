# EPA — Everyone's Package Adapter

**Your commands. Any distribution.**

EPA is an experimental Linux package-manager interface written in Go. It uses the host's existing package manager instead of replacing its database, repository configuration, package formats, signature verification or dependency solver.

EPA never translates package names. Use the name provided by the host distribution, for example `apache2` on Debian and `httpd` on RHEL.

## Build

Go 1.23 or later; Linux. No third-party Go dependencies.

```sh
CGO_ENABLED=0 go build -trimpath -o epa ./cmd/epa
sudo install -m 0755 epa /usr/local/bin/epa
```

## Usage

```sh
epa style list
epa style set apt
epa style current
epa update
epa install httpd -y       # RHEL/Fedora example
epa --dry-run install httpd -y
epa style set pacman
epa -S nginx
epa --style apt search nginx
epa backend info
epa doctor
```

The default style is APT. The selected style belongs to the current account and is stored in `$XDG_CONFIG_HOME/epa/config.json` (default: `~/.config/epa/config.json`). `epa style reset` restores APT. Use `--style` before the command to pin its meaning in scripts.

Run EPA directly as your normal user; write operations elevate only the native package manager through sudo. If invoking EPA itself through sudo, specify `--style` explicitly. Style changes via sudo are rejected. Root accounts can configure their own style when logged in directly.

## Unified terminal output

EPA uses inline progress for package-changing commands on an interactive
Linux terminal. APT, DNF/DNF5 and Pacman share the same renderer. Recognized
native progress records are translated to a spinner and a width-aware bar
with real percentages. Only explicit completion records or the native
process's exit status are used to mark success; unknown progress is not
estimated.

```sh
epa upgrade -y
epa --progress tty install nginx
epa --progress plain upgrade
epa --progress json update
epa --progress native install nginx
epa config set progress auto
epa config get progress
```

| Mode | Behavior |
|---|---|
| auto | Interactive writes use the inline renderer; non-terminal writes use plain output. Read-only queries retain native output. |
| tty | Inline animated progress, falling back to plain output when a suitable terminal is unavailable. |
| plain | Nonanimated output with recognized progress on a terminal; lifecycle messages and native streams otherwise. |
| json | Newline-delimited lifecycle events on stdout, native output/prompts on stderr. |
| native | Original backend streams and exit status, without the renderer. |
| quiet | Suppresses EPA progress and recognized native progress records for interactive transactions. Unrecognized output, prompts and errors are retained; redirected streams and queries remain unfiltered. |

The mode is saved per account; `--progress` overrides it for one invocation.
A Linux pseudo-terminal preserves the controlling terminal and input used by
sudo, package managers and maintainer scripts. Unrecognized messages, partial
prompts, warnings and errors pass through; complex terminal control sequences
switch the remainder of the operation to native display. No automatic yes or
noninteractive setting is added by the renderer. Terminal resizing is forwarded
to the backend. Only the currently observed progress item is animated, rather
than maintaining an unbounded list of packages. Set `NO_COLOR=1` to disable
EPA colors.

This is an initial conservative CLI-output adapter, not a complete
implementation of every native progress callback. Progress coverage depends
on backend version, locale and output format; unsupported records remain
visible as native text. DNF5 multi-line output may trigger passthrough.
Use native mode for unusual interactive programs and unsupported terminal
setups. JSON is a lifecycle stream, not a structured package listing.

## Shell completion

Bash (add to your Bash startup configuration or source once):

```sh
source <(epa completion bash)
```

Zsh (after initializing completion with compinit):

```zsh
autoload -Uz compinit
compinit
source <(epa completion zsh)
```

Completion suggests commands/options for the active style and package names from the actual native backend. Install/search/info use available names; remove/reinstall use installed names. Package queries use existing local metadata, have a two-second timeout and do not ask for root access. Missing or inaccessible caches produce no package suggestions.

## Initial supported subset

| Style | Common operations |
|---|---|
| APT | update, upgrade, full-upgrade, install, remove, search, show, list, outdated, autoremove, reinstall, clean |
| DNF | makecache, update/upgrade, install, remove/erase, search, info, list, check-update, autoremove, reinstall, clean all |
| Pacman | -S, -Ss, -Si, -Sy, -Su, -Syu, -Syyu, -R, -Rs, -Rn, -Rns, -Q, -Qu, -Sc |

Backends: APT, DNF (including DNF5 command candidates), Pacman. Detection uses os-release and fixed system executable paths. APK, Zypper, legacy YUM and other package managers are not yet supported.

This is a deliberately limited, pre-release subset, not a drop-in reimplementation of all native commands. Unknown commands/options are rejected; version selectors, files, URLs, repository-qualified names, globs, groups and native option passthrough are outside the initial scope. APT-style list is an installed-package query by default; use list --upgradable for updates. Native solver policies and native output/exit statuses still apply. DNF check-update may return 100 when updates exist.

## Safety and known semantic differences

- APT-style update refreshes metadata, while DNF-style update requests a package upgrade.
- EPA refuses standalone refresh on Pacman and refuses APT-style upgrade on Pacman because it cannot guarantee the APT no-removal semantics. Use explicit `epa --style pacman -Syu` or `epa full-upgrade` in APT style. Maintain a fully upgraded Arch system before installing individual packages; EPA does not refresh metadata alone.
- Pacman-style -Syu on APT performs apt-get update followed by a conservative APT upgrade, not dist-upgrade. The first command must succeed before the second runs. Forced refresh across backends is not supported.
- APT-style DNF upgrade disables obsoletes but DNF native transaction rules (including install-only package limits) still apply. DNF removal disables automatic orphan removal for APT style but may remove dependent packages according to native policy. Pacman-style removal on DNF is rejected rather than broadening its semantics.
- Targeted upgrades across styles/backends and cache-cleaning operations without safe equivalents are rejected. Native dependency and replacement policies can differ; the adapter does not promise identical transactions on different distributions.
- No package-name mapping, signature-check bypass, arbitrary native flag forwarding or shell evaluation.
- Dry-run prints the planned native argv; it does not resolve dependencies or simulate transactions. Always review native prompts, especially for removals or upgrades. -y and --noconfirm request native noninteractive confirmation.
- Explicit style is recommended for automation. Package names, version selection, metadata and locks remain governed by the native manager.
- The development version includes CLI translation, completion and a conservative unified terminal renderer. Native integration testing on DNF/Pacman, progress format coverage and broader option coverage are still required before production use.

## Development

```sh
go test ./...
go vet ./...
go test -c -o /tmp/epa-pty.test ./internal/epa
python3 tests/pty_integration.py /tmp/epa-pty.test
go test ./internal/epa -fuzz=FuzzParse -fuzztime=10s
```

References: [APT manual](https://manpages.debian.org/bookworm/apt/apt-get.8.en.html), [DNF command reference](https://dnf.readthedocs.io/en/latest/command_ref.html), [Pacman manual](https://man.archlinux.org/man/pacman.8), [Arch maintenance policy](https://wiki.archlinux.org/title/System_maintenance#Partial_upgrades_are_unsupported).

Licensed under GPL-3.0; see [LICENSE](LICENSE).
