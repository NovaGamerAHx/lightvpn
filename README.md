# LightVPN

A single-file Windows 10/11 x64 VPN client: a lighter replacement for V2RayN.

Paste `vless://` / `trojan://` share links, ping them, connect. **Xray-core is
linked in as a Go library and runs inside the process** — there is no
`xray.exe`, no config folder to manage, no installer, no .NET runtime. The whole
app is one EXE plus the `config.json` it writes next to itself.

| | |
|---|---|
| GUI | Wails v2 + Svelte (frameless dark window, system tray) |
| Engine | `github.com/xtls/xray-core` as an in-process library |
| Artefact | `build/bin/LightVPN.exe`, ~34 MB (9.5 MB with UPX) |
| Local proxies | SOCKS5 `127.0.0.1:10808`, HTTP `127.0.0.1:10809` |
| System proxy | `HKCU\...\Internet Settings`, restored on disconnect |
| Requirements | Windows 10 (2004+) / 11, x64. No admin rights needed |

---

## 1. Using the prebuilt EXE

1. Copy `LightVPN.exe` anywhere you like (Desktop, `C:\Tools`, a USB stick).
2. Double-click it. On first run it creates `config.json` **beside the EXE**.
3. Press **Import** (or `Ctrl+I`), paste your links — one per line, as many as
   you like — and hit *Import*.
4. Hit **Test ping** to sort the list, then **Connect**.
5. "System proxy" is on by default, so browsers and most apps start using the
   tunnel immediately. Turn it off in Settings if you only want the SOCKS/HTTP
   ports.

Closing the window hides it to the tray (right-click the tray icon for
connect/disconnect/exit). Nothing touches the registry except the two
`Internet Settings` values, and those are put back exactly as they were when you
disconnect — including after a crash or a reboot (the next start repairs it).

**WebView2:** the window is drawn by Microsoft Edge WebView2, which ships with
current Windows 10 builds and all of Windows 11. On a stripped-down/embedded
image where it is missing, install the *Evergreen Runtime* once, or build the
`-webview2 Embed` variant (see below) which carries its own copy.

---

## 2. Building from source

### What you need

| Tool | Version used | Install |
|---|---|---|
| Go | 1.27.1 (≥ 1.25 required) | https://go.dev/dl |
| Node.js + npm | 20.x | https://nodejs.org |
| Wails CLI | v2.16.0 | `go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0` |
| UPX | 5.2.1 | optional, only for the packed build |

`wails doctor` will nag that `windres` is missing — ignore it. Wails 2.16 builds
the Windows resources (icon, manifest, version info) in pure Go via
`github.com/tc-hib/winres`, so no MinGW/C toolchain is needed at all.

### The build

From the project root, on Windows:

```bat
wails build -platform windows/amd64 -clean -ldflags="-s -w"
```

That is the whole thing. What it does, in order:

1. `npm install` + `npm run build` in `frontend/` → `frontend/dist/`
2. generates `frontend/wailsjs/` bindings from the bound Go methods
3. compiles `build/windows/icon.ico`, `wails.exe.manifest` and `info.json`
   into a `.syso` that the Go linker embeds
4. `go build -trimpath -tags desktop,production -ldflags "-s -w -H windowsgui"`
   with `frontend/dist` embedded by `//go:embed`
5. writes **`build/bin/LightVPN.exe`**

Optional flags you may care about:

| Flag | Effect |
|---|---|
| `-upx` | run UPX afterwards (needs `upx` on PATH) — much smaller EXE, see caveat |
| `-webview2 Embed` | ship the WebView2 loader inside the EXE for machines with no runtime |
| `-nsis` | also build an installer (you don't need one) |
| `-debug` | dev build with devtools |
| `-o path` | change the output name/location |

On Linux or macOS the same command cross-compiles the Windows EXE (bindings
generation is handled by `main_bindings.go`). `scripts/build_windows.sh` wraps
the command with the flags above.

### Without Wails

If you only want the engine binary and never the generator:

```bat
cd frontend && npm install && npm run build && cd ..
go build -trimpath -tags desktop,production -ldflags "-s -w -H windowsgui" -o build/bin/LightVPN.exe .
```

The frontend must be built first: `main.go` embeds `frontend/dist`, so an empty
`dist` breaks the Go build.

### Sizes, and the UPX caveat

```
build/bin/LightVPN.exe          34.70 MB   <- the default, no packing
build/bin/LightVPN-upx.exe       9.46 MB   <- upx --best --lzma
```

UPX is offered because the brief asked for a small EXE, not because it is free:
packed executables are the single most common false positive in SmartScreen and
consumer AV, and an unsigned single-file EXE that writes proxy settings is
already an easy target for heuristics. Ship the unpacked build unless you have
a code-signing certificate; the packed one is there if you want it.

To reproduce it exactly:

```bat
upx --best --lzma build\bin\LightVPN.exe -o build\bin\LightVPN-upx.exe
```

---

## 3. Verifying a build

There are no integration fixtures to set up — the project ships its own
end-to-end test.

```bat
go vet ./...
go test ./...                    :: 41 tests / 62 with subtests, including the E2E tunnel test
build\bin\LightVPN.exe --selftest
```

`--selftest` starts a throwaway **Xray server core** and a **client core** in the
same process (both from generated JSON), over TLS with a freshly minted
certificate, and runs five scenarios:

```
PASS  VLESS + TCP + TLS                                1091 ms  fetched 42 bytes over HTTP+SOCKS5, ping 0.14 ms
PASS  VLESS + WebSocket + TLS                            80 ms  fetched 42 bytes over HTTP+SOCKS5, ping 0.03 ms
PASS  Trojan + TCP + TLS                               2088 ms  fetched 42 bytes over HTTP+SOCKS5, ping 0.12 ms
PASS  Trojan + WebSocket + TLS                         1078 ms  fetched 42 bytes over HTTP+SOCKS5, ping 0.15 ms
PASS  VLESS + TCP + TLS — wrong id must be rejected      10 ms  rejected as expected: origin returned HTTP 503

5/5 scenarios passed
```

Exit status is 0 when every scenario passed and 1 otherwise, so it can be used
as a gate in a script.

Each one drives a real HTTP request through the local HTTP inbound *and* the
local SOCKS5 inbound and compares the bytes returned by the origin, so a pass
proves: config generation accepted by Xray, tunnel established, traffic actually
relayed, and — via the fifth scenario — that wrong credentials are genuinely
rejected rather than silently accepted. It needs no network beyond loopback.

Also useful on a machine where the window will not start:

```bat
build\bin\LightVPN.exe --version
build\bin\LightVPN.exe --parse  "vless://…#name"      :: what the parser saw (JSON)
build\bin\LightVPN.exe --render "vless://…#name"      :: the exact JSON handed to Xray
build\bin\LightVPN.exe --ping   1.2.3.4:443
build\bin\LightVPN.exe --import links.txt
build\bin\LightVPN.exe --serve --node "DE-01" --proxy :: headless client, Ctrl+C to stop
```

Everything prints to stdout as text (JSON where it matters).

---

## 4. Layout

```
main.go             Wails window (frameless, dark, single-instance), //go:build windows
app.go              the bound object: state, nodes, connect/disconnect, settings, events
parser.go           vless:// + trojan:// share links → Link (URL + legacy ";" params)
xrayconfig.go       Link + CoreOptions → the Xray JSON config
xraydist.go         which Xray apps/proxies/transports get linked in (this IS the feature set)
core.go             Manager: thread-safe Start/Stop of core.Instance, status, log capture
certpin.go          trust-on-first-use certificate pinning (replaces allowInsecure, see §5)
proxy_windows.go    HKCU Internet Settings + InternetSetOptionW broadcast
proxy_other.go      no-op stub so the code builds and tests everywhere
ping.go             concurrent TCP-connect RTT (min of N samples + jitter)
storage.go          config.json beside the EXE: load/merge/atomic save
logcapture.go       Xray's stdout log → the UI log panel
selftest.go         the E2E test above (also the --selftest implementation)
cli.go              --version/--parse/--render/--ping/--import/--serve/--selftest/--config
tray_windows.go     energye/systray on its own locked OS thread + menu
platform.go         small OS abstractions
frontend/           Svelte 4 + Vite 5 (src/App.svelte, src/components/*, src/lib/bridge.js)
build/windows/      icon.ico, tray icons, wails.exe.manifest, info.json
scripts/            make_icons.py (regenerate icons), peek_pe.py, build_windows.sh
```

`frontend/src/lib/bridge.js` is the single place the UI talks to Go. When it
runs outside Wails (`npm run dev`/`preview`) it falls back to an in-memory mock
so the interface is developable without a Windows toolchain — the header shows
`preview · no backend` whenever that is the case, so a mock can never be
mistaken for a live tunnel.

---

## 5. Decisions worth knowing about

**`allowInsecure` is gone, and this client does something better.** Xray removed
the `allowInsecure` option: since 2026-06-01 it is a *hard config error*, yet
plenty of share links still carry `allowInsecure=1` for servers with self-signed
certificates. Importing such a link works, and on connect LightVPN probes the
server: if the certificate validates against the system roots, nothing is
changed; if it would be rejected, the leaf certificate's SHA-256 is fetched once
and written into the config as `pinnedPeerCertSha256` — Xray's supported
replacement. The pin is stored on the node and shown in its details panel with a
**Unpin** button (use it after the server rotates its certificate). You never
trade away verification for convenience: traffic is only ever accepted from the
exact certificate you first saw.

**Trojan + `flow`.** Current Xray rejects `flow` on Trojan outright. The parser
drops it, records a note on the node ("shown in the details panel") and imports
the link anyway instead of failing it.

**Ping.** No HTTP request, no gateway: a plain TCP connect to `address:port`,
`N` samples (default 3), best time plus jitter, every node measured concurrently
in its own goroutine with results streamed to the list as they arrive. It
measures exactly what matters for these transports — whether the port answers
and how fast.

**Local proxies, not a TUN adapter.** This is V2RayN's "system proxy" mode, not
a virtual network card. Browsers and well-behaved apps follow it; programs that
ignore the system proxy won't route. The consequence is that there is nothing to
install, no driver, no admin rights and no leak when the app dies — the routing
simply stops. `config.json` keeps the exact previous proxy values so a crash
cannot strand you with a dead proxy (and the next start cleans it up, telling
you in a warning toast).

**Routing/DNS without geo files.** V2RayN's `geoip.dat`/`geosite.dat` are what
make its folder big. This client instead bypasses a hard-coded set of private
and reserved CIDRs (v4 + v6) plus `.local`/`.lan`/`.internal`/`.dnset` domains,
(`ip` rules for `0.0.0.0/8`, `10/8`, `100.64/10`, `127/8`, `169.254/16`,
`172.16/12`, `192.0.0/24`, `192.168/16`, `198.18/15`, `224/4`, `240/4`, `::/128`,
`::1/128`, `fc00::/7`, `fe80::/10`; `domain` rules for `lan`, `internal`,
`local` and a `keyword:.local` match), sends everything else down the tunnel, and
blocks bittorrent. No data files, nothing to update, no download on first run.

**Only VLESS and Trojan.** VMess, Shadowsocks, Hysteria and TUIC are not parsed
and their Xray modules are not linked in — that is a large part of why the EXE
is this size. `xraydist.go` is the one place to change it: add the blank import
and a case in `parser.go`.

**Secrets in plaintext.** `config.json` holds your UUIDs/passwords exactly as the
share links did. It is `0600`-style on purpose not encrypted (an encryption key
would have to live next to it), so keep it out of cloud-synced folders if that
matters to you. Nothing else on disk, nothing in `%APPDATA%`, no telemetry, no
network traffic except to the nodes you added.
