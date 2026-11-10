package cli

import (
	"fmt"

	"github.com/Villoh/nixup/internal/exit"
)

// The scripts only pass the words to `nixup __complete` and show what it prints, so the
// three shells offer the same candidates.
var completionScripts = map[string]string{
	"bash": `# bash completion for nixup: source <(nixup completion bash)
_nixup() {
    local cur=${COMP_WORDS[COMP_CWORD]} out IFS=$'\n'
    out=$(nixup __complete "${COMP_WORDS[@]:1:COMP_CWORD}" 2>/dev/null)
    case $out in
    :dirs) COMPREPLY=($(compgen -d -- "$cur")) ;;
    :files) COMPREPLY=($(compgen -f -- "$cur")) ;;
    *) COMPREPLY=($(compgen -W "$(cut -f1 <<<"$out")" -- "$cur")) ;;
    esac
}
complete -o default -F _nixup nixup
`,
	"zsh": `#compdef nixup
# zsh completion for nixup: put it in $fpath as _nixup, or source <(nixup completion zsh)
_nixup() {
    local -a out
    out=("${(@f)$(nixup __complete "${(@)words[2,CURRENT]}" 2>/dev/null)}")
    case $out[1] in
    :dirs) _files -/ ;;
    :files) _files ;;
    *)
        out=("${(@)out//:/\\:}")     # _describe splits value and description on ':'
        out=("${(@)out//$'\t'/:}")
        _describe -t values nixup out
        ;;
    esac
}
if [[ $funcstack[1] == _nixup ]]; then
    _nixup "$@"
else
    compdef _nixup nixup
fi
`,
	"fish": `# fish completion for nixup: nixup completion fish | source
function __nixup_complete
    set -l out (nixup __complete (commandline -opc)[2..] (commandline -ct) 2>/dev/null)
    switch "$out[1]"
        case :dirs
            __fish_complete_directories (commandline -ct)
        case :files
            __fish_complete_path (commandline -ct)
        case '*'
            printf '%s\n' $out
    end
end
complete -c nixup -f -a '(__nixup_complete)'
`,
}

func runCompletion(args []string, d Deps) int {
	if len(args) == 1 {
		if s, ok := completionScripts[args[0]]; ok {
			fmt.Fprint(d.T.Out, s)
			return exit.OK
		}
	}
	return d.fail(fmt.Errorf("usage: nixup completion bash|zsh|fish"))
}
