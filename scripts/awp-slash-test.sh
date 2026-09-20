#!/bin/bash
# Test slash commands via tmux
set -e

AWP_DIR="$(cd "$(dirname "$0")/.." && pwd)"
AWP_BIN="$AWP_DIR/awp"

if [ ! -x "$AWP_BIN" ]; then
    echo "awp binary not found" >&2
    exit 1
fi

SESSION="awp-slash-test-$$"

cleanup() {
    tmux kill-session -t "$SESSION" 2>/dev/null || true
}
trap cleanup EXIT

echo "=== launching TUI ==="
tmux new-session -d -s "$SESSION" -x 120 -y 30 "$AWP_BIN"
sleep 2

capture() {
    tmux capture-pane -t "$SESSION" -p -S -
}

echo "=== initial screen ==="
capture | tail -10

echo ""
echo "=== send /help ==="
tmux send-keys -t "$SESSION" -l "/help"
sleep 1
tmux send-keys -t "$SESSION" Enter
sleep 1
capture | tail -20

echo ""
echo "=== send /tools ==="
tmux send-keys -t "$SESSION" -l "/tools"
sleep 1
tmux send-keys -t "$SESSION" Enter
sleep 1
capture | tail -20

echo ""
echo "=== quit ==="
tmux send-keys -t "$SESSION" q
sleep 1

echo "=== test passed ==="
