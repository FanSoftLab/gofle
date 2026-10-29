# CLI Usage Examples

> **Content type**: Task — verified usage examples for common scenarios.
> **Audience**: Terminal users, automation
> **Verified**: 2026-10-29 — tested against built binary

## Purpose

Practical, tested examples for common `gofle` CLI workflows.

---

## Local Files

```bash
# Open single file in viewer
gofle /var/log/syslog

# Open multiple files (each in a tab)
gofle /var/log/syslog /var/log/auth.log /home/user/app.log

# Open with explicit viewer flag
gofle --view /var/log/syslog
```

---

## SSH — Interactive Viewer

```bash
# Basic: prompt for password, browse remote files
gofle ssh user@10.0.0.5

# Specify host, port, user, key, remote path
gofle ssh -H 10.0.0.5 -u deploy -p 2222 -k ~/.ssh/id_ed25519 --remote /var/log/nginx/access.log

# Short form with user@host
gofle ssh deploy@10.0.0.5 --remote /var/log/app.log

# Use key with passphrase (prompted interactively)
gofle ssh -k ~/.ssh/id_ed25519 user@host --remote /var/log/app.log
```

---

## SSH — Dump to Stdout (Piping)

```bash
# Stream remote file and pipe to grep
gofle ssh host --remote /var/log/app.log --dump | grep ERROR

# Stream and count lines
gofle ssh host --remote /var/log/app.log --dump | wc -l

# Stream and save to local file
gofle ssh host --remote /var/log/app.log --dump > local-copy.log

# Stream and follow with less
gofle ssh host --remote /var/log/app.log --dump | less +F
```

---

## FTP/FTPS — Interactive Viewer

```bash
# Plain FTP
gofle ftp user@ftp.example.com --remote /logs/app.log

# Explicit FTPS (AUTH TLS)
gofle ftp user@ftp.example.com --remote /logs/app.log --tls explicit

# Implicit FTPS (TLS from start)
gofle ftp user@ftp.example.com --remote /logs/app.log --tls implicit

# Custom port
gofle ftp user@ftp.example.com:2121 --remote /logs/app.log
```

---

## FTP/FTPS — Dump to Stdout

```bash
# Plain FTP dump
gofle ftp host --remote /logs/app.log --dump | grep "FAILED"

# Explicit FTPS dump
gofle ftp host --remote /logs/app.log --tls explicit --dump | jq -R '.'
```

---

## Recent Files

```bash
# Interactive recent picker (shared with GUI)
gofle recent

# Reopen recent entry #3 non-interactively
gofle recent open 3

# Clear all recent entries
gofle recent clear
```

---

## Search Presets

```bash
# List presets
gofle presets

# Export presets to file
gofle presets export my-presets.json

# Export to stdout
gofle presets export

# Import from file
gofle presets import my-presets.json
```

---

## Session Management

```bash
# Show saved session tabs
gofle session show

# Restore saved session (opens all tabs)
gofle session restore

# Clear saved session
gofle session clear
```

---

## Automation Examples

### Cron: Daily Error Check

```bash
#!/bin/bash
# /etc/cron.daily/check-errors
ERRORS=$(gofle ssh prod-server --remote /var/log/app.log --dump 2>/dev/null | grep -c "ERROR")
if [ "$ERRORS" -gt 0 ]; then
    echo "Found $ERRORS errors" | mail -s "Production Errors" ops@example.com
fi
```

### CI/CD: Verify Deploy Log

```bash
#!/bin/bash
# Check deploy log for success marker
gofle ssh deploy@prod --remote /var/log/deploy.log --dump | grep -q "DEPLOY_SUCCESS"
if [ $? -eq 0 ]; then
    echo "Deploy verified"
else
    echo "Deploy verification failed"
    exit 1
fi
```

### Log Rotation Helper

```bash
#!/bin/bash
# Archive yesterday's logs from remote
DATE=$(date -d "yesterday" +%Y-%m-%d)
gofle ssh log-server --remote "/var/log/app-${DATE}.log" --dump | gzip > "app-${DATE}.log.gz"
```

---

## Viewer Interactive Examples

```bash
# Inside viewer:
# 1. Open recent: press T, select, Enter
# 2. Search: press /, type ERROR, Enter
# 3. Regex: press Alt+R, type \d{4}-\d{2}-\d{2}, Enter
# 4. Advanced filters: press A, configure Nearby/Levels/Time
# 5. Presets: press P, select preset
# 6. Export matches: press e, choose "matches", save
# 7. Toggle follow: press f
# 8. Switch tab: Ctrl+Tab
# 9. Group tabs: press G, name "Production", pick color
# 10. Quit: press q
```

---

## Common Flag Combinations

| Scenario | Command |
|----------|---------|
| SSH with key, no password prompt | `gofle ssh -k ~/.ssh/id_ed25519 user@host --remote /path` |
| FTPS explicit, custom port | `gofle ftp user@host:2121 --remote /path --tls explicit` |
| Dump remote log, follow locally | `gofle ssh host --remote /var/log/app.log --dump \| less +F` |
| Quick recent reopen | `gofle recent open 1` |
| Script-friendly (no prompts) | `gofle ssh --insecure-accept-host-key user@host --remote /path --dump` |

---

## Troubleshooting Examples

```bash
# Test SSH connectivity
gofle ssh user@host --remote /dev/null --dump

# Test FTP connectivity
gofle ftp user@host --remote / --dump | head -5

# Debug connection (verbose not available; check logs)
# Logs at: ~/.config/gofle/gofle.log

# Clear problematic host key
rm ~/.config/gofle/known_hosts  # or edit to remove entry

# Clear FTPS cert trust
rm ~/.config/gofle/ftp_certs
```

---

## Related Information

- [CLI Overview](cli-overview.md)
- [CLI Commands](cli-commands.md)
- [CLI Viewer Keys](cli-viewer-keys.md)
- [GUI Remote SSH](../user-guide/open-remote-ssh.md)
- [GUI Remote FTP](../user-guide/open-remote-ftp.md)