# EPA dynamic completions for Fish 3.x and 4.x.
function __epa_complete
    set -l words (commandline -xpc 2>/dev/null; or commandline -opc)
    if set -q words[1]
        set -e words[1]
    end
    set -l prefix (commandline -ct)
    if not set -q prefix[1]
        set prefix ''
    end
    command epa __complete -- $words "$prefix" 2>/dev/null
end
complete -c epa -f -a '(__epa_complete)'
