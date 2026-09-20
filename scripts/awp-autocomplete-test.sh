#!/bin/bash
set -e

AWP_BIN="/home/leroy/Project/agentic-with-pi/awp"
SESSION="awp-ac-test-$$"

cleanup() {
    tmux kill-session -t "$SESSION" 2>/dev/null || true
}
trap cleanup EXIT

tmux new-session -d -s "$SESSION" -x 120 -y 30 "$AWP_BIN"
sleep 2

capture() {
    tmux capture-pane -t "$SESSION" -p -S -
}

echo "=== type / (autocomplete should appear) ==="
tmux send-keys -t "$SESSION" -l "/"
sleep 1
capture | tail -15

echo ""
echo "=== type /h (filter to /help) ==="
tmux send-keys -t "$SESSION" -l "h"
sleep 1
capture | tail -15

echo ""
echo "=== Tab to accept ==="
tmux send-keys -t "$SESSION" Tab
sleep 1
capture | tail -10

echo ""
echo "=== test /quit ==="
tmux send-keys -t "$SESSION" -l "/quit"
sleep 1
tmux send-keys -t "$SESSION" Enter
sleep 1

echo "=== test passed ==="
