#!/bin/sh
# ag-mux — o Claude Code informa o próprio ciclo de vida para a sidebar.
#
# O Claude chama este script uma vez por evento, em exec form (sem shell), com
# o estado que aquele evento significa:
#
#     ag-mux-hook.sh waiting permissão
#     ag-mux-hook.sh busy
#     ag-mux-hook.sh clear
#
# O estado vai para a opção @ag-mux-hook do próprio pane. Isso é de propósito:
# a sidebar já roda um list-panes por varredura, e ler mais um campo ali custa
# zero. Uma opção de pane também morre com o pane, então não há arquivo a
# limpar nem estado obsoleto a expirar.
#
# Nunca falha e nunca escreve na saída: um hook que erra é um harness que
# engasga. Todo caminho sai com 0.

state="${1:-}"
detail="${2:-}"

[ -n "$state" ] || exit 0
[ -n "${TMUX:-}" ] || exit 0
[ -n "${TMUX_PANE:-}" ] || exit 0

# $TMUX_PANE é o pane onde o claude roda, não o pane em foco — que é
# justamente a diferença que importa: quem está em foco costuma ser a sidebar.
if [ "$state" = "clear" ]; then
    tmux set-option -p -u -t "$TMUX_PANE" @ag-mux-hook >/dev/null 2>&1
else
    tmux set-option -p -t "$TMUX_PANE" @ag-mux-hook \
        "$state $(date +%s) $detail" >/dev/null 2>&1
fi

exit 0
