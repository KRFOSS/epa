#!/usr/bin/env bash
# Run against a native EPA binary, with disposable account configuration.
set -euo pipefail
binary=$(realpath "${1:?native EPA binary required}")
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
export HOME="$work/home with spaces"
export XDG_CONFIG_HOME="$HOME/.config"
unset ZDOTDIR SUDO_USER SUDO_COMMAND
mkdir -m 700 -p "$HOME" "$work/bin"
ln -s "$binary" "$work/bin/epa"
export PATH="$work/bin:$PATH"
export TEST_EPA_HOME="$HOME"
export TEST_EPA_BIN="$work/bin"

printf '%s\n' '# existing Bash configuration' > "$HOME/.bashrc"
# Ensure the test bin directory survives PATH initialization on login.
printf '%s\n' '# existing login configuration' 'export PATH="$TEST_EPA_BIN:$PATH"' > "$HOME/.profile"
mkdir "$work/insecure-zsh"
chmod 0777 "$work/insecure-zsh"
printf '%s\n' '#compdef epa-unsafe-fixture' ':' > "$work/insecure-zsh/_epa_unsafe_fixture"
export TEST_EPA_INSECURE_FPATH="$work/insecure-zsh"
printf '%s\n' '# existing Zsh configuration' 'fpath=("$TEST_EPA_INSECURE_FPATH" $fpath)' > "$HOME/.zshrc"
epa completion install bash
epa completion install zsh
cp "$HOME/.bashrc" "$work/bashrc"
cp "$HOME/.profile" "$work/profile"
cp "$HOME/.zshrc" "$work/zshrc"
epa completion install bash
epa completion install zsh
cmp "$HOME/.bashrc" "$work/bashrc"
cmp "$HOME/.profile" "$work/profile"
cmp "$HOME/.zshrc" "$work/zshrc"
test ! -e "$HOME/.bash_profile"
# .profile must still parse as POSIX shell.
sh -n "$HOME/.profile"

bash_check='set -e
[[ $HOME == "$TEST_EPA_HOME" ]] || exit 1
complete -p epa | grep -F "_epa_complete"
COMP_WORDS=(epa st)
COMP_CWORD=1
_epa_complete
printf "%s\n" "${COMPREPLY[@]}" | grep -Fx style'
bash --noprofile -ic "$bash_check"
# Exercise the real login startup path without touching the real account.
bash -lic "$bash_check"
# Noninteractive Bash does not register the completion.
bash --noprofile --norc -c '! complete -p epa >/dev/null 2>&1'

if command -v zsh >/dev/null 2>&1; then
    zsh -d -ic '
        [[ "$HOME" == "$TEST_EPA_HOME" ]] || exit 1
        [[ "${_comps[epa]}" == _epa_complete ]] || exit 1
        [[ -z "${_comps[epa-unsafe-fixture]-}" ]] || exit 1
        compadd() { print -rl -- "${candidates[@]}"; }
        words=(epa st)
        CURRENT=2
        _epa_complete
    ' | grep -Fx style
    export ZDOTDIR="$work/zdotdir"
    mkdir -m 700 "$ZDOTDIR"
    epa completion install zsh
    zsh -d -ic '[[ "${_comps[epa]}" == _epa_complete ]]'
else
    echo 'Zsh unavailable: skipping local Zsh runtime test' >&2
fi

if command -v fish >/dev/null 2>&1; then
    epa completion install fish
    fish_loader="$XDG_CONFIG_HOME/fish/completions/epa.fish"
    test -f "$fish_loader"
    cp "$fish_loader" "$work/fish-loader"
    epa completion install fish
    cmp "$fish_loader" "$work/fish-loader"
    epa completion fish | fish -n
    # Each fresh Fish process autoloads the account-local completion file.
    fish -c 'complete -C "epa st"' | cut -f1 | grep -Fx style
    fish -c 'complete -C "epa --style pacman -Sy"' | cut -f1 | grep -Fx -- -Syu
    fish -c 'complete -C "epa completion install f"' | cut -f1 | grep -Fx fish
    fish -c 'complete -C "epa install bas"' | cut -f1 | grep -Fx bash
    printf '%s\n' '# custom fish completion' > "$fish_loader"
    if epa completion install fish; then
        echo 'custom Fish completion was unexpectedly overwritten' >&2
        exit 1
    fi
    grep -Fx '# custom fish completion' "$fish_loader"
else
    echo 'Fish unavailable: skipping local Fish runtime test' >&2
fi

# The loader must respect a custom existing completion.
printf '%s\n' 'complete -W custom epa' > "$HOME/.bashrc"
epa completion install bash
bash --noprofile -ic 'complete -p epa | grep -F custom'
echo 'Persistent completion startup tests passed'
