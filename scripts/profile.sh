#!/usr/bin/env bash
# Capture concurrency-focused pprof profiles under load. Output: build/profile/
#   scripts/profile.sh                       build + run locally, profile, shut down
#   scripts/profile.sh docker                docker compose up --build, profile the container
#   scripts/profile.sh url <pprof-url> [app-url]   attach to an already-running server
# Env: DUR=cpu profile seconds (15)  CONC=concurrent clients (50)  REQS=total requests (20000)
set -euo pipefail
cd "$(dirname "$0")/.."
MODE=${1:-local}
OUT=build/profile
DUR=${DUR:-15} CONC=${CONC:-50} REQS=${REQS:-20000}
APP=http://localhost:8080 PPROF=http://localhost:6060
rm -rf "$OUT" && mkdir -p "$OUT"

# Background jobs in a non-interactive shell ignore SIGINT, so Ctrl+C would leave
# the load generator and snapshot jobs running. cleanup kills them, then the
# server (local mode) and waits for its graceful exit.
PID= LOADPID= SNAPPID=
cleanup() {
  [ -n "$LOADPID$SNAPPID" ] && { kill $LOADPID $SNAPPID 2>/dev/null; pkill -P "${SNAPPID:-0}" 2>/dev/null; } || true
  [ -n "$PID" ] || return 0
  kill -TERM "$PID" 2>/dev/null
  wait "$PID" && echo "server shut down cleanly (exit 0)" || echo "server exit $?"
}
trap cleanup EXIT

case $MODE in
  local)
    go build -o build/omnic ./cmd/omnic
    build/omnic -addr :8080 -pprof localhost:6060 >"$OUT/server.log" 2>&1 & PID=$! ;;
  docker) docker compose up -d --build --wait; APP=http://localhost:8090 ;;
  url) PPROF=$2 APP=${3:-} ;;
  *) echo "usage: $0 [local|docker|url <pprof-url> [app-url]]" >&2; exit 2 ;;
esac

for _ in $(seq 40); do curl -sf "$PPROF/debug/pprof/" >/dev/null && break; sleep 0.5; done
curl -sf "$PPROF/debug/pprof/" >/dev/null || { echo "pprof not reachable at $PPROF" >&2; exit 1; }

curl -s "$PPROF/debug/pprof/goroutine?debug=1" >"$OUT/goroutine-before.txt"

if [ -n "$APP" ]; then
  echo "load: $CONC clients x $REQS reqs -> $APP/healthz"
  # No wrapping subshell: $! must be xargs itself so cleanup can stop it (a
  # subshell defers signals until its foreground child exits).
  seq "$REQS" | xargs -P "$CONC" -I{} curl -s -o /dev/null "$APP/healthz" & LOADPID=$!
fi
echo "cpu profile ${DUR}s..."
( sleep $((DUR / 2))  # mid-load snapshots
  curl -s "$PPROF/debug/pprof/goroutine?debug=1" >"$OUT/goroutine-during.txt"
  curl -s "$PPROF/debug/pprof/goroutine?debug=2" >"$OUT/goroutine-full.txt"
  for p in goroutine heap allocs block mutex threadcreate; do curl -s "$PPROF/debug/pprof/$p" >"$OUT/$p.pprof"; done ) & SNAPPID=$!
curl -s "$PPROF/debug/pprof/profile?seconds=$DUR" >"$OUT/cpu.pprof"
wait "$SNAPPID"
[ -n "$LOADPID" ] && wait "$LOADPID" || true
sleep 1
curl -s "$PPROF/debug/pprof/goroutine?debug=1" >"$OUT/goroutine-after.txt"
# Go 1.27+: goroutines blocked on primitives the GC proved unreachable. Taken after load so
# transient per-request goroutines are gone and only true leaks remain.
curl -s "$PPROF/debug/pprof/goroutineleak?debug=1" >"$OUT/goroutineleak.txt"

top() { go tool pprof -top -nodecount=20 "$@" 2>/dev/null | grep -v '^File:\|^Type:\|^Time:\|^Build' ; }
{
  echo "# pprof summary  (mode=$MODE pprof=$PPROF conc=$CONC reqs=$REQS cpu=${DUR}s)"
  echo; echo "## goroutines: before / during / after load"
  for f in before during after; do printf '%s: ' "$f"; head -1 "$OUT/goroutine-$f.txt"; done
  echo; echo "## goroutineleak (blocked forever, unreachable; want 0)"; head -40 "$OUT/goroutineleak.txt"
  echo; echo "## block (time goroutines spent blocked on sync/chan/select)"; top "$OUT/block.pprof"
  echo; echo "## mutex (contended lock holders)"; top "$OUT/mutex.pprof"
  echo; echo "## cpu"; top "$OUT/cpu.pprof"
  echo; echo "## heap inuse_space"; top -sample_index=inuse_space "$OUT/heap.pprof"
  echo; echo "## allocs alloc_space"; top -sample_index=alloc_space "$OUT/allocs.pprof"
  echo; echo "## goroutine stacks during load (grouped)"; head -60 "$OUT/goroutine-during.txt"
} >"$OUT/summary.txt"
echo "wrote $OUT/summary.txt (raw .pprof files alongside; open with: go tool pprof -http=: $OUT/<name>.pprof)"
