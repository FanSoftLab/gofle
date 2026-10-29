# GoFLE Documentation

**Go Fast Log Explorer** — cross-platform desktop application for browsing, searching, and tailing very large log files and remote (SSH/FTP) logs.

---

## Audience

| Role | Description | Recommended Starting Point |
|------|-------------|---------------------------|
| **SRE / DevOps Engineer** | Tailing production logs, debugging incidents, remote server access | [Getting Started](../getting-started/prerequisites.md) → [User Guide](../user-guide/open-files.md) |
| **Developer** | Inspecting local debug output, large test/trace logs | [Getting Started](../getting-started/prerequisites.md) → [Search Basics](../user-guide/search-basics.md) |
| **General User** | Interactive grep-like tool for multi-GB files without terminal | [Getting Started](../getting-started/first-run.md) → [Keyboard Shortcuts](../user-guide/keyboard-shortcuts.md) |
| **CLI User** | Terminal companion `gofle` for quick view/search without GUI | [CLI Reference](../cli-reference/cli-overview.md) |
| **Contributor** | Building, testing, extending GoFLE | [Contributing](../contributing/dev-setup.md) |

---

## Scope

GoFLE is a **read-oriented log viewer** designed for:

- Opening files of any size (hundreds of MB to GB) instantly via line-offset index
- Live tail/follow of local and remote logs
- Parallel substring/regex search with advanced filters (Nearby, Levels, Time, Exclude, Range, Top)
- Multiple tabs with colored groups and session restore
- Remote access over SSH and FTP/FTPS (explicit/implicit) with certificate pinning
- Terminal companion (`gofle`) reusing the same Go backend

GoFLE is **intentionally not**:

- A log aggregator or collector (no ingestion, no indexing server, no agents)
- A text editor (cannot modify logs — only read, search, copy, export)
- A structured log parser/analyzer (search is line-based)

---

## Quick Links

| Task | Documentation |
|------|---------------|
| Install and run | [Getting Started](../getting-started/prerequisites.md) |
| Open a local file | [Open Files](../user-guide/open-files.md) |
| Connect via SSH | [Open Remote (SSH)](../user-guide/open-remote-ssh.md) |
| Connect via FTP/FTPS | [Open Remote (FTP/FTPS)](../user-guide/open-remote-ftp.md) |
| Search with regex | [Search Basics](../user-guide/search-basics.md) |
| Use advanced filters | [Advanced Search](../user-guide/search-advanced.md) |
| Enable live tail | [Follow (Live Tail)](../user-guide/follow-tail.md) |
| Organize with tab groups | [Tabs and Groups](../user-guide/tabs-groups.md) |
| Configure settings | [Settings UI](../configuration/settings-ui.md) |
| Use the CLI | [CLI Overview](../cli-reference/cli-overview.md) |
| Troubleshoot SSH | [SSH Connection](../troubleshooting/ssh-connection.md) |
| View all shortcuts | [Keyboard Shortcuts](../user-guide/keyboard-shortcuts.md) |

---

## Architecture Overview

GoFLE follows a **thin-backend / thin-frontend** model (Wails v2):

```
┌─────────────────────────────────────────────────────────────────┐
│  Frontend (TypeScript + Vite)                                   │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐            │
│  │VirtualScroller│ │ TabManager   │ │ Modal/SSH/   │            │
│  │ (rendering)   │ │ (tabs/groups)│ │ ContextMenu  │            │
│  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘            │
└─────────┼────────────────┼────────────────┼────────────────────┘
          │  wailsjs bridge (generated)   │
┌─────────┼────────────────┼────────────────┼────────────────────┐
│  Go Backend (bound to frontend)                              │
│  ┌──────▼───────┐ ┌─────▼───────┐ ┌─────▼───────┐ ┌────────▼──┐│
│  │   app.go     │ │ pkg/indexer │ │ pkg/search  │ │ pkg/ssh   ││
│  │ (App, tabs,  │ │ (offset     │ │ (parallel   │ │ (SSH      ││
│  │  polling,    │──▶│  index,     │──▶│  regex/     │ │  streamer)││
│  │  orchestr.)  │ │  tail)      │ │  substring) │ │           ││
│  └──────────────┘ └─────────────┘ └─────────────┘ └───────────┘│
│         │                │                │                      │
│         └────────────────┴────────────────┘                      │
│                      ┌───────▼────────┐                          │
│                      │  pkg/config    │◄── settings, recent,     │
│                      │ (settings,     │    tabs, AES-GCM secrets │
│                      │  recent, tabs) │                          │
│                      └────────────────┘                          │
└─────────────────────────────────────────────────────────────────┘
```

Key packages:
- `pkg/indexer` — byte-offset index, `pread` random access, incremental tail
- `pkg/search` — parallel worker pool, regex/substring, LRU compiled-regex cache
- `pkg/remote` — shared `Streamer`/`Lister` interfaces (implemented by `pkg/ssh`, `pkg/ftp`)
- `pkg/config` — persisted JSON + AES-GCM encrypted secrets
- `pkg/logging` — centralized `slog` for desktop binary and `gofle` CLI

---

## Documentation Structure

This documentation follows **ISO/IEC/IEEE 26514** (structure, completeness) and **ASD-STE100** (controlled technical English).

| Section | Content Type | Purpose |
|---------|--------------|---------|
| [Overview](index.md) | Concept | What GoFLE is, who it's for, architecture |
| [Getting Started](../getting-started/) | Task | Prerequisites, installation, first run |
| [User Guide](../user-guide/) | Task | All user workflows (open, search, follow, tabs, export) |
| [Configuration](../configuration/) | Reference | Settings, config files, complete settings table |
| [CLI Reference](../cli-reference/) | Reference | `gofle` commands, flags, viewer keybindings |
| [Troubleshooting](../troubleshooting/) | Troubleshooting | Symptom → cause → resolution for common issues |
| [API Reference](../api-reference/) | Reference | Wails bridge, Go packages, frontend modules |
| [Release](../release/) | Release | Changelog, release process, migration guides |
| [Contributing](../contributing/) | Task/Reference | Dev setup, conventions, testing, PR workflow |

---

## Terminology

All documentation uses the [project glossary](../glossary.md) for consistent bilingual (UK/EN) terminology.

Key terms:
- **follow** / **слідкувати** — live tailing (not "tail", not "хвіст")
- **line-offset index** / **індекс зміщень рядків** — in-memory line position array
- **results pane** / **панель результатів** — search results panel
- **advanced filters bar** / **панель розширених фільтрів** — filter chips row
- **search preset** / **пресет пошуку** — saved filter combination
- **tab group** / **група вкладок** — colored tab collection
- **config directory** / **директорія конфігурації** — OS-specific settings folder
- **terminal viewer** / **термінальний переглядач** — `gofle` CLI interactive viewer

---

## Version

This documentation corresponds to GoFLE **v2.x**. See [Changelog](../release/changelog.md) for version history.