# Linux Package Install

> **Content type**: Task — install GoFLE system-wide or per-user on Linux.
> **Audience**: Linux end users
> **Prerequisites**: Built binary from [Docker Build](install-docker.md) or [Local Build](install-local.md)
> **Verified**: 2026-10-29 — tested on Ubuntu 22.04, Fedora 39

## Purpose

Install the GoFLE binary, icon, and desktop entry so GoFLE appears in the application menu and can be set as default handler for `.log`/`.txt` files.

## Prerequisites

- GoFLE binary built and located at `./GoFLE` (repository root) or `build/bin/GoFLE`
- Linux with systemd and desktop environment (GNOME, KDE, XFCE, etc.)
- `sudo` for system-wide install (optional)

## Procedure

### System-wide Install (requires sudo)

```bash
sudo ./scripts/install.sh --system
```

Installs to:
- Binary: `/usr/local/bin/GoFLE`
- Icon: `/usr/local/share/icons/hicolor/256x256/apps/gofle.png`
- Desktop entry: `/usr/local/share/applications/GoFLE.desktop`

### Per-user Install (no sudo)

```bash
./scripts/install.sh --user
```

Installs to:
- Binary: `~/.local/bin/GoFLE`
- Icon: `~/.local/share/icons/hicolor/256x256/apps/gofle.png`
- Desktop entry: `~/.local/share/applications/GoFLE.desktop`

**Note**: Ensure `~/.local/bin` is in your `PATH`.

## Expected Result

- GoFLE appears in application menu under "Development" or "Utilities"
- `.log` and `.txt` files can be associated with GoFLE via "Open With"
- Command `GoFLE` works from terminal (if install directory in PATH)

## Verification

```bash
# Check binary in PATH
which GoFLE

# Check desktop entry
ls ~/.local/share/applications/GoFLE.desktop  # or /usr/local/share/applications/

# Launch from menu or terminal
GoFLE
```

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| `GoFLE: command not found` | Install dir not in PATH | Add `~/.local/bin` to PATH in `~/.bashrc` or `~/.zshrc` |
| Icon not showing in menu | Icon cache stale | Run `gtk-update-icon-cache -f ~/.local/share/icons/hicolor` |
| Desktop entry not appearing | Desktop database stale | Run `update-desktop-database ~/.local/share/applications` |
| "No such file" for binary | Binary not at expected location | Run `./build.sh` first (produces `./GoFLE` at repo root) |

## Uninstall

```bash
# System-wide
sudo ./scripts/uninstall.sh --system

# Per-user
./scripts/uninstall.sh --user
```

## Related Information

- [Docker Build](install-docker.md)
- [Local Build](install-local.md)
- [First Run](first-run.md)
- [Configuration Directory](../../configuration/config-files.md)