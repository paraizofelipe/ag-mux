#!/usr/bin/env bash
# tmux plugin entrypoint. Sourced from ~/.tmux.conf via run-shell, or picked up
# automatically by TPM.
set -euo pipefail

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$CURRENT_DIR/bin/ag-mux"

# Hooks are installed at a high array index so they sit alongside anything else
# bound to the same event instead of replacing it, and so removing ours leaves
# the rest intact.
HOOK_SLOT=99

opt() {
    local value
    value="$(tmux show-option -gqv "$1")"
    if [ -n "$value" ]; then echo "$value"; else echo "$2"; fi
}

if [ ! -x "$BIN" ]; then
    tmux display-message "ag-mux: binário ausente — rode 'make build' em $CURRENT_DIR"
    exit 0
fi

tmux bind-key "$(opt "@ag-mux-key" "a")" run-shell "$BIN toggle"

# The sidebar is a single pane, and a tmux pane lives in exactly one window, so
# "visible everywhere" means moving it to the window you switch to.
FOLLOW_CMD="run-shell \"'$BIN' follow -window '#{window_id}'\""
for event in session-window-changed client-session-changed; do
    if [ "$(opt "@ag-mux-follow" "on")" = "on" ]; then
        tmux set-hook -g "${event}[${HOOK_SLOT}]" "$FOLLOW_CMD"
    else
        tmux set-hook -gu "${event}[${HOOK_SLOT}]" 2>/dev/null || true
    fi
done

# A window holding only the sidebar should not survive: closing the last real
# pane takes the window with it, as it would without a sidebar there.
REAP_CMD="run-shell \"'$BIN' reap\""
for event in pane-exited after-kill-pane; do
    tmux set-hook -g "${event}[${HOOK_SLOT}]" "$REAP_CMD"
done

# Clean up the per-window hook installed by @ag-mux-auto, which earlier
# versions used and the following sidebar replaces.
if tmux show-hooks -g after-new-window 2>/dev/null | grep -q ag-mux; then
    tmux set-hook -gu after-new-window
fi
