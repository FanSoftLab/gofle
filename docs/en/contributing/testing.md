# Testing Guide

> **Content type**: Reference — testing strategies, tools, and patterns for GoFLE.
> **Audience**: Contributors
> **Verified**: 2026-10-29 — matches project practice

## Purpose

Document how to write, run, and debug tests for Go backend and TypeScript frontend.

---

## Test Organization

```
├── pkg/*/*_test.go           # Go unit/integration tests
├── cmd/gofle/*_test.go       # CLI tests
├── frontend/
│   ├── *.test.ts             # Unit tests (Vitest)
│   └── test/                 # Test utilities
└── .github/workflows/ci.yml  # CI pipeline
```

---

## Go Testing

### Tools

| Tool | Purpose |
|------|---------|
| `testing` | Standard library |
| `testify/assert` | Assertions |
| `testify/mock` | Mock interfaces |
| `testify/require` | Fail-fast assertions |
| `goleak` | Goroutine leak detection |
| `govulncheck` | Vulnerability scanning |

### Running Tests

```bash
# All packages
go test ./...

# Specific package
go test ./pkg/indexer -v

# With race detector
go test -race ./...

# Benchmarks
go test -bench=. ./pkg/search

# Coverage
go test -coverprofile=cover.out ./...
go tool cover -html=cover.out
```

### Test Patterns

#### Table-Driven Tests

```go
func TestSearchEngine_Search(t *testing.T) {
    tests := []struct {
        name     string
        req      SearchRequest
        want     *SearchResult
        wantErr  bool
    }{
        {
            name: "substring match",
            req:  SearchRequest{Query: "ERROR", Regex: false},
            want: &SearchResult{TotalMatches: 5},
        },
        {
            name: "regex match",
            req:  SearchRequest{Query: "ERR.*", Regex: true},
            want: &SearchResult{TotalMatches: 5},
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := engine.Search(tt.req)
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            assert.NoError(t, err)
            assert.Equal(t, tt.want.TotalMatches, result.TotalMatches)
        })
    }
}
```

#### Mocking Interfaces

```go
func TestApp_OpenSSHFile(t *testing.T) {
    mockStreamer := new(mocks.Streamer)
    mockStreamer.On("Stream", mock.Anything, "/remote/path", mock.Anything).Return(nil)
    
    app := &App{streamer: mockStreamer}
    err := app.OpenSSHFile(SSHConfig{RemotePath: "/remote/path"})
    
    assert.NoError(t, err)
    mockStreamer.AssertExpectations(t)
}
```

#### Goroutine Leak Detection

```go
func TestSearchEngine_NoLeaks(t *testing.T) {
    defer goleak.VerifyNone(t)
    // ... test that spawns goroutines
}
```

### Test Organization by Package

| Package | Focus |
|---------|-------|
| `pkg/indexer` | Index building, line access, tail update |
| `pkg/search` | Parallel search, regex/substring, filters |
| `pkg/config` | Load/save, encryption, recent files, session |
| `pkg/ssh` | Streamer/Lister, host key verification |
| `pkg/ftp` | Streamer/Lister, TLS modes, cert trust |
| `pkg/remote` | Interface contracts |
| `cmd/gofle` | CLI commands, viewer integration |

---

## TypeScript Testing

### Tools

| Tool | Purpose |
|------|---------|
| `vitest` | Unit test runner |
| `@testing-library/dom` | DOM testing utilities |
| `jsdom` | DOM environment |
| `happy-dom` | Faster DOM (configured) |

### Running Tests

```bash
cd frontend

# All tests
npm run test

# Watch mode
npm run test:watch

# Coverage
npm run test:coverage

# UI
npm run test:ui
```

### Test Patterns

#### Component Logic (Pure Functions)

```typescript
// log_highlighter.test.ts
import { highlight } from './log_highlighter'

describe('highlight', () => {
  it('wraps primary term in mark.hl-primary', () => {
    const result = highlight('ERROR: timeout', { primary: 'ERROR' })
    expect(result).toContain('<mark class="hl-primary">ERROR</mark>')
  })

  it('escapes regex special chars in substring mode', () => {
    const result = highlight('ERROR (timeout)', { primary: 'ERROR (timeout)' })
    expect(result).not.toContain('<mark class="hl-primary">ERROR (timeout)</mark>')
    // Should escape parens
  })
})
```

#### Store/State Tests

```typescript
// tab_manager.test.ts
import { createAppContext } from './app_context'

describe('TabManager', () => {
  let ctx: ReturnType<typeof createAppContext>

  beforeEach(() => {
    ctx = createAppContext()
  })

  it('creates tab and sets active', () => {
    const id = ctx.createTab({ type: 'local', path: '/test.log' })
    expect(ctx.tabs).toHaveLength(1)
    expect(ctx.activeTabId).toBe(id)
  })
})
```

#### i18n Tests

```typescript
// i18n.test.ts
import { t, setLang, getLang } from './i18n'

describe('i18n', () => {
  it('returns Ukrainian by default', () => {
    expect(getLang()).toBe('uk')
    expect(t('btn_open')).toBe('Відкрити')
  })

  it('switches to English', () => {
    setLang('en')
    expect(t('btn_open')).toBe('Open')
  })
})
```

### Test File Naming

- `*.test.ts` alongside source
- `test/` directory for utilities and complex setups

---

## Integration Tests

### SSH/FTP Live Integration

```bash
# Requires running SSH/FTP test servers
cd scripts/ssh-live-integration && go run main.go
cd scripts/ftp-live-integration && go run main.go
```

- Not run in CI (requires infrastructure)
- Run manually for release validation

### CLI Integration

```bash
# Build CLI
go build ./cmd/gofle

# Test viewer
./gofle /var/log/syslog
./gofle ssh host --remote /var/log/app.log --dump | head -5
```

---

## CI Pipeline

```yaml
# .github/workflows/ci.yml
jobs:
  build-test:
    steps:
      - Build frontend
      - go vet ./...
      - go test ./...
      - golangci-lint run
      - npm run test (frontend)
  govulncheck:
    - govulncheck ./...
```

### Pre-commit (Optional)

```bash
# Install
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
cd frontend && npm install -D husky && npx husky install

# .husky/pre-commit
golangci-lint run
cd frontend && npm run test
```

---

## Debugging Tests

### Go

```bash
# Verbose
go test -v ./pkg/search -run TestSearchEngine_Search

# Delve
dlv test ./pkg/search -- -run TestSearchEngine_Search

# Profile
go test -cpuprofile=cpu.prof -memprofile=mem.prof ./pkg/search
go tool pprof cpu.prof
```

### TypeScript

```bash
# Debug in VS Code: launch vitest
# Or: npx vitest --inspect-brk
```

---

## Coverage Targets

| Package | Target |
|---------|--------|
| `pkg/indexer` | > 90% |
| `pkg/search` | > 85% |
| `pkg/config` | > 80% |
| `pkg/ssh` | > 75% |
| `pkg/ftp` | > 75% |
| Frontend logic | > 70% |

---

## Related Information

- [Development Setup](dev-setup.md)
- [Coding Conventions](coding-conventions.md)
- [PR Workflow](pr-workflow.md)
- [Go Testing](https://go.dev/doc/tutorial/add-a-test)
- [Vitest](https://vitest.dev/guide/)