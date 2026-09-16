#!/usr/bin/env bash
# Spins up a throwaway tmux session with fake agents in known states, so the
# sidebar can be exercised without waiting for real agents to be busy.
#
# The fakes are real enough for detection: each pane prints a captured screen
# from a real harness and then parks on a process that is not a shell, which is
# exactly what the two liveness checks look for.
set -euo pipefail

SESSION="${AG_MUX_DEMO_SESSION:-ag-mux-demo}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$ROOT/bin/ag-mux"
DATA="$ROOT/internal/agent/testdata"

[ -x "$BIN" ] || { echo "falta compilar: make build" >&2; exit 1; }

tmux kill-session -t "$SESSION" 2>/dev/null || true
tmux new-session -d -s "$SESSION" -x 200 -y 50 -n sidebar

# Each fake gets its own directory, because the sidebar names agents after
# their working directory — running them all from one would hide exactly the
# case where that naming needs help.
WORKDIRS="$(mktemp -d "${TMPDIR:-/tmp}/ag-mux-demo.XXXXXX")"

# fake <window> <fixture> <pane title> [dir]
fake() {
    local win="$1" fixture="$2" title="$3" dir="${4:-$1}" pane
    mkdir -p "$WORKDIRS/$dir"
    pane=$(tmux new-window -d -t "$SESSION" -n "$win" -c "$WORKDIRS/$dir" -P -F '#{pane_id}' \
        "cat $(printf %q "$DATA/$fixture"); exec tail -f /dev/null")
    tmux select-pane -t "$pane" -T "$title"
}

fake atlas   claude-busy.txt  '✳ ajustar exportacao'
fake farol   claude-draft.txt '✳ implementar filtros'
fake pomar   omp-busy.txt     'π ⠸ escrever testes de integracao'
# Two agents on one repo, to show how the list tells them apart.
fake plan    claude-idle.txt  '✳ mapear configuracoes'                orbita
fake review  claude-idle-done-clock.txt '✳ revisar estrutura'         orbita
# A harness that exited and left its title behind: it must NOT be listed.
fake morto   omp-dead-scrollback.txt 'π - pomar'

tmux send-keys -t "$SESSION:sidebar" \
    "clear; echo 'sandbox ag-mux — prefix+a abre a sidebar, q fecha'" C-m
tmux run-shell "$ROOT/ag-mux.tmux"

cat <<TXT
sessão de demo pronta: $SESSION

  6 panes falsos: 5 agentes vivos + 1 título obsoleto (janela "morto",
  que NÃO deve aparecer na lista). Dois deles ("plan" e "review")
  compartilham diretório, pra mostrar como a lista os diferencia.

  diretórios temporários em: $WORKDIRS

entre com:
  tmux switch-client -t $SESSION     (de dentro do tmux)
  tmux attach -t $SESSION            (de fora)

dentro dela: prefix+a abre a sidebar.
quando terminar:
  tmux kill-session -t $SESSION
  rm -rf $WORKDIRS
TXT
