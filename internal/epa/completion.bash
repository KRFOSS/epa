_epa_complete() {
    local candidate
    COMPREPLY=()
    while IFS= read -r candidate; do
        [[ -n "$candidate" ]] && COMPREPLY+=("$candidate")
    done < <(epa __complete -- "${COMP_WORDS[@]:1:COMP_CWORD}" 2>/dev/null)
}
complete -F _epa_complete epa
