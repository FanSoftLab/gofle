# Third-Party Licenses

This document lists the open-source libraries and assets used by **Go Fast Log Explorer (GoFLE)**.

Application license: **MIT** — see [LICENSE.md](LICENSE.md).

---

## Direct dependencies (Go)

### github.com/wailsapp/wails/v2
- **Link:** https://github.com/wailsapp/wails
- **License:** MIT
- **Description:** Framework for building cross-platform desktop apps with Go and a web frontend (WebView).

### github.com/charmbracelet/bubbletea
- **Link:** https://github.com/charmbracelet/bubbletea
- **License:** MIT
- **Description:** Terminal UI framework (Elm Architecture) used by the CLI interactive viewer.

### github.com/charmbracelet/bubbles
- **Link:** https://github.com/charmbracelet/bubbles
- **License:** MIT
- **Description:** Reusable TUI components (text input, viewport, etc.) for Bubble Tea.

### github.com/charmbracelet/lipgloss
- **Link:** https://github.com/charmbracelet/lipgloss
- **License:** MIT
- **Description:** Style definitions and layout helpers for terminal output (colors, borders, padding).

### github.com/spf13/cobra
- **Link:** https://github.com/spf13/cobra
- **License:** Apache-2.0
- **Description:** CLI command structure, flags, subcommands, and help generation.

### github.com/jlaffaye/ftp
- **Link:** https://github.com/jlaffaye/ftp
- **License:** ISC
- **Description:** FTP/FTPS client for streaming remote log files.

### golang.org/x/crypto
- **Link:** https://pkg.go.dev/golang.org/x/crypto
- **License:** BSD-3-Clause
- **Description:** Cryptographic primitives; SSH client support for remote logs.

### golang.org/x/term
- **Link:** https://pkg.go.dev/golang.org/x/term
- **License:** BSD-3-Clause
- **Description:** Terminal state (raw mode, size) for the CLI viewer.

### golang.org/x/sync
- **Link:** https://pkg.go.dev/golang.org/x/sync
- **License:** BSD-3-Clause
- **Description:** Extended concurrency primitives (errgroup, singleflight, etc.).

### github.com/mattn/go-runewidth
- **Link:** https://github.com/mattn/go-runewidth
- **License:** MIT
- **Description:** Correct display width of Unicode strings in the terminal (CJK, emoji).

---

## Notable transitive dependencies (Go)

### github.com/muesli/termenv
- **Link:** https://github.com/muesli/termenv
- **License:** MIT
- **Description:** Terminal color profile detection and ANSI styling (used via Lip Gloss).

### github.com/lucasb-eyer/go-colorful
- **Link:** https://github.com/lucasb-eyer/go-colorful
- **License:** MIT
- **Description:** Color space conversions for terminal themes.

### github.com/rivo/uniseg
- **Link:** https://github.com/rivo/uniseg
- **License:** MIT
- **Description:** Unicode segmentation (grapheme clusters) for correct text width.

### github.com/atotto/clipboard
- **Link:** https://github.com/atotto/clipboard
- **License:** BSD-3-Clause
- **Description:** Cross-platform clipboard access (TUI components).

### github.com/gorilla/websocket
- **Link:** https://github.com/gorilla/websocket
- **License:** BSD-2-Clause
- **Description:** WebSocket protocol (Wails runtime / dev tooling).

### github.com/labstack/echo/v4
- **Link:** https://github.com/labstack/echo
- **License:** MIT
- **Description:** HTTP server used internally by the Wails asset/dev server stack.

### github.com/google/uuid
- **Link:** https://github.com/google/uuid
- **License:** BSD-3-Clause
- **Description:** UUID generation.

### github.com/samber/lo
- **Link:** https://github.com/samber/lo
- **License:** MIT
- **Description:** Generic helper utilities (slices, maps) used by Wails tooling.

### github.com/pkg/errors
- **Link:** https://github.com/pkg/errors
- **License:** BSD-2-Clause
- **Description:** Error wrapping helpers.

### github.com/pkg/browser
- **Link:** https://github.com/pkg/browser
- **License:** BSD-2-Clause
- **Description:** Open URLs in the system browser.

### golang.org/x/sys, golang.org/x/net, golang.org/x/text
- **Links:** https://pkg.go.dev/golang.org/x/sys · https://pkg.go.dev/golang.org/x/net · https://pkg.go.dev/golang.org/x/text
- **License:** BSD-3-Clause
- **Description:** Low-level OS, networking, and text support used across the stack.

---

## Frontend (build-time / bundled assets)

### Vite
- **Link:** https://vitejs.dev/
- **License:** MIT
- **Description:** Frontend build tool and dev server (TypeScript → production bundle).

### TypeScript
- **Link:** https://www.typescriptlang.org/
- **License:** Apache-2.0
- **Description:** Typed JavaScript used for the desktop UI source.

### JetBrains Mono (font)
- **Link:** https://www.jetbrains.com/lp/mono/
- **License:** SIL Open Font License 1.1
- **Description:** Monospace font bundled for the log viewer.

### Fira Code (font)
- **Link:** https://github.com/tonsky/FiraCode
- **License:** SIL Open Font License 1.1
- **Description:** Monospace font with programming ligatures (optional viewer font).

### Cascadia Code (font)
- **Link:** https://github.com/microsoft/cascadia-code
- **License:** SIL Open Font License 1.1
- **Description:** Monospace font from Microsoft (optional viewer font).

---

## Notes

- Only **direct** and selected **runtime-relevant** transitive dependencies are listed above.
- Full module graph: `go mod graph` / `go list -m all`.
- Frontend production bundle ships compiled JS/CSS; npm packages such as Vite and TypeScript are not required at runtime.
- Font Awesome (`@fortawesome/fontawesome-free`) appears only as a optional/dev dependency; the UI uses inline SVG icons.
