# Troubleshooting Index

> **Content type**: Reference — symptom-based navigation to troubleshooting topics.
> **Audience**: All users
> **Verified**: 2026-10-29

## Purpose

Quick lookup: find the troubleshooting topic matching your symptom.

---

## By Symptom

| Symptom | Topic |
|---------|-------|
| Cannot connect to SSH server | [SSH Connection](ssh-connection.md) |
| SSH host key verification fails | [SSH Connection](ssh-connection.md#host-key-verification-failed) |
| SSH authentication fails | [SSH Connection](ssh-connection.md#authentication-failed) |
| SSH connection times out | [SSH Connection](ssh-connection.md#connection-timeout) |
| SSH follow not updating | [SSH Connection](ssh-connection.md#follow-not-updating) |
| Cannot connect to FTP server | [FTP Connection](ftp-connection.md) |
| FTPS certificate trust fails | [FTP Connection](ftp-connection.md#certificate-trust-failed) |
| FTP TLS handshake fails | [FTP Connection](ftp-connection.md#tls-handshake-failed) |
| FTP follow not updating | [FTP Connection](ftp-connection.md#follow-not-updating) |
| File opens but empty | [Performance](performance.md#file-opens-empty) |
| High memory usage | [Performance](performance.md#high-memory-usage) |
| Slow indexing | [Performance](performance.md#slow-indexing) |
| Search returns no results | [Search Issues](search-issues.md#no-results) |
| Regex error | [Search Issues](search-issues.md#regex-error) |
| Advanced filters not working | [Search Issues](search-issues.md#advanced-filters-not-working) |
| UI not rendering correctly | [UI Issues](ui-issues.md) |
| Shortcuts not working | [UI Issues](ui-issues.md#shortcuts-not-working) |
| Theme not applying | [UI Issues](ui-issues.md#theme-not-applying) |
| Tabs not restored | [Data Recovery](data-recovery.md#tabs-not-restored) |
| Config corrupted | [Data Recovery](data-recovery.md#config-corrupted) |
| Lost SSH credentials | [Data Recovery](data-recovery.md#lost-ssh-credentials) |

---

## By Component

| Component | Topics |
|-----------|--------|
| **SSH** | [SSH Connection](ssh-connection.md) |
| **FTP/FTPS** | [FTP Connection](ftp-connection.md) |
| **Performance** | [Performance](performance.md) |
| **Search** | [Search Issues](search-issues.md) |
| **UI/Frontend** | [UI Issues](ui-issues.md) |
| **Data/Config** | [Data Recovery](data-recovery.md) |

---

## General Diagnostic Steps

1. **Check logs**: `~/.config/gofle/gofle.log` (or platform equivalent)
2. **Verify version**: `gofle version` or GUI Help → About
3. **Test minimal case**: Open a small local file
4. **Check system requirements**: [Prerequisites](../getting-started/prerequisites.md)
5. **Try Docker build**: `./build.sh` (isolates environment)

---

## Log Locations

| Platform | Log File |
|----------|----------|
| Linux | `~/.config/gofle/gofle.log` |
| macOS | `~/Library/Application Support/gofle/gofle.log` |
| Windows | `%APPDATA%\gofle\gofle.log` |

Log level: `INFO` by default. Set `GOFLE_LOG_LEVEL=debug` for more detail.

---

## Reporting Issues

If troubleshooting doesn't resolve:

1. Collect: version, OS, steps to reproduce, relevant log excerpt
2. Check existing issues: https://github.com/FanSoftLab/gofle/issues
3. Open new issue with: **Description**, **Steps to Reproduce**, **Expected vs Actual**, **Logs**

---

## Related Information

- [SSH Connection](ssh-connection.md)
- [FTP Connection](ftp-connection.md)
- [Performance](performance.md)
- [Search Issues](search-issues.md)
- [UI Issues](ui-issues.md)
- [Data Recovery](data-recovery.md)