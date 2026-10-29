# Migration Guides

> **Content type**: Release documentation — breaking changes and migration steps per version.
> **Audience**: Users upgrading between versions
> **Verified**: 2026-10-29

## Purpose

Guide users through breaking changes when upgrading GoFLE.

---

## v1.x → v2.0.0

**Release Date**: 2026-10-04

### Breaking Changes

| Area | Change | Migration |
|------|--------|-----------|
| **Architecture** | Complete rewrite: Electron → Wails v2 | Reinstall; no in-place upgrade |
| **Config Format** | New `settings.json` structure | Settings reset to defaults; reconfigure |
| **Recent Files** | New `recent.json` with encryption | Previous recent list lost; reopen files |
| **Session Restore** | New `saved_tabs.json` format | Previous session not restored |
| **SSH Keys** | Encryption changed (AES-GCM) | Re-enter SSH passwords/key passphrases |
| **CLI** | New `gofle` companion (separate binary) | Build `gofle` separately |
| **Remote** | SSH only (v1); SSH + FTP (v2) | FTP users: new capability |
| **Search** | New advanced filters (Nearby, Levels, Time, etc.) | No migration; new features |
| **Tabs** | Tab groups added | No migration; new feature |

### Migration Steps

1. **Uninstall v1** (if installed via package manager)
2. **Install v2** via `./build.sh` or `wails build`
3. **Launch v2** — starts with empty settings
4. **Reconfigure**: Settings → Appearance, Viewer, Search, Session, Remote
5. **Reopen files**: Ctrl+O or Recent (will be empty initially)
6. **Re-enter SSH credentials**: First SSH connection prompts for password/key passphrase
7. **Recreate tab groups** if used

### Config Directory

- v1: `~/.config/glogg/` (or similar)
- v2: `~/.config/gofle/` — **different directory, no auto-migration**

---

## v2.0.x → v2.1.0 (Planned)

### Expected Breaking Changes

| Area | Change | Migration |
|------|--------|-----------|
| **Config** | New remote lockout settings | Auto-migrated with defaults |
| **CLI** | New session commands | No migration; new features |
| **Search** | Preset format v2 | Auto-migrated on load |

### Migration Steps

1. Update via `./build.sh` or download new release
2. Launch — config auto-migrates
3. Verify Settings → Remote shows new lockout settings

---

## General Upgrade Procedure

### For Docker Build Users

```bash
# Pull latest
git pull

# Build new version
./build.sh -v 2.1.0

# Replace binary
sudo cp GoFLE /usr/local/bin/GoFLE  # or user install
```

### For Local Build Users

```bash
git pull
cd frontend && npm install && npm run build && cd ..
wails build
# Binary at build/bin/GoFLE
```

### For Package Install Users

```bash
# Re-run installer
./scripts/install.sh --user  # or --system
```

### Config Backup (Recommended)

```bash
# Before upgrade
cp -r ~/.config/gofle ~/.config/gofle.backup.$(date +%F)

# After upgrade, if issues:
# cp ~/.config/gofle.backup.*/recent.json ~/.config/gofle/
# cp ~/.config/gofle.backup.*/search_presets.json ~/.config/gofle/
```

**Note**: `settings.json` contains encryption key — restore with `recent.json` together or neither.

---

## Downgrade Procedure

If v2.x has critical issue:

1. **Backup current config**: `cp -r ~/.config/gofle ~/.config/gofle.v2`
2. **Install previous version** (download from GitHub Releases)
3. **Restore config** from pre-upgrade backup:
   ```bash
   rm -rf ~/.config/gofle
   cp -r ~/.config/gofle.backup.2026-10-01 ~/.config/gofle
   ```
4. **Launch previous version**

**Warning**: Downgrading config format not supported. Use pre-upgrade backup.

---

## Related Information

- [Release Process](release-process.md)
- [Changelog](changelog.md)
- [Config Files](../configuration/config-files.md)
- [Security Notes](../../../SECURITY.md)