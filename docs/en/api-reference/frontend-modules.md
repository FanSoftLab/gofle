# Frontend Modules Reference

> **Content type**: Reference — TypeScript modules in `frontend/src/`.
> **Audience**: Frontend developers, contributors
> **Verified**: 2026-10-29 — matches source code

## Purpose

Document frontend architecture, module responsibilities, and key APIs for contributors.

---

## Module Overview

```
frontend/src/
├── main.ts                 # Composition root: wiring, shortcuts, events
├── app_context.ts          # Shared types: Tab, Group, AppContext
├── virtual_scroller.ts     # Windowed list renderer (lazy line fetching)
├── tab_manager.ts          # Tab & group lifecycle, UI
├── modal_controller.ts     # Recent / SSH / Group / GoTo / Settings modals
├── ssh_handler.ts          # SSH connection form logic
├── settings_manager.ts     # Theme / language / save-tabs persistence
├── context_menu.ts         # Right-click copy / goto / export
├── log_highlighter.ts      # Regex/term highlighting in rows
├── i18n.ts                 # Ukrainian & English translations
├── style.css               # Themes (dark/light/dracula)
├── help_content.ts         # Structured in-app help (bilingual)
├── help_modal.ts           # Help window (F1) with search
├── toast.ts                # Toast notifications
├── confirm_dialog.ts       # Reusable confirmation dialog
├── input_history.ts        # Search input history management
├── search_advanced.ts      # Advanced filters bar (chips + forms)
├── web_client.ts           # WebSocket client for -web mode
├── vite-env.d.ts           # Vite type declarations
├── wails.d.ts              # Wails runtime types
└── wailsjs/                # Auto-generated Go↔JS bridge
    ├── go/models.ts
    └── go/main/App.d.ts
```

---

## Core Modules

### `main.ts` — Composition Root

**Responsibilities**:
- Initialize all managers
- Wire Wails event listeners (`file_updated`, `search_progress`, etc.)
- Register global keyboard shortcuts
- Bootstrap i18n, theme, settings
- Handle `-web` mode WebSocket server

**Key Exports**:
```typescript
function initApp(): Promise<void>
function registerShortcuts(): void
function setupEventListeners(): void
```

---

### `app_context.ts` — Shared Types & State

**Responsibilities**:
- Define TypeScript interfaces for Tab, Group, AppContext
- Provide reactive state (Vue-like reactivity via proxies)
- Single source of truth for UI state

**Key Types**:
```typescript
interface Tab {
  id: string
  name: string
  path: string
  type: 'local' | 'ssh' | 'ftp'
  group: string
  active: boolean
  scrollPos: number
  follow: boolean
  searchQuery: string
  // ... remote config fields
}

interface Group {
  name: string
  color: string
  tabs: string[]
  collapsed: boolean
}

interface AppContext {
  tabs: Tab[]
  groups: Group[]
  activeTabId: string | null
  settings: Settings
  // ... methods for mutations
}
```

---

### `tab_manager.ts` — Tab & Group Lifecycle

**Responsibilities**:
- Tab CRUD: create, close, rename, reorder
- Group management: create, update, delete, collapse/expand
- Tab activation, drag-to-reorder
- Session persistence integration
- Context menu for tabs/groups

**Key Methods**:
```typescript
function createTab(config: TabConfig): string // returns tabId
function closeTab(tabId: string): void
function activateTab(tabId: string): void
function renameTab(tabId: string, name: string): void
function moveTab(tabId: string, direction: 'left' | 'right'): void
function createGroup(name: string, color: string): void
function updateGroup(name: string, color: string): void
function deleteGroup(name: string): void
function assignTabToGroup(tabId: string, groupName: string): void
function removeTabFromGroup(tabId: string): void
function saveSession(): void
function restoreSession(): void
```

---

### `virtual_scroller.ts` — Windowed List Renderer

**Responsibilities**:
- Render only visible log/result rows (DOM recycling)
- Fetch lines from backend via `GetLogLines` / `GetSearchMatches`
- Handle variable row heights (word wrap)
- Sync scroll position with line numbers
- Keyboard navigation (Home, End, PgUp, PgDn)

**Key Methods**:
```typescript
function setDataSource(getLines: (start, count) => Promise<LogLine[]>): void
function setLineCount(count: number): void
function scrollToLine(lineNum: number): void
function getVisibleRange(): { start: number; end: number }
function refresh(): void
```

**Performance**:
- Only ~50-100 DOM nodes regardless of file size
- Lazy fetch on scroll (debounced)
- Row height caching for word wrap

---

### `modal_controller.ts` — Modal Management

**Responsibilities**:
- Manage modal stack (only one open at a time)
- Recent files modal (Local/SSH/FTP tabs)
- SSH/FTP connection forms
- Tab group editor
- Go to line dialog
- Settings modal (5 tabs)
- Keyboard trap (Esc to close, Tab cycling)

**Key Methods**:
```typescript
function openModal(type: ModalType, data?: any): void
function closeModal(): void
function openRecentModal(): void
function openSSHModal(): void
function openFTPModal(): void
function openGroupModal(group?: Group): void
function openGotoModal(): void
function openSettingsModal(): void
```

---

### `ssh_handler.ts` — SSH Connection Form

**Responsibilities**:
- SSH form state: host, port, user, auth type, key/password
- Remote directory browser (sftp via backend)
- Host key verification dialog
- Form validation
- Connection progress (connecting → downloading → indexing)

**Key Methods**:
```typescript
function connect(config: SSHConfig): Promise<void>
function browseDirectory(path: string): Promise<RemoteEntry[]>
function validateForm(): boolean
```

---

### `settings_manager.ts` — Settings Persistence

**Responsibilities**:
- Load/save settings to backend (`GetSettings`, `SaveSettings`)
- Theme switching (CSS custom properties)
- Language switching (i18n)
- Save-tabs persistence
- Settings UI binding (5 tabs: Appearance, Viewer, Search, Session, Remote)

**Key Methods**:
```typescript
function loadSettings(): Promise<void>
function saveSettings(partial: Partial<Settings>): Promise<void>
function applyTheme(theme: string): void
function applyLanguage(lang: string): void
```

---

### `i18n.ts` — Internationalization

**Responsibilities**:
- Bilingual translations (Ukrainian/English)
- Runtime language switching
- DOM attribute-based translation (`data-i18n`, `data-i18n-title`, `data-i18n-placeholder`)
- Plural forms support

**Key Exports**:
```typescript
type LangCode = 'uk' | 'en'
const translations: Record<LangCode, Record<string, string>>
function setLang(lang: LangCode): void
function getLang(): LangCode
function resolveLang(raw: string): LangCode
function t(key: string, params?: Record<string, string|number>): string
function applyI18n(): void
```

**Usage in HTML**:
```html
<button data-i18n="btn_open" data-i18n-title="btn_open_title"></button>
<input data-i18n-placeholder="search_placeholder">
```

---

### `log_highlighter.ts` — Term Highlighting

**Responsibilities**:
- Highlight primary, AND, nearby, exclude terms in log rows
- Regex-aware highlighting (escape special chars for substring)
- Color classes: `hl-primary`, `hl-secondary`, `hl-nearby`, `hl-exclude`
- Toggle via Settings → Search → Highlight toggles

**Key Methods**:
```typescript
function highlight(text: string, terms: HighlightTerms): string
// Returns HTML with <mark class="hl-*"> wrapped terms
```

---

### `search_advanced.ts` — Advanced Filters Bar

**Responsibilities**:
- Render filter chips: Nearby, Levels, Time, Exclude, Range, Top, Presets
- Manage chip state (active, form open)
- Form validation and Apply/Clear
- Preset save/load UI

**Key Methods**:
```typescript
function renderChips(): void
function openFilter(filter: FilterType): void
function applyFilter(filter: FilterType): void
function clearFilter(filter: FilterType): void
function savePreset(name: string): void
function loadPreset(name: string): void
```

---

### `help_content.ts` — In-App Help Content

**Responsibilities**:
- Structured bilingual help (10 sections, 40+ topics)
- Topic structure: title, body[], example[]
- Search/filter within help modal
- License entries

**Key Exports**:
```typescript
const HELP_SECTIONS: HelpSection[]
const LICENSE_ENTRIES: LicenseEntry[]
type HelpLang = 'uk' | 'en'
function helpLangFromApp(lang: string): HelpLang
```

---

### `help_modal.ts` — Help Window (F1)

**Responsibilities**:
- Render help sections as tabs
- Search within help (highlight matches)
- Language follows app language
- Keyboard navigation (Esc to close)

---

### `context_menu.ts` — Right-Click Menus

**Responsibilities**:
- Log pane: copy line/number, goto line, add to search, export
- Results pane: copy, goto, export
- Tab bar: settings, close, group, rename, browse server
- Group label: rename, color, collapse, close
- Dynamic plural forms (single vs multi-select)

---

### `toast.ts` — Toast Notifications

**Responsibilities**:
- Non-blocking notifications (success, error, info)
- Auto-dismiss after 3s
- Stack multiple toasts

```typescript
function showToast(message: string, type: 'success' | 'error' | 'info'): void
```

---

### `confirm_dialog.ts` — Confirmation Dialog

**Responsibilities**:
- Reusable modal: title, message, confirm/cancel
- Promise-based API
- Used for: close remote tab, delete group, clear history

```typescript
function confirm(title: string, message: string): Promise<boolean>
```

---

### `input_history.ts` — Search Input History

**Responsibilities**:
- Per-field history (primary, AND, nearby, exclude)
- Up/Down navigation in input
- Configurable depth (Settings → Search → History size)
- Persist to `settings.json`

---

### `web_client.ts` — WebSocket Client (-web mode)

**Responsibilities**:
- Connect to GoFLE `-web` HTTP server
- Handle WebSocket messages: log lines, search results, progress
- Reconnection logic
- File upload via HTTP (browser file picker)

---

### `wailsjs/go/models.ts` — Generated Types

**Auto-generated** from Go structs via `wails generate module`.

Contains TypeScript interfaces for:
- All `SearchRequest` / `SearchResult` types
- `TabInfo`, `TabGroup`, `FileInfo`, `RemoteEntry`
- `Settings`, `RecentEntry`, `SavedSession`
- `SSHFileConfig`, `FTPFileConfig`

**Do not edit manually** — regenerate after Go struct changes.

---

### `wailsjs/go/main/App.d.ts` — Generated Bridge

**Auto-generated** TypeScript declarations for `window.go.main.App.*` methods.

Matches `pkg/...` Go method signatures exactly.

**Do not edit manually** — regenerate after Go method changes.

---

## State Flow

```
User Action
    │
    ▼
Component (tab_manager, modal_controller, etc.)
    │
    ├──→ Local State Update (reactive)
    │
    └──→ Backend Call (window.go.main.App.*)
            │
            ▼
        Go Backend (app.go, pkg/*)
            │
            ▼
        Runtime Event (file_updated, search_complete, etc.)
            │
            ▼
        Frontend Event Listener (main.ts)
            │
            ▼
        State Update → UI Re-render
```

---

## Related Information

- [Wails Bridge](wails-bridge.md)
- [Go Packages](go-packages.md)
- [Frontend Source](https://github.com/sankam-nikolya/GoFLE/tree/main/frontend/src)