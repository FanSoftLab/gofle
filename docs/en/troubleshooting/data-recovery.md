# Data Recovery Troubleshooting

> **Content type**: Troubleshooting — recover lost tabs, corrupted config, and credentials.
> **Audience**: All users
> **Severity**: High (data loss)
> **Verified**: 2026-10-29 — tested recovery scenarios

## Purpose

Recover from lost session, corrupted config files, lost SSH/FTP credentials, and missing recent files.

---

## Symptoms Index

| Symptom | Jump To |
|---------|---------|
| Tabs not restored on launch | [Tabs Not Restored](#tabs-not-restored) |
| Config file corrupted | [Config Corrupted](#config-corrupted) |
| Lost SSH credentials | [Lost SSH Credentials](#lost-ssh-credentials) |
| Lost FTP credentials | [Lost FTP Credentials](#lost-ftp-credentials) |
| Recent files missing | [Recent Files Missing](#recent-files-missing) |
| Search presets lost | [Search Presets Lost](#search-presets-lost) |
| Tab group lost | [Tab Group Lost](#tab-group-lost) |

---

## Tabs Not Restored

**Symptom**: Launch GoFLE, expected tabs not open

### Conditions

- "Remember open tabs" disabled
- `saved_tabs.json` missing/corrupted
- Clean exit not performed (crash/kill)
- Remote tabs require reconnection

### Possible Causes

| Cause | Verified |
|-------|----------|
| Setting disabled | Yes |
| Config file missing | Yes |
| Crash on exit | Yes |
| Remote auth failed | Yes |

### Diagnosis

1. Check Settings → Session → Remember open tabs
2. Check file: `~/.config/gofle/saved_tabs.json` exists
3. Check GoFLE log for restore errors
4. Try CLI: `gofle session show`

### Resolution

**Setting disabled**:
1. Enable Settings → Session → Remember open tabs
2. Open tabs, exit cleanly, relaunch

**File missing/corrupted**:
1. Check `gofle session show` — if shows tabs, try `gofle session restore`
2. Recreate tabs manually from Recent (Ctrl+R)

**Remote tabs fail**:
1. SSH: re-enter password if key passphrase not saved
2. FTP: re-enter password
3. Check `known_hosts` / `ftp_certs` for trust

---

## Config Corrupted

**Symptom**: GoFLE fails to start / settings reset / JSON parse error in logs

### Conditions

- `settings.json` truncated/invalid JSON
- Disk full during write
- Concurrent writes (multiple instances)

### Possible Causes

| Cause | Verified |
|-------|----------|
| Disk full | Yes |
| Kill during write | Yes |
| Multiple instances | Yes |
| Manual edit error | Yes |

### Diagnosis

1. Check log: `~/.config/gofle/gofle.log` for "JSON parse error"
2. Validate JSON: `cat ~/.config/gofle/settings.json | jq .`
3. Check disk space: `df -h ~/.config/gofle/`

### Resolution

**Minor corruption**:
1. Fix JSON manually (use `jq` to validate)
2. Restart GoFLE

**Severe corruption**:
1. Backup config dir: `cp -r ~/.config/gofle ~/.config/gofle.bak`
2. Delete corrupted file(s): `rm ~/.config/gofle/settings.json`
3. Restart GoFLE — regenerates defaults
4. Reconfigure settings

**All configs corrupted**:
1. Backup: `mv ~/.config/gofle ~/.config/gofle.bak`
2. Restart GoFLE — fresh config
3. Restore `recent.json` and `search_presets.json` from backup if valid

---

## Lost SSH Credentials

**Symptom**: SSH recent entry prompts for password/key passphrase that was previously saved

### Conditions

- `secret_key` changed (settings.json)
- `recent.json` corrupted
- Manual config edit removed credentials
- Different config directory (multi-user)

### Possible Causes

| Cause | Verified |
|-------|----------|
| Secret key rotated | Yes |
| Recent file corrupted | Yes |
| Credentials not saved (key auth) | Yes |
| Wrong config directory | Yes |

### Diagnosis

1. Check `settings.json` has `secret_key` (base64)
2. Check `recent.json` SSH entries have `key_passphrase` or `password` fields
3. Try manual SSH: `ssh user@host` — if works, creds not in GoFLE
4. Check config directory: `echo $XDG_CONFIG_HOME` (Linux)

### Resolution

**Re-enter credentials**:
1. Open SSH dialog (Recent → SSH tab → entry → Open)
2. Re-enter password or key passphrase
3. GoFLE re-encrypts with current `secret_key`

**Secret key mismatch**:
- If `secret_key` changed, old encrypted values unrecoverable
- Must re-enter all SSH/FTP passwords

**Prevention**:
- Backup `settings.json` (contains `secret_key`)
- Don't edit `settings.json` manually

---

## Lost FTP Credentials

**Symptom**: FTP recent entry prompts for password previously saved

### Conditions

- Same as SSH credentials (shared encryption)
- `secret_key` changed
- `recent.json` corrupted

### Resolution

Same as [Lost SSH Credentials](#lost-ssh-credentials) — re-enter password in FTP dialog.

---

## Recent Files Missing

**Symptom**: Recent (Ctrl+R) shows empty or missing entries

### Conditions

- `recent.json` missing/corrupted
- `max_recent` setting too low
- Entries expired (not time-based, but count-based)
- Config directory changed

### Possible Causes

| Cause | Verified |
|-------|----------|
| Config file missing | Yes |
| max_recent = 0 | Yes |
| Config directory changed | Yes |

### Diagnosis

1. Check `recent.json` exists and valid JSON
2. Settings → Session → Recent list size
3. Check config directory path

### Resolution

1. Increase Recent list size: Settings → Session → Recent list size (default 50)
2. Reopen files — they re-add to recent
3. If `recent.json` corrupted: delete, restart, rebuild from scratch

---

## Search Presets Lost

**Symptom**: Presets (Advanced filters → Presets) empty

### Conditions

- `search_presets.json` missing
- localStorage cleared (GUI)
- Config directory changed

### Possible Causes

| Cause | Verified |
|-------|----------|
| File missing | Yes |
| localStorage cleared | Yes |
| Different config dir | Yes |

### Diagnosis

1. Check `~/.config/gofle/search_presets.json`
2. Check browser localStorage (F12 → Application → Local Storage)
3. CLI: `gofle presets list`

### Resolution

1. Recreate presets in GUI (Advanced filters → Presets → Save)
2. CLI import: `gofle presets import backup.json`
3. If using multiple config dirs: ensure same directory

---

## Tab Group Lost

**Symptom**: Tab group (colored label) missing after restart

### Conditions

- `saved_tabs.json` missing group info
- Group not saved with session
- Config corrupted

### Possible Causes

| Cause | Verified |
|-------|----------|
| Session not saved | Yes |
| Group data not in saved_tabs | Yes |

### Diagnosis

1. Check `saved_tabs.json` for `groups` array
2. Verify Settings → Session → Remember open tabs

### Resolution

1. Recreate group: right-click tab → New Group
2. Reassign tabs: right-click → Move to Group
3. Enable session restore, exit cleanly

---

## Complete Config Reset (Last Resort)

If all else fails:

```bash
# 1. Backup
cp -r ~/.config/gofle ~/.config/gofle.backup.$(date +%s)

# 2. Reset
rm -rf ~/.config/gofle

# 3. Restart GoFLE — fresh config
./GoFLE

# 4. Restore selective files from backup if valid
# cp ~/.config/gofle.backup.*/recent.json ~/.config/gofle/
# cp ~/.config/gofle.backup.*/search_presets.json ~/.config/gofle/
```

**Note**: `settings.json` contains encryption key — restoring it without `recent.json` makes credentials unreadable. Restore both together or neither.

---

## Related Information

- [Session Restore](../user-guide/session-restore.md)
- [Tabs and Groups](../user-guide/tabs-groups.md)
- [Config Files](../configuration/config-files.md)
- [Security Notes](../../../SECURITY.md)