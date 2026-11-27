#compdef epa
_epa_complete() {
    local -a candidates
    candidates=("${(@f)$(epa __complete -- "${(@)words[2,CURRENT]}" 2>/dev/null)}")
    (( ${#candidates} )) && compadd -a candidates
}
compdef _epa_complete epa
