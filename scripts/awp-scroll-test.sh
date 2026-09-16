#!/bin/bash
set -e

AWP_BIN="/home/leroy/Project/agentic-with-pi/awp"
SESSION="awp-scroll-test-$$"

cleanup() {
    tmux kill-session -t "$SESSION" 2>/dev/null || true
}
trap cleanup EXIT

tmux new-session -d -s "$SESSION" -x 120 -y 25 "$AWP_BIN"
sleep 2

capture() {
    tmux capture-pane -t "$SESSION" -p -S -
}

tmux send-keys -t "$SESSION" -l "write a 30-line poem"
sleep 1
tmux send-keys -t "$SESSION" Enter
sleep 25
echo "=== after streaming ==="
capture | tail -15

# PgUp multiple times
tmux send-keys -t "$SESSION" PgUp PgUp PgUp PgUp PgUp
sleep 2
echo ""
echo "=== after 5x PgUp ==="
capture | tail -15

# 'g' to top
tmux send-keys -t "$SESSION" g
sleep 1
echo ""
echo "=== after g ==="
capture | tail -10

# End to bottom
tmux send-keys -t "$SESSION" End
sleep 1
echo ""
echo "=== after End ==="
capture | tail -10

tmux send-keys -t "$SESSION" -l "/quit"
sleep 1
tmux send-keys -t "$SESSION" Enter
sleep 1
echo ""
echo "=== test passed ==="
