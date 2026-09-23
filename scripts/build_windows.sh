#!/usr/bin/env bash
# build_windows.sh — one command from Linux/macOS/Windows(Git Bash) to produce
# the portable Windows 10/11 x64 EXE, then verify it.
#
#   ./scripts/build_windows.sh            # -> build/bin/LightVPN.exe
#   ./scripts/build_windows.sh --upx      # also writes build/bin/LightVPN-upx.exe
#   ./scripts/build_windows.sh --embed-wv2  # WebView2 loader inside the EXE
#   ./scripts/build_windows.sh --no-verify  # skip the test + selftest gates
set -euo pipefail

cd "$(dirname "$0")/.."
PROJECT=$PWD
UPX=${UPX:-upx}
FLAGS=(-platform windows/amd64 -clean -ldflags="-s -w")
VERIFY=1
FOR_PACKED=""

for arg in "$@"; do
  case "$arg" in
    --upx)        FOR_PACKED=1 ;;
    --embed-wv2)  FLAGS+=(-webview2 Embed) ;;
    --no-verify)  VERIFY=0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

export PATH="$PATH:$(go env GOPATH)/bin"

if ! command -v wails >/dev/null 2>&1; then
  echo "wails CLI not found — install it with:" >&2
  echo "  go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0" >&2
  exit 1
fi

# 1. Backend gates first: cheaper to fail here than after a minute of linking.
echo "==> go vet"
go vet ./...
if [ "$VERIFY" = 1 ]; then
  echo "==> go test"
  go test -count=1 -timeout 15m ./...
fi

# 2. The real build (also rebuilds the frontend via npm).
echo "==> wails build ${FLAGS[*]}"
wails build "${FLAGS[@]}"

EXE=build/bin/LightVPN.exe
ls -l "$EXE"

# 3. Optional packing. UPX trips SmartScreen/AV heuristics far more often than
#    a plain Go binary, so this is opt-in and produces a *second* file.
if [ -n "$FOR_PACKED" ]; then
  if command -v "$UPX" >/dev/null 2>&1; then
    cp "$EXE" build/bin/LightVPN-upx.exe
    "$UPX" --best --lzma build/bin/LightVPN-upx.exe
    ls -l build/bin/LightVPN-upx.exe
  else
    echo "!! UPX requested but '$UPX' is not on PATH — skipping" >&2
  fi
fi

# 4. Verify the artefact itself. The GUI cannot start in a container, but the
#    PE header + embedded resources can be inspected, and the CLI paths of a
#    Linux build exercise the same engine.
echo "==> inspecting PE resources"
python3 scripts/peek_pe.py "$EXE"
if [ "$VERIFY" = 1 ]; then
  echo "==> engine self test (host build, same code as the EXE)"
  go run . --selftest
fi
echo
echo "Done: $PROJECT/$EXE"
