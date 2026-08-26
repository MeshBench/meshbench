#!/bin/bash
# Wave 1A spike: can CI drive the real binary under Xvfb with software GL?
#
# The question is not whether it starts. It is whether the control socket
# answers, because that is the whole interface a test harness has.
set -u
cd ~/Documents/projects/meshbench
go build -o /tmp/msim-headless ./cmd/meshbench || exit 1

export XDG_RUNTIME_DIR=/tmp/hlrun
mkdir -p "$XDG_RUNTIME_DIR"
rm -f "$XDG_RUNTIME_DIR/meshbench.sock"

# llvmpipe: no GPU on a CI runner, and the GPU paths all have CPU twins anyway.
export LIBGL_ALWAYS_SOFTWARE=1
export GALLIUM_DRIVER=llvmpipe
unset WAYLAND_DISPLAY

echo "=== starting under Xvfb"
setsid nohup xvfb-run -a --server-args="-screen 0 1600x1000x24" \
  /tmp/msim-headless workbench > /tmp/headless.log 2>&1 < /dev/null &
sleep 1

for i in $(seq 1 40); do
  [ -S "$XDG_RUNTIME_DIR/meshbench.sock" ] && break
  sleep 1
done

if [ ! -S "$XDG_RUNTIME_DIR/meshbench.sock" ]; then
  echo "no control socket after 40s"
  tail -15 /tmp/headless.log
  exit 1
fi
echo "control socket up"

ask() {
  printf '{"id":1,"method":"%s","params":%s}\n' "$1" "${2:-\{\}}" \
    | timeout 30 socat - UNIX-CONNECT:"$XDG_RUNTIME_DIR/meshbench.sock"
}

echo "=== session.describe"
ask session.describe
echo "=== nodes.list (count)"
ask nodes.list | python3 -c "import sys,json; print(len(json.load(sys.stdin)['result']),'nodes')" 2>/dev/null
echo "=== sim.run 2000ms"
ask sim.run '{"for_ms":2000}'
sleep 3
echo "=== after running"
ask session.describe

echo "=== frames rendered (a real GL context, not a stub)"
grep -ciE "glfw|opengl|context" /tmp/headless.log || true
tail -3 /tmp/headless.log

for p in $(pgrep -u 1000 -f msim-headless); do kill "$p" 2>/dev/null; done
