#!/bin/bash
set -e

AWP_DIR="$(cd "$(dirname "$0")/.." && pwd)"
AWP_BIN="$AWP_DIR/awp"
SOCKET="${AWP_SOCKET:-$HOME/.awp/runtime/awp.sock}"

if [ ! -x "$AWP_BIN" ]; then
    echo "awp binary not found, run 'make build' first" >&2
    exit 1
fi

SESSION="awp-test-$$"
PROMPT_TEXT="${1:-list the files in the current directory}"

cleanup() {
    tmux kill-session -t "$SESSION" 2>/dev/null || true
}
trap cleanup EXIT

echo "=== launching awp TUI in tmux session: $SESSION ==="
tmux new-session -d -s "$SESSION" -x 100 -y 30 "$AWP_BIN"

sleep 2

if ! tmux capture-pane -t "$SESSION" -p | grep -q "awp"; then
    echo "FAIL: TUI did not start within 2s" >&2
    tmux capture-pane -t "$SESSION" -p >&2
    exit 1
fi

echo "=== TUI started, capturing screen ==="
tmux capture-pane -t "$SESSION" -p

echo ""
echo "=== sending prompt: '$PROMPT_TEXT' ==="
tmux send-keys -t "$SESSION" -l "$PROMPT_TEXT"
sleep 1
tmux send-keys -t "$SESSION" Enter

echo "=== waiting 8s for streaming events ==="
sleep 8

tmux capture-pane -t "$SESSION" -p

echo ""
echo "=== quitting (q) ==="
tmux send-keys -t "$SESSION" q
sleep 1

echo "=== final capture ==="
tmux capture-pane -t "$SESSION" -p

echo ""
echo "=== test passed ==="
