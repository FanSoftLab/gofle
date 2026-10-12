# GoFLE

[![Go Version](https://img.shields.io/github/go-mod/go-version/sankam-nikolya/GoFLE)](https://go.dev/)
[![License](https://img.shields.io/github/license/sankam-nikolya/GoFLE)](./LICENSE.md)
[![Go Report Card](https://goreportcard.com/badge/github.com/sankam-nikolya/GoFLE)](https://goreportcard.com/report/github.com/sankam-nikolya/GoFLE)

**Go Fast Log Explorer** — a cross-platform desktop application for browsing,
searching, and tailing very large log files and remote (SSH) logs.

GoFLE is a re-implementation of the classic [glogg](https://github.com/nickbnf/glogg)
"log explorer" concept, rebuilt with **Go** and **Wails v2** so it runs natively on
Linux, macOS, and Windows from a single codebase. The backend (file indexing, search,
SSH streaming) is written in Go; the UI is a lightweight TypeScript + Vite web frontend
rendered inside a native webview.

---

## Table of Contents

- [Demo](#demo)
- [Getting Started](#getting-started)
- [Why GoFLE](#why-gofle)
- [Features](#features)
- [Search capabilities (detail)](#search-capabilities-detail)
- [Scope & Intended Use](#scope--intended-use)
- [Remote Transports (SSH & FTP)](#remote-transports-ssh--ftp)
- [Architecture](#architecture)
- [Project Structure](#project-structure)
- [Requirements](#requirements)
- [Building](#building)
- [Running](#running)
- [Usage Examples](#usage-examples)
- [Keyboard Shortcuts](#keyboard-shortcuts)
- [CLI Companion](#cli-companion)
- [Configuration](#configuration)
- [Security Notes](#security-notes)
- [Development](#development)
- [Contributing](#contributing)
- [Contributors](#contributors)
- [License](#license)

---

## Demo

GoFLE is a native GUI application, so the fastest way to see it is to build and run
it (see [Getting Started](#getting-started)). A typical session looks like this:

1. **Open** a log file — drag-and-drop it onto the window, press `Ctrl+O`, or use
   **Open via SSH** for a remote file.
2. The file loads via a byte-offset index — only line offsets are kept in memory,
   so even a multi-GB file becomes scrollable right away.
3. Type a query in the search box (e.g. `ERROR|WARN`), toggle **Regex** (`Alt+R`),
   and the bottom results pane fills with matches you can jump between (`F3`).
4. Enable **Follow** (`F`) to tail new lines as they are written to disk.

Minimal terminal example (after building the binary):

```bash
# Build once (see Getting Started), then launch and open two files directly
./GoFLE /var/log/syslog /srv/app/app.log
```

> No screenshot image is committed to the repository, so the UI is described in
> text above and demonstrated step-by-step in [Usage Examples](#usage-examples).

---

## Getting Started

**Prerequisites**

- **Go 1.27+**
- **Node.js 18+** and npm
- **Wails CLI v2**: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Linux only**: CGO enabled plus the GTK/WebKit system libraries (see
  [Requirements](#requirements)). macOS needs Xcode command-line tools; Windows
  needs a C compiler and the WebView2 runtime.

**Fastest build & run** — `./build.sh` builds everything inside Docker, so no host
GTK/WebKit toolchain is required, and produces the binary at the repository root:

```bash
./build.sh        # → ./GoFLE at the repo root
./GoFLE           # launch the GUI
```

**Local build** — requires the native toolchain (Go, Node, Wails CLI, and on Linux
the GTK/WebKit dev packages) to be installed on your machine:

```bash
cd frontend && npm install && npm run build && cd ..
wails build                              # → build/bin/GoFLE
./build/bin/GoFLE
```

For live front-end hot-reload during development, use `wails dev` (see [Development](#development)).

---

## Why GoFLE

**GoFLE — Go Fast Log Explorer.** Production logs are often multi-gigabyte and keep
growing. Editors choke, `less` stays terminal-only, and web viewers need a server
pipeline. GoFLE is a **native desktop** log explorer that prioritizes **speed and
low memory**:

| Need | How GoFLE handles it |
| --- | --- |
| Open huge files | Byte-offset **index only** — file content is read lazily (`pread`), not loaded into RAM |
| Smooth scroll | **Virtualized** log view — only visible rows exist in the DOM |
| Find signal in noise | Parallel **substring / regex** search, AND filter, proximity, levels, time, exclude |
| Live systems | **Follow/tail** local and remote streams without reopening the file |
| Many files at once | **Tabs + groups**, session restore, recent local/SSH/FTP |
| Remote ops | **SSH** and **FTP/FTPS** into the same UI as local files |
| One binary story | **Go + Wails v2** — Linux / macOS / Windows; optional CLI companion `gofle` |

Compared to a general editor, GoFLE is **search- and tail-first**. Compared to
`grep`/`less`, it keeps context, navigation, and multi-file workflow in one place
without scanning the entire file into memory.

---

## Features

| Feature | Description |
| --- | --- |
| **Instant large-file loading** | Only a byte-offset index is built; lines are read lazily via `pread`. |
| **Virtualized rendering** | Only visible rows are in the DOM — smooth scrolling on millions of lines. |
| **Live tail / follow** | Background poller detects growth and appends new lines without full reload. |
| **Parallel search engine** | Worker-pool over CPU cores; primary scan + optional refine filters. |
| **Rich search toolbar** | Substring/regex, case, invert, secondary **AND**, history, export matches. |
| **Advanced filters** | Nearby (before/after/either), levels, time window, exclude patterns, line range. |
| **Results context** | Optional −B/−A style context lines embedded on each match row. |
| **Top unique** | Aggregate frequent match texts over the current result set (batched, memory-bounded). |
| **Match navigation** | First / prev / next / last, plus **from current line** prev/next. |
| **Multi-term highlight** | Primary, AND, nearby, and exclude terms with distinct colors. |
| **Search presets** | Save/load advanced filter combinations from the toolbar. |
| **Tabs & groups** | Multiple files, colored groups, drag order, session restore. |
| **Remote SSH / FTP** | Stream remote logs into the same indexer/search path as local files. |
| **Recent files** | Local / SSH / FTP tabs; reopen without retyping connection details. |
| **Settings** | Appearance, log view, **Search**, session, remote timeouts & host-key policy. |
| **In-app help (F1)** | Tabbed guide with search; Ukrainian / English. |
| **i18n & themes** | Ukrainian / English; light/dark/system. |
| **CLI companion** | `gofle` for terminal open/search/recent (unchanged by GUI-only features). |

### Search capabilities (detail)

```text
Primary query ──► parallel index scan ──► Secondary AND (same line)
                                      ──► Exclude patterns
                                      ──► Level filter (ERROR/WARN/…)
                                      ──► Time from/to (timestamp heuristic)
                                      ──► Nearby window (before | after | either)
                                      ──► Line range / from-cursor
                                      ──► Results pane (+ optional context)
                                      ──► Top unique (optional)
```

| Mode | What it does | Performance notes |
| --- | --- | --- |
| **Substring / Regex** | Classic match on each line | Parallel workers; regex demoted to substring when no metacharacters |
| **Case / Invert** | Sensitivity and non-match mode | Applied in the engine, not a second full-file pass |
| **Secondary AND** | Line must also match a second pattern | Refine path over primary hits when only AND changes |
| **Nearby** | Primary line must have another pattern within N lines | Single post-pass; line cache ≤256; window capped (≤200) |
| **Levels** | Keep ERROR / WARN / INFO / DEBUG / OTHER | Cheap prefix scan of match lines only |
| **Time** | Keep lines whose parsed timestamp is in range | Heuristic layouts (RFC3339, syslog-like); optional “require stamp” |
| **Exclude** | Drop lines matching any exclude pattern | Same matcher stack as AND; optional regex |
| **Range** | Search entire file, from/to cursor, or line numbers | Uses `SearchRange` — less work than full file |
| **Context** | Show ±K lines around each **visible** match | In-row only; does not inflate virtual list length |
| **Top** | Count duplicate match texts | Batches of 200; max 20k matches scanned; map pruned to limit |
| **From here** | Next/prev match relative to the viewport line | Client-side over current filtered indices |
| **Presets** | Named snapshots of advanced filters | Stored in `localStorage` |

Defaults stay **backward compatible**: empty advanced filters mean the previous
primary + AND + regex/case/invert behaviour.

### Settings → Search

Configurable defaults (GUI **Settings → Пошук / Search**):

- Context lines before / after  
- Default Nearby mode and window  
- Exclude-as-regex default  
- Time: require parseable timestamp  
- Top: max unique rows  
- Highlight toggles: primary, AND, nearby, exclude  

---
## Scope & Intended Use

**GoFLE is a read-oriented log viewer.** It is designed for:

- SREs / DevOps engineers tailing application, container, or system logs.
- Developers inspecting local debug output and large test/trace logs.
- Anyone who needs to grep a multi-gigabyte file interactively without a terminal.
- Tailing remote server logs over SSH without copying the file down.

**It is intentionally not:**

- a log *aggregator* or collector (no ingestion, no indexing server, no agents);
- a full text editor (you cannot modify the log — only read, search, copy, export);
- a log *parser/analyzer* with structured-field extraction (search is line-based).

---

## Remote Transports (SSH & FTP)

GoFLE reaches logs on a remote host over two transports, and both share one
backend abstraction (`pkg/remote`): the application and the indexer depend only
on the `remote.Streamer` (stream a file to a local temp file) and
`remote.Lister` (browse a directory) interfaces, while `pkg/ssh` and `pkg/ftp`
each implement them. A URL of either scheme opens identically in the GUI
(**Open via SSH** also accepts `ftp://`) and in the CLI (`gofle <url>`).

### SSH (`ssh://`)

Streams the remote file to a local temp file and tails it live. Host keys are
verified against the OpenSSH `known_hosts` file
(`~/.config/GoFLE/known_hosts`). An unknown key is shown (host + SHA256
fingerprint) and must be confirmed before it is persisted; a key that changes
after being trusted is rejected as a possible MITM. See
[Usage Examples](#3-tail-a-remote-log-over-ssh).

### FTP / FTPS (`ftp://`)

`ftp://user@host[:port]/remote/path` opens an FTP or FTPS (explicit/implicit)
stream the same way. Transport security is selected by `TLSMode`:

| `TLSMode` | Meaning |
| --- | --- |
| `""` / `"none"` | Plain FTP (no TLS). |
| `"explicit"` | FTPS: `AUTH TLS` on the control connection; data is tunneled after. |
| `"implicit"` | FTPS: TLS from the first byte of the control connection. |

FTP authenticates with a **password only** (key auth is unsupported). The
streamer copies the remote file to a local temp file and polls for appended
bytes every 30 s (`streamProbeInterval`); the indexer treats the temp file like
any local file.

**Certificate trust.** The server's FTPS certificate is pinned against a local
trust store at `~/.config/GoFLE/ftp_certs` (one `host:port <sha256-fingerprint>`
line per host). On first contact an unknown certificate surfaces as a
`CertUnknownError`: the GUI/CLI shows the host and fingerprint and asks you to
confirm; on acceptance the line is written via `ftp.WriteCertTrust` and the
connection is retried. A certificate that *differs* from the stored one raises a
`CertMismatchError` (potential MITM) and is never auto-trusted.

> SSH and FTP recent entries are stored together (deduped by a canonical
> `scheme://host:port/path` key with no credentials) and reopenable from both
> the GUI's Recent list and the CLI (`gofle recent`).

## Architecture

GoFLE follows a **thin-backend / thin-frontend** model typical of Wails v2 apps:
the Go `App` is bound to the JS runtime and exposes async methods; the frontend calls
them through the generated `wailsjs` bridge and renders the results.

```
        ┌─────────────────────────── Native Webview (Frontend) ───────────────────────────┐
        │  TypeScript (Vite)                                                              │
        │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌────────────────────┐   │
        │  │VirtualScroller│  │ TabManager   │  │ Modal/SSH/   │  │ SettingsManager,   │   │
        │  │ (rendering)   │  │ (tabs/groups)│  │ ContextMenu  │  │ i18n, highlighter  │   │
        │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └─────────┬──────────┘   │
        └─────────┼─────────────────┼─────────────────┼────────────────────┼──────────────┘
                  │  window.go.main.App.*   (wailsjs generated bridge)        │
        ┌─────────┼─────────────────┼─────────────────┼────────────────────┼──────────────┐
        │  Go Backend (bound)       │                 │                    │               │
        │  ┌──────▼───────┐   ┌─────▼───────┐   ┌─────▼───────┐  ┌────────▼───────────┐  │
        │  │   app.go     │   │  pkg/indexer│   │ pkg/search  │  │     pkg/ssh        │  │
        │  │  (App, tabs, │   │  (offset    │   │ (parallel   │  │  (SSH streamer +  │  │
        │  │   polling,   │──▶│   index,    │──▶│  regex/     │  │   incremental tail)│  │
        │  │   orchestr.) │   │   tail)     │   │  substring) │  │                    │  │
        │  └──────┬───────┘   └─────┬───────┘   └─────┬───────┘  └────────┬───────────┘  │
        │         │                 │                 │                   │               │
        │         └────────────────►│◄────────────────┘                   │               │
        │                   ┌───────▼────────┐                           │               │
        │                   │  pkg/config    │◄── persisted JSON +       │               │
        │                   │ (settings,     │    AES-GCM secrets        │               │
        │                   │  recent, tabs) │                           │               │
        │                   └────────────────┘                           │               │
        │                         │                                     │               │
        │                   ~/.config/GoFLE/*                        │               │
        │                                                              local temp file    │
        └─────────────────────────────────────────────────────────────────────────────────┘
```

The `pkg/ssh` and `pkg/ftp` transport packages sit behind a shared
`pkg/remote` abstraction: both implement `remote.Streamer` (copy a remote file
to a local temp file for the indexer) and `remote.Lister` (browse a remote
directory). `pkg/logging` centralizes `slog` setup for both the desktop binary
and the `gofle` CLI, writing to stderr and `gofle.log` beside the config dir.

### Data flow

1. **Open** — `App.OpenFile` creates a `FileIndexer` that scans the file once to build
   a slice of byte offsets (one per line). This is O(file size) but memory-light
   (just integers). The file handle stays open for `pread`-based random access.
2. **Render** — the frontend's `VirtualScroller` asks for visible line ranges via
   `App.GetLogLines(tabID, start, count)`; the indexer seeks to the offset and returns
   just those lines.
3. **Tail** — a 500 ms `time.Ticker` in `App` calls `Indexer.UpdateIndex()`, which
   `stat`s the file, seeks to the last known offset, and appends any new lines. A
   `file_updated` runtime event notifies the UI. If a search is active, only the new
   range is re-searched (`SearchEngine.SearchRange`) and appended to the match list.
4. **Search** — `App.SearchLines` delegates to `SearchEngine`, which fans the line range
   across a worker pool (`runtime.NumCPU` goroutines), each scanning a 3000-line chunk
   via substring or compiled regex, then merges indices back in order.
5. **SSH / FTP** — `ssh.Streamer` opens an SSH session, `cat`s the remote file into a local
   temp file (initial sync), then tails it every 2 s with `tail -c +N`. `ftp.Streamer`
   mirrors this over FTP/FTPS, polling for appended bytes every 30 s. Both temp files are
   then indexed exactly like a local file, so search/tail behave identically across transports.

---

## Project Structure

```
    GoFLE/
    ├── main.go                 # Wails app entrypoint: window options, asset embedding, bindings
    ├── app.go                 # Backend core: App struct, tab lifecycle, polling, search orchestration
    ├── wails.json             # Wails project config (frontend dir, build scripts, metadata)
    ├── go.mod / go.sum        # Go module (Go 1.27, Wails v2, golang.org/x/crypto)
    ├── build.sh               # Reproducible multi-platform build via Docker
    ├── Dockerfile             # Build image: Go + GTK/WebKit + Node, runs wails build
├── scripts/install.sh     # Linux install helper: copies binary + icon, generates the .desktop entry
└── GoFLE                   # Binary produced at the repo root by ./build.sh (raw wails build → build/bin/GoFLE)
    ├── .golangci.yml          # golangci-lint configuration
    ├── .github/workflows/     # CI: lint + vet + test on push/PR
    ├── build/
    │   └── appicon.png        # Application icon (embedded)
    ├── cmd/
    │   └── gofle/              # Terminal companion binary: cobra CLI (ssh/ftp/list/version) + Bubbletea viewer
    │       └── main.go        # CLI entrypoint: wires commands, signal-scoped cancellation, exit codes
    ├── pkg/
    │   ├── config/
    │   │   ├── config.go          # Config load/persist, recent files, saved tabs, AES-GCM secrets
    │   │   └── config_test.go
    │   ├── indexer/
    │   │   ├── indexer.go         # FileIndexer: offset index, random line access via `pread`, incremental tail
    │   │   └── indexer_test.go
    │   ├── search/
    │   │   ├── search.go          # SearchEngine: parallel regex/substring search over a line range, case-sensitivity, invert-match; caches the compiled regex for the tail poller.
    │   │   └── search_test.go
    │   ├── remote/
    │   │   └── remote.go       # Shared transport contract: Streamer/Lister interfaces + unified Config (ssh & ftp implement these).
    │   ├── ftp/
    │   │   └── ftp.go          # Streamer/Lister over FTP/FTPS (explicit/implicit); pins server certs in the ftp_certs trust store.
    │   ├── logging/
    │   │   └── logging.go      # Centralized slog setup for the desktop binary and the gofle CLI (stderr + gofle.log).
    │   └── ssh/
    │       └── ssh.go             # Streamer: streams a remote file over SSH to a local temp file, tails it every 2 s, verifies host keys via OpenSSH `known_hosts`.
    └── frontend/
        ├── index.html             # App shell (toolbar, panes, modals)
        ├── package.json           # npm scripts (dev / build) + dev deps (vite, typescript, fa)
        ├── vite.config.ts         # Vite build config (output → dist, embedded by Go)
        ├── tsconfig.json
        └── src/
            ├── main.ts            # Composition root: wires managers, shortcuts, events
            ├── app_context.ts     # Shared AppContext type & tab/group models
            ├── virtual_scroller.ts# Windowed list renderer (lazy line fetching)
            ├── tab_manager.ts     # Tab & group lifecycle, UI
            ├── modal_controller.ts# Recent / SSH / Group / GoTo / Settings modals
            ├── ssh_handler.ts     # SSH connection form logic
            ├── settings_manager.ts# Theme / language / save-tabs persistence
            ├── context_menu.ts    # Right-click copy / goto / export
            ├── log_highlighter.ts # Regex/term highlighting in rows
            ├── i18n.ts            # Ukrainian & English translations
            ├── style.css          # Themes (dark/light/dracula)
            └── wailsjs/           # Auto-generated Go↔JS bridge (App.d.ts/.js, models.ts)
```

---

## Requirements

### All platforms
- **Go 1.27+**
- **Node.js 18+** and npm (for the frontend build)
- **Wails CLI v2**: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### Linux (native build)
System libraries for the WebKit webview:
```bash
# Debian / Ubuntu
sudo apt-get update
sudo apt-get install -y build-essential pkg-config \
    libgtk-3-dev libwebkit2gtk-4.1-dev

# Fedora
sudo dnf install gtk3-devel webkit2gtk4.1-devel gcc pkgconf-pkg-config
```

### macOS
```bash
xcode-select --install
```

### Windows
Install the [Wails prerequisites](https://wails.io/docs/gettingstarted/installation/#windows)
(requires a C compiler and WebView2 runtime).

---

## Building

### Option A — Local Wails build (fastest for development)

```bash
# 1. Build the frontend bundle (output: frontend/dist)
cd frontend
npm install
npm run build
cd ..

# 2. Build the desktop binary (CGO required on Linux)
wails build            # production → build/bin/GoFLE
wails dev               # dev build with hot-reload + dev tools (not "wails build -dev")
wails build -platform windows/amd64,darwin/arm64,linux/amd64   # cross-build
```

The binary is created under `build/bin/`.

> **Binary location depends on the build path you chose:**
> - `./build.sh` (Docker build) → produces the binary at the **repository root** as `./GoFLE`.
> - a raw `wails build` → produces `build/bin/GoFLE`.
>
> See [Running](#running) for how to launch either one.

### Option B — Reproducible Docker build (recommended for releases)

`build.sh` uses the included `Dockerfile` to build inside a container with all system
dependencies, producing deterministic artifacts for one or many platforms.

```bash
# Native linux/amd64 production binary → ./GoFLE
./build.sh

# Cross-platform / multi-arch binaries → ./dist/
./build.sh -os all -arch all

# Run tests and vet inside the container before building
./build.sh -run-tests -os linux -arch amd64

# Build a dev binary for macOS
./build.sh -mode dev -os mac -arch arm64
```

| Flag | Values | Default | Meaning |
| --- | --- | --- | --- |
| `-mode` | `dev` \| `prod` | `prod` | Build mode. |
| `-os` | `linux` \| `mac` \| `windows` \| `all` | `linux` | Target OS. |
| `-arch` | `amd64` \| `arm64` \| `all` | `amd64` | Target arch. |
| `-run-tests` | (flag) | off | Run `go test ./...` + `go vet` first. |
| `-h` | — | — | Show help. |

> CGO is enabled automatically for the native Linux/WebKit build. Wails disables CGO
> when cross-compiling, so cross-platform builds do not need the host GTK toolchain.

### Installing (Linux)

A Linux install/package helper is shipped at `scripts/install.sh`. It copies the
binary to a `bin/` directory, installs the icon into the hicolor theme, and
**generates** the `.desktop` entry for you (a committed `GoFLE.desktop` file is not
part of the repo — the installer creates it at install time). Run it after a
successful `linux/amd64` build:

```bash
# System-wide install (copies to /usr/local/bin/GoFLE and registers the menu entry)
sudo ./scripts/install.sh --system

# Or install for the current user only (default; no sudo needed)
./scripts/install.sh --user
```

This makes GoFLE appear in the application menu and lets it be set as the default
handler for `.log`/`.txt` files. The installer looks for the binary first next to
itself and then at the repository root (`scripts/install.sh` lives one level under
the repo root), so run `./build.sh` (which produces `./GoFLE` at the root) first.

### Releasing (local only)

Релізи **лише з локальної машини** — без GitHub Actions, без deb/rpm, **без macOS**.

Платформи: **linux + windows** (amd64/arm64). macOS прибрано: Wails GUI з Linux
просто так не крос-компілюється.

1. **`./build.sh`** збирає CLI + GUI у `dist/`.
2. **GoReleaser** пакує готові CLI (`builder: prebuilt`), checksums і публікує Release;
   GUI з `dist/GoFLE*` — як `extra_files`.

```bash
export GITHUB_TOKEN=ghp_…   # classic PAT, scope: repo

./scripts/release.sh v2.1.0              # build.sh + package + publish
./scripts/release.sh v2.1.0 --snapshot   # без публікації
./scripts/release.sh v2.1.0 --skip-build # тільки pack існуючого dist/
```

Вручну:

```bash
./build.sh -os linux -arch all -target all -v 2.1.0
./build.sh -os windows -arch all -target all -v 2.1.0
git tag -a v2.1.0 -m "v2.1.0" && git push origin v2.1.0
export GITHUB_TOKEN=ghp_… GORELEASER_CURRENT_TAG=v2.1.0
goreleaser release --clean
```

Конфіг: `.goreleaser.yaml`. Оркестратор: `scripts/release.sh`.

---

## Running

```bash
# If you built with ./build.sh, the binary is at the repo root:
./GoFLE

# If you built with a raw `wails build`, it is under build/bin/:
./build/bin/GoFLE

# Open specific files at launch (also works via file association / "Open with")
./GoFLE /var/log/syslog /srv/app/app.log

# Or run via the Wails dev server (frontend hot-reload) instead of a built binary
wails dev
```

---

## Usage Examples

### 1. Open and tail a live log

1. Click **Open** (or press `Ctrl+O`) and choose a file, or drag a file onto the window.
2. The log loads instantly. New lines appended by other processes appear automatically
   when **Follow** (Tail) is active — watch the status bar show `Tail: ACTIVE`.
3. Pause tailing with the **Follow** button (or `F`) so the view stays put while you read.

### 2. Search with a regular expression

1. Type a pattern into the search box, e.g. `ERROR|WARN`.
2. Click the **Regex** toggle (or `Alt+R`) to interpret it as a regular expression.
3. Toggle **Match Case** (`Alt+C`) and/or **Invert Match** (`Alt+I`) as needed.
4. The bottom pane lists every matching line. Click a result to jump the main view to it.
5. Navigate matches with `F3` / `Shift+F3` or the first/prev/next/last buttons.

Example patterns:

```regex
# Lines mentioning a specific request id
req-[0-9a-f]{8}

# IPv4 addresses
\b\d{1,3}(\.\d{1,3}){3}\b

# Timestamps after a given time (case-insensitive)
2026-08-13 2[0-9]:

# Everything EXCEPT debug lines (use Invert Match)
DEBUG
```

### 2b. Advanced search (nearby, exclude, range)

1. Enter a primary query (e.g. `timeout`).
2. Open **Nearby** on the advanced bar — set “connection refused”, mode **after**, window **15**.
3. Optionally **Exclude** noisy patterns (`healthcheck`) and limit **Range** to “from cursor down”.
4. Results list only primary lines that satisfy every active filter; use **Top** to see the most frequent match texts.

### 3. Tail a remote log over SSH

1. Click **Open via SSH** (the SSH toolbar button).
2. Fill in host, port, username, and choose **Password** or **Private Key** auth
   (with optional key passphrase).
3. Enter the remote file path (e.g. `/var/log/nginx/access.log`) and click **Connect**.
4. GoFLE streams the file to a local temp file and indexes it. The remote file keeps
   getting tailed; new lines appear live, just like a local file.
5. The connection is saved to **Recent** so you can reopen it later. SSH credentials are
   stored encrypted at rest (AES-GCM) — see [Security Notes](#security-notes).

```typescript
// Equivalent backend call (exposed to the frontend via the Wails bridge):
await window.go.main.App.OpenSSHFile({
  host: "10.0.0.5",
  port: 22,
  username: "deploy",
  auth_type: "key",
  key_path: "/home/me/.ssh/id_ed25519",
  key_passphrase: "",
  remote_file_path: "/var/log/app/service.log",
});
```

### 4. Organize with tabs and groups

- Open multiple files; each becomes a tab.
- Click **Create Tab Group**, name it, and pick a color.
- Assign tabs to groups from the per-tab settings menu to visually separate, for example,
  "Production" vs "Staging" logs.

### 5. Export results

- **Save Search Results** (`Ctrl+S`) writes only the matched lines (prefixed with their
  line numbers) to a `.log` file.
- From a tab's context menu, **Save Log As…** exports the entire file.

### 6. Restore your session

Enable **Save open tabs on exit** in Settings. Next launch, all previously open tabs
(local and SSH) are restored automatically.

---


## In-app help

Press **F1** or the **?** button next to Settings to open the interactive help window.

- Content is grouped into tabs (Overview, Files, Search, Advanced filters, SSH/FTP, Settings, Shortcuts, CLI).
- Each topic includes short explanations and, where useful, examples.
- Use the search field at the top of the help window to filter topics and highlight matches.
- Language follows the app UI language (Ukrainian / English).
Topics cover toolbar buttons, tabs, search, every advanced filter, settings groups, remote access, and shortcuts.

---
## Keyboard Shortcuts

| Shortcut | Action |
| --- | --- |
| `Ctrl+O` | Open file(s) |
| `Ctrl+R` | Recent files |
| `Ctrl+G` | Go to line |
| `Ctrl+S` | Save search results |
| `/` | Focus search box |
| `F` | Toggle Follow / Tail |
| `Alt+C` | Toggle Match Case |
| `Alt+R` | Toggle Regex |
| `Alt+I` | Toggle Invert Match |
| `F3` / `Alt+↓` | Next match |
| `Shift+F3` / `Alt+↑` | Previous match |
| `Alt+Home` | First match |
| `Alt+End` | Last match |

---

## CLI Companion

`gofle` is a terminal companion to the GoFLE desktop app. It reuses the same
pure-Go backend (`pkg/indexer`, `pkg/search`, and `pkg/remote` + `pkg/ssh` /
`pkg/ftp`) to open a log — local or over `ssh://` / `ftp://` — in an interactive
terminal viewer built with Bubbletea, or to dump a remote file straight to stdout.

### Install / build

```bash
go build ./cmd/gofle     # produces ./gofle next to the repo root
./gofle --help
```

> The Go module path is `GoFLE` (not a proxy-resolvable URL), so
> `go install GoFLE/cmd/gofle@latest` will not resolve from a proxy. Build it
> locally with `go build ./cmd/gofle` instead.

### Command surface

| Command | Description |
| --- | --- |
| `gofle [target]` | Open a local file or an `ssh://` / `ftp://` URL in the **interactive viewer**. With no argument, prints help. |
| `gofle version` | Print the `gofle` version. |
| `gofle ssh [flags] <host>` | Connect over SSH and stream a remote file. Default: open the interactive viewer. Pass `--dump` to print to stdout. Omit `--remote` to browse the remote filesystem interactively. |
| `gofle ftp [flags] <host>` | Same as `ssh`, but over FTP/FTPS. `--tls explicit\|implicit` selects FTPS. |
| `gofle recent` | Interactive recent-file picker (shared with the GUI). |
| `gofle recent open <n>` | Reopen recent entry `<n>` in the viewer (non-interactive). |
| `gofle recent clear` | Clear the recent list. |
| `gofle presets` / `list` | List shared search presets. |
| `gofle presets import <file.json>` | Import GUI localStorage-format presets. |
| `gofle presets export [file.json]` | Export presets (stdout or file, GUI-compatible JSON). |
| `gofle session show` | List tabs in the saved session. |
| `gofle session restore` | Open saved session and **auto-load all tabs**. |
| `gofle session clear` | Clear the saved session. |

**`--view` vs `--dump` (ssh / ftp only)**

| Flag | Behaviour |
| --- | --- |
| *(none)* / `--view` | Stream the remote file and open it in the interactive terminal viewer (default). |
| `--dump` | Stream the remote file and print it to stdout. Useful for piping: `gofle ssh host --remote /var/log/x.log --dump \| grep error`. |

Root-level targets (`gofle /path/to/file`, `gofle ssh://…`, `gofle ftp://…`) always open the viewer; there is no dump mode for them.

Common flags for `ssh` / `ftp`:

| Flag | Description |
| --- | --- |
| `-H` / `--host [user@]host` | Remote host (alternative to the positional `<host>`). |
| `-u` / `--user` | Username (overrides any `user@` embedded in the host). |
| `-p` / `--port` | Port (default 22 for SSH, 21 for FTP). |
| `--remote <path>` | Remote file path. Optional — omit to launch an interactive directory browser. |
| `-k` / `--key <path>` | Private key for SSH public-key auth (ignored for FTP). |
| `--view` | Open the streamed file in the interactive terminal viewer (default). |
| `--dump` | Print the streamed file to stdout instead of opening the viewer. |
| `--insecure-accept-host-key` | Trust an unknown SSH host key or FTPS certificate instead of aborting. |
| `--tls` | FTP only: `""` (plain), `explicit`, or `implicit`. |

**Password auth:** there is no `--password` flag. When password authentication is
used (SSH without `--key`, or any FTP connection), the CLI always prompts on the
terminal (`Password: `, no echo). The secret never appears in shell history or
process listings.

On first contact with an unknown SSH host key or FTPS certificate, the CLI
prints the fingerprint and prompts `Trust this host? (y/N)`; approving writes
it to `known_hosts` / `ftp_certs` and retries the connection.

### Interactive viewer keys

When a viewer is open, the following keys apply:

| Key | Action |
| --- | --- |
| `↑`/`↓`, `PgUp`/`PgDn`, `Home`/`End` | Scroll. |
| `/` | Start a search (`Alt+R` regex, `Alt+I` invert, `Alt+C` case). |
| `\` | Secondary AND-filter within current results. |
| `A` | **Advanced filters** form: Nearby, Exclude, Levels, Time, Line range, Context. |
| *(highlight)* | Match lines color primary (gold), AND (green), nearby (cyan), exclude (red); toggles in Settings. |
| `u` | **Top unique** — frequent match texts (jump with Enter). |
| `P` | **Search presets** — load / save / delete (shared `search_presets.json`). |
| `F2` / `,` | **Settings** — view mode, search defaults, context/nearby/top (shared `settings.json`). |
| `Y` | **Restore tabs** from the shared saved session (`saved_session.json`). |
| `G` | **Tab group** — name/color for the active tab (shown in the tab strip). |
| `T` | Open recent list to open selection **as a new tab**. |
| `t` | In recent picker: open highlighted entry as a **new tab**. |
| `Ctrl+Tab` / `Ctrl+PgDn` | Next tab. |
| `Ctrl+Shift+Tab` / `Ctrl+PgUp` | Previous tab. |
| `1`…`9` | Jump to tab N. |
| `Ctrl+W` | Close current tab (closing the last tab quits). |
| `n` / `p` | Next / previous match. |
| `[` / `]` | First / last match. |
| `r` | Toggle results-only (filter) view. |
| `s` / `m` | Split view / cycle view mode. |
| `f` | Toggle follow/tail mode. |
| `g` | Go to a line number. |
| `e` | Export (whole file or matches) to a file. |
| `?` | Show the full help overlay. |
| `q` / `Ctrl+C` | Quit. |

This mirrors the root command's own `Long` help, so `gofle --help` is always
authoritative.

## Configuration

Settings live in the OS user config directory under `GoFLE/`:

| Platform | Path |
| --- | --- |
| Linux | `~/.config/GoFLE/` |
| macOS | `~/Library/Application Support/GoFLE/` |
| Windows | `%APPDATA%\GoFLE\` |

Files written there:

| File | Contents |
| --- | --- |
| `settings.json` | Theme, language, save-tabs flag, window geometry, pane height, derived secret key. |
| `recent.json` | Recent files list (SSH usernames/passwords stored **encrypted**). |
| `saved_tabs.json` | Snapshot of open tabs (only when "save tabs on exit" is enabled). |
| `search_presets.json` | Named advanced-search presets (shared by GUI and CLI). |

Available settings (via the Settings dialog or `App.SaveSettings`):

| Setting | Values | Default |
| --- | --- | --- |
| `theme` | `dark`, `light`, `dracula`, `system` | `system` |
| `language` | `uk`, `en`, `system` | `system` |
| `save_tabs_exit` | `true` / `false` | `true` |
| `search_pane_height` | pixels | `250` |
| `window_width` / `window_height` / `window_maximized` | persisted on close | maximized |

---

## Security Notes

- **Local desktop app by design.** GoFLE is a single-user desktop tool; it assumes the
  local user is trusted.
- **SSH secret encryption.** SSH usernames/passwords are encrypted at rest with
  **AES-GCM** (never CFB or any non-authenticated mode) using a random 32-byte key
  (`SecretKey`) generated once and stored in `settings.json`. The key never leaves the
  machine. Decryption happens only to re-open a "Recent" SSH entry for the same user.
- **SSH host key verification.** Server host keys are verified against an OpenSSH
  `known_hosts` file (`~/.config/GoFLE/known_hosts`). On first connect to an unknown
  host, the app shows the SHA256 fingerprint and asks for explicit confirmation before
  persisting it; a key that changes after being trusted is treated as a potential MITM
  and rejected.
- **Temp files.** Remote logs are streamed to `os.CreateTemp("", "GoFLE-ssh-*.log")`
  and removed when the tab/stream is closed.
- **No telemetry.** The application does not phone home or collect data.

---

## Development

```bash
# Frontend dev server with hot reload (runs alongside the Go app)
cd frontend && npm install && npm run dev

# In another terminal, run the app wired to the dev server
wails dev
```

Useful commands:

```bash
wails generate module   # regenerate the wailsjs Go↔JS bridge after changing App methods
go test ./...           # run backend unit tests (indexer, search, config)
go vet ./...            # static analysis
golangci-lint run       # lint (also enforced in CI)
```

CI (`.github/workflows/ci.yml`) builds the frontend, runs `golangci-lint`, `go vet`,
and `go test ./...` on every push/PR.

---

## License

See the `LICENSE.md` file in the repository root for licensing details.

---

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for the
build, test, and pull-request workflow before opening an issue or a PR. It covers
the local/Wails and Docker (`build.sh`) build paths, `go test ./...`, `go vet ./...`,
and `golangci-lint`, and the CI setup.

## Contributors

Thanks to everyone who has reported issues, suggested features, and contributed
code and translations. A full list of contributors is maintained in the project's
GitHub contributors view. If you've contributed and would like to be named here,
open a PR against this section.
