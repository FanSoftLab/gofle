# Performance Troubleshooting

> **Content type**: Troubleshooting — diagnose and resolve performance issues.
> **Audience**: All users
> **Severity**: Medium
> **Verified**: 2026-10-29 — tested with files up to 10 GB

## Purpose

Resolve slow indexing, high memory usage, UI lag, and search performance issues.

---

## Symptoms Index

| Symptom | Jump To |
|---------|---------|
| Slow initial file indexing | [Slow Indexing](#slow-indexing) |
| High memory usage | [High Memory Usage](#high-memory-usage) |
| UI lag / stuttering scroll | [UI Lag](#ui-lag) |
| Slow search | [Slow Search](#slow-search) |
| Follow mode CPU spike | [Follow CPU Spike](#follow-cpu-spike) |
| File opens but empty | [File Opens Empty](#file-opens-empty) |
| Remote download stalls | [Remote Download Stalls](#remote-download-stalls) |

---

## Slow Indexing

**Symptom**: "Indexing..." takes long time on file open

### Conditions

- Very large file (GB+)
- Slow storage (HDD, network mount)
- First open (no index cache)

### Expected Behavior

- Indexing is O(file size) but memory-light (only line offsets)
- Typical: ~100-500 MB/s on SSD, ~50-100 MB/s on HDD
- 1 GB file: 2-10 seconds typical

### Possible Causes

| Cause | Verified |
|-------|----------|
| File size > 1 GB | Yes |
| Slow storage (HDD, NFS, SMB) | Yes |
| File on compressed filesystem | Yes |
| CPU throttling | Possible |

### Diagnosis

1. Check file size: `ls -lh /path/file`
2. Check storage type: `df -T /path` / `mount | grep /path`
3. Test raw read speed: `dd if=/path/file of=/dev/null bs=1M count=100`
4. Monitor CPU during indexing: `top -p $(pgrep GoFLE)`

### Resolution

1. Accept indexing time for large files (one-time per session)
2. Use Range filter: "Last N lines" to avoid full indexing
3. Move file to faster storage (local SSD)
4. For network mounts: copy locally first

---

## High Memory Usage

**Symptom**: GoFLE process uses excessive RAM

### Expected Behavior

- Line-offset index: ~8 bytes per line (int64)
- 10M lines ≈ 80 MB index
- Log content NOT loaded — only visible lines via `pread`
- Typical total: 100-300 MB for large files

### Possible Causes

| Cause | Verified |
|-------|----------|
| Many tabs open (each has index) | Yes |
| Search results cached (large matches) | Yes |
| Frontend DOM bloat (word wrap on huge files) | Yes |
| Memory leak (bug) | Possible |

### Diagnosis

1. Check process memory: `ps aux | grep GoFLE` or `htop`
2. Count open tabs — each tab = separate index
3. Check if Word Wrap enabled (Settings → Viewer)
4. Check search results count (status bar)

### Resolution

1. Close unused tabs (each tab holds index in memory)
2. Disable Word Wrap for huge files (Settings → Viewer)
3. Clear search results (Esc) when not needed
4. Restart GoFLE if memory grows unbounded (report bug)

---

## UI Lag

**Symptom**: Stuttering scroll, delayed keystrokes, frozen UI

### Conditions

- Word Wrap enabled on huge files
- Many matches highlighted
- Results pane with many rows
- Low-end GPU / no hardware acceleration

### Possible Causes

| Cause | Verified |
|-------|----------|
| Word Wrap on large file | Yes |
| Many highlighted terms | Yes |
| Results pane: thousands of rows | Yes |
| WebView rendering bottleneck | Yes |

### Diagnosis

1. Disable Word Wrap: Settings → Viewer → Word Wrap → Off
2. Clear search results: Esc
3. Reduce results pane height
4. Check GPU acceleration: Chrome/Edge `chrome://gpu` (WebView uses same)

### Resolution

1. **Disable Word Wrap** — biggest impact for huge files
2. Use "Log only" view mode (hide results pane)
3. Reduce line density: Settings → Appearance → Compact
4. Close results pane when not searching

---

## Slow Search

**Symptom**: Search takes seconds to return results

### Expected Behavior

- Parallel worker pool (CPU cores)
- Regex cached (LRU) — second search faster
- Typical: < 1s for 100M lines on modern CPU

### Possible Causes

| Cause | Verified |
|-------|----------|
| First regex search (compile) | Yes |
| Complex regex (backtracking) | Yes |
| Huge file, no Range filter | Yes |
| CPU contention | Possible |

### Diagnosis

1. Repeat same search — should be faster (cached regex)
2. Try simpler pattern (substring vs regex)
3. Enable Range filter: "Last N lines"
4. Check CPU usage during search

### Resolution

1. Use substring (default) unless regex needed
2. Enable Regex only when needed (Alt+R)
3. Use Range filter for huge files
4. Simplify regex: avoid `.*`, use specific patterns

---

## Follow CPU Spike

**Symptom**: High CPU when Follow enabled on active log

### Conditions

- Very high log rate (1000+ lines/sec)
- Follow polls every 500ms (local) / 2s (SSH) / 30s (FTP)
- Regex search active + follow

### Possible Causes

| Cause | Verified |
|-------|----------|
| High log write rate | Yes |
| Search + follow combined | Yes |
| Regex re-evaluation on new lines | Yes |

### Diagnosis

1. Disable Follow — CPU drops?
2. Disable search — CPU drops?
3. Check log rate: `wc -l` over 10s interval

### Resolution

1. Disable Follow for high-rate logs
2. Use Range: "Last N lines" instead of follow
3. Disable regex during follow
4. Increase poll interval (not currently configurable)

---

## File Opens Empty

**Symptom**: File opens, indexing completes, but log pane shows no lines

### Conditions

- Binary file (non-UTF8)
- Empty file
- Encoding issue
- Permission: readable but no content

### Possible Causes

| Cause | Verified |
|-------|----------|
| Binary file (null bytes) | Yes |
| Empty file (0 bytes) | Yes |
| UTF-8 decode error | Yes |
| File truncated during index | Possible |

### Diagnosis

1. Check file: `file /path/file` — shows "data" for binary
2. Check size: `ls -lh /path/file`
3. Check encoding: `head -c 100 /path/file | xxd`
4. Try `cat /path/file` — see raw content

### Resolution

1. Binary files: GoFLE shows replacement chars (�) — expected
2. Empty file: verify source writes to it
3. Encoding: GoFLE reads as UTF-8; non-UTF8 shows replacement chars
4. Re-index: close tab, reopen

---

## Remote Download Stalls

**Symptom**: "Downloading log..." stalls, then fails or continues slowly

### Conditions

- SSH/FTP large file
- Network instability
- Server throttling
- Stall timeout (default 30s no bytes)

### Possible Causes

| Cause | Verified |
|-------|----------|
| Network packet loss | Yes |
| Server rate limiting | Yes |
| Stall timeout too low | Yes |
| Large file > 1 GB | Yes |

### Diagnosis

1. Check GoFLE log for "stall timeout" messages
2. Test manual download: `scp user@host:/path/file .` or `wget ftp://...`
3. Monitor network: `ping host` during download

### Resolution

1. Increase stall timeout: Settings → Remote → Download Silence Timeout
2. Use Range: "Last N lines" for huge remote files
3. Improve network path (VPN, direct connection)
4. For SSH: ensure server not throttling `cat`/`tail`

---

## Related Information

- [Open Files](../user-guide/open-files.md)
- [Follow (Live Tail)](../user-guide/follow-tail.md)
- [Search Basics](../user-guide/search-basics.md)
- [Advanced Search](../user-guide/search-advanced.md)
- [Remote Settings](../configuration/settings-ui.md#remote-settings)