# Coding Conventions

> **Content type**: Reference — Go and TypeScript style guide for GoFLE.
> **Audience**: Contributors
> **Verified**: 2026-10-29 — matches project practice

## Purpose

Consistent code style across Go backend and TypeScript frontend.

---

## Go Conventions

### Formatting

- `gofmt` / `gofumpt` — standard formatting
- Line length: **no hard limit** (prefer < 120 cols)
- `goimports` for import organization

### Naming

| Element | Convention | Example |
|---------|------------|---------|
| Packages | lowercase, single word | `indexer`, `search` |
| Types | PascalCase | `FileIndexer`, `SearchRequest` |
| Interfaces | PascalCase, often `-er` suffix | `Streamer`, `Lister` |
| Functions/Methods | PascalCase (exported), camelCase (unexported) | `BuildIndex`, `updateIndex` |
| Variables | camelCase | `lineCount`, `sshConfig` |
| Constants | PascalCase or UPPER_SNAKE | `DefaultTimeout`, `MAX_LINES` |
| Error types | `*Error` suffix | `CertMismatchError` |
| Constructor | `New` + Type | `NewFileIndexer` |

### Error Handling

- Return `error` as last return value
- Wrap with `fmt.Errorf("context: %w", err)` for chain
- Use sentinel errors for expected failures: `var ErrNotFound = errors.New("not found")`
- Custom error types for actionable errors (retry, auth, etc.)
- Never ignore errors — use `_` only when intentionally discarded

### Concurrency

- Protect shared state with `sync.RWMutex` (see `App`, `FileIndexer`)
- Prefer channels for ownership transfer
- Use `errgroup` for parallel operations with cancellation
- `context.Context` for cancellation/timeout propagation

### Testing

- Table-driven tests for multiple cases
- `testify/assert` for assertions
- `testify/mock` for interfaces
- Test file: `*_test.go` in same package
- Benchmark: `*_bench_test.go`

### Documentation

```go
// FileIndexer builds a byte-offset index for random line access.
// It uses pread for O(1) line reads without loading the full file.
type FileIndexer struct { ... }

// NewFileIndexer creates an indexer for the given file path.
// Returns error if file cannot be opened.
func NewFileIndexer(path string) (*FileIndexer, error) { ... }
```

- Every **exported** identifier has godoc
- Start with identifier name
- Explain **why** / behavior, not just what
- Document side effects, concurrency, limitations

---

## TypeScript Conventions

### Formatting

- `prettier` (configured in `frontend/package.json`)
- 2 spaces, single quotes, trailing commas
- Max line: 100 chars

### Naming

| Element | Convention | Example |
|---------|------------|---------|
| Files | kebab-case | `tab_manager.ts` |
| Classes | PascalCase | `TabManager` |
| Interfaces | PascalCase | `TabConfig` |
| Types | PascalCase | `SearchRequest` |
| Functions/Methods | camelCase | `createTab`, `getTabs` |
| Variables | camelCase | `activeTabId` |
| Constants | UPPER_SNAKE | `DEFAULT_TIMEOUT` |
| CSS Classes | kebab-case | `.tab-bar`, `.hl-primary` |
| Data Attributes | kebab-case | `data-i18n`, `data-tab-id` |

### Types

- Use generated `wailsjs/go/models.ts` for backend contracts
- Define local types in `app_context.ts` for UI state
- Prefer `interface` over `type` for object shapes
- Discriminated unions for state:
  ```typescript
  type TabType = 'local' | 'ssh' | 'ftp'
  ```

### Reactivity

- `app_context.ts` uses Proxy-based reactivity
- Mutate via methods, not direct property assignment
- Components subscribe via `watch` / `onMounted`

### Event Handling

- Backend → Frontend: Wails runtime events (`runtime.EventsOn`)
- Frontend → Backend: Direct `window.go.main.App.*` calls
- Component communication: Shared `AppContext` methods

### CSS

- CSS Custom Properties for theming (`--color-bg`, `--font-family`)
- Themes in `style.css`: `[data-theme="dark"]`, `[data-theme="light"]`
- Scoped styles per component (CSS Modules not used — global with prefixes)
- Utility classes: `.flex`, `.hidden`, `.sr-only`

### i18n

- All user-facing strings in `i18n.ts`
- HTML: `data-i18n="key"`, `data-i18n-title="key"`, `data-i18n-placeholder="key"`
- TS: `t('key', { param: value })`
- Plural forms: separate keys (`key`, `key_plural`)

---

## Git Conventions

### Branch Naming

| Type | Pattern | Example |
|------|---------|---------|
| Feature | `feat/short-description` | `feat/ftp-transport` |
| Fix | `fix/short-description` | `fix/ssh-timeout` |
| Refactor | `refactor/short-description` | `refactor/search-engine` |
| Docs | `docs/short-description` | `docs/cli-reference` |
| Chore | `chore/short-description` | `chore/update-deps` |

### Commit Messages

```
<type>(<scope>): <subject>

<body>

<footer>
```

- **Subject**: Imperative, lowercase, no period, ≤ 72 chars
- **Body**: Why, not what (optional)
- **Footer**: Breaking changes, issue refs

---

## Code Review Checklist

- [ ] Go: `gofmt`, `go vet`, `golangci-lint` pass
- [ ] TS: `prettier`, `tsc --noEmit`, `npm run test` pass
- [ ] Tests added/updated for new behavior
- [ ] Godoc for exported Go identifiers
- [ ] Types for TS backend contracts (use generated)
- [ ] No plaintext secrets in code or logs
- [ ] Concurrency safe (mutexes, contexts)
- [ ] Error handling complete (no ignored errors)
- [ ] i18n keys for new UI strings
- [ ] CHANGELOG entry if user-visible (conventional commit)

---

## Related Information

- [Development Setup](dev-setup.md)
- [Testing](testing.md)
- [PR Workflow](pr-workflow.md)
- [Go Style Guide](https://go.dev/doc/effective_go)
- [TypeScript Style Guide](https://typescript-eslint.io/rules/)