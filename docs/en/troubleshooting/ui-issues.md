# UI Issues Troubleshooting

> **Content type**: Troubleshooting — diagnose and resolve UI/rendering issues.
> **Audience**: All users
> **Severity**: Medium
> **Verified**: 2026-10-29 — tested on Linux, macOS, Windows

## Purpose

Resolve rendering issues, shortcut failures, theme problems, and input issues.

---

## Symptoms Index

| Symptom | Jump To |
|---------|---------|
| Shortcuts not working | [Shortcuts Not Working](#shortcuts-not-working) |
| Theme not applying | [Theme Not Applying](#theme-not-applying) |
| Font not loading | [Font Not Loading](#font-not-loading) |
| Window geometry not saved | [Window Geometry Not Saved](#window-geometry-not-saved) |
| Panes not resizing | [Panes Not Resizing](#panes-not-resizing) |
| Context menu not appearing | [Context Menu Not Appearing](#context-menu-not-appearing) |
| Drag-and-drop not working | [Drag-and-Drop Not Working](#drag-and-drop-not-working) |
| High DPI / scaling issues | [High DPI Issues](#high-dpi-issues) |
| WebView crashes | [WebView Crashes](#webview-crashes) |

---

## Shortcuts Not Working

**Symptom**: Keyboard shortcuts (Ctrl+O, F, F3, etc.) don't trigger actions

### Conditions

- Focus in input field (search, dialog)
- Shortcut conflict with OS/WM
- WebView focus issue

### Possible Causes

| Cause | Verified |
|-------|----------|
| Focus in search field | Yes |
| OS/WM captures shortcut | Yes |
| WebView not focused | Yes |

### Diagnosis

1. Click in log pane (upper) to ensure focus
2. Test `F` (Follow) — works globally
3. Test `Ctrl+O` — may be captured by OS
4. Check OS shortcut settings (GNOME, KDE, macOS, Windows)

### Resolution

1. Ensure focus in log/results pane, not input field
2. For `Ctrl+O`: some WMs capture — use toolbar button
3. For `F1`: may open OS help — use `?` button
4. On Linux: check GNOME/KDE shortcut settings

---

## Theme Not Applying

**Symptom**: Theme change in Settings doesn't update UI colors

### Conditions

- Theme set to "System" but OS theme changed
- Theme change requires restart (some elements)
- WebView caching

### Possible Causes

| Cause | Verified |
|-------|----------|
| System theme not detected | Yes |
| Requires app restart | Yes |
| WebView cache | Yes |

### Diagnosis

1. Set explicit theme (Dark/Light/Dracula) — not System
2. Restart GoFLE
3. Check Settings → Appearance → Theme shows selection

### Resolution

1. Use explicit theme instead of "System"
2. Restart GoFLE after theme change
3. On Linux: ensure GTK theme matches (Adwaita for Light, Adwaita-dark for Dark)

---

## Font Not Loading

**Symptom**: Log text shows fallback font (not JetBrains Mono/Fira Code/Cascadia Code)

### Conditions

- Bundled font not embedded correctly
- Font family name mismatch
- System font substitution

### Possible Causes

| Cause | Verified |
|-------|----------|
| Font not in frontend/dist | Yes |
| CSS font-family mismatch | Yes |
| System fontconfig issue | Possible |

### Diagnosis

1. Check Settings → Appearance → Log Font shows bundled options
2. Inspect browser devtools (F12 in `wails dev`) → Computed font-family
3. Verify `frontend/src/style.css` has `@font-face` for bundled fonts

### Resolution

1. Rebuild frontend: `cd frontend && npm run build`
2. Rebuild Go binary: `wails build`
3. Select different bundled font in Settings

---

## Window Geometry Not Saved

**Symptom**: Window size/position/maximized state not restored on launch

### Conditions

- `settings.json` not writable
- Config directory permission issue
- Multiple instances running

### Possible Causes

| Cause | Verified |
|-------|----------|
| Config directory not writable | Yes |
| Multiple instances | Yes |
| Settings save debounce | Possible |

### Diagnosis

1. Check config directory permissions: `ls -ld ~/.config/gofle/`
2. Check `settings.json` for `window_width`, `window_height`, `window_maximized`
3. Close all GoFLE instances, restart

### Resolution

1. Fix permissions: `chmod 700 ~/.config/gofle/`
2. Ensure single instance
3. Wait 1s after resize before close (debounced save)

---

## Panes Not Resizing

**Symptom**: Splitter between log and results panes doesn't drag

### Conditions

- Splitter handle not visible
- Vertical/horizontal orientation confusion
- Results pane hidden

### Possible Causes

| Cause | Verified |
|-------|----------|
| Results pane hidden (Log Only mode) | Yes |
| Splitter CSS issue | Possible |
| View mode not Split | Yes |

### Diagnosis

1. Check View Mode: Settings → Viewer → Split (not Log Only / Results Only)
2. Check Search Orientation: Horizontal = top/bottom, Vertical = left/right
3. Look for thin drag handle between panes

### Resolution

1. Set View Mode to **Split**
2. For Horizontal: drag horizontal splitter (up/down)
3. For Vertical: drag vertical splitter (left/right)

---

## Context Menu Not Appearing

**Symptom**: Right-click in log/results/tab doesn't show context menu

### Conditions

- Right-click captured by WebView
- Click on wrong element
- Touchscreen / tablet mode

### Possible Causes

| Cause | Verified |
|-------|----------|
| Click on scrollbar | Yes |
| Click on splitter | Yes |
| WebView context menu override | Possible |

### Diagnosis

1. Right-click on log line text (not line number, not scrollbar)
2. Right-click on result row
3. Right-click on tab (not close button)

### Resolution

1. Click directly on content area
2. Use keyboard alternatives: `Ctrl+W` (close tab), `Ctrl+G` (goto line)
3. On touchscreen: long-press may work

---

## Drag-and-Drop Not Working

**Symptom**: Dragging file onto GoFLE window doesn't open it

### Conditions

- Wayland session (Linux)
- File manager permissions
- WebView drop zone not targeted

### Possible Causes

| Cause | Verified |
|-------|----------|
| Wayland security | Yes |
| Drop on wrong area | Yes |
| File manager sandbox | Possible |

### Diagnosis

1. Try dropping on center of log pane
2. Test with `Ctrl+O` instead
3. On Wayland: known limitation with some file managers

### Resolution

1. Use `Ctrl+O` (Open button) as primary method
2. On Wayland: drag from GTK3 apps (Nautilus, Thunar) works better
3. Ensure drop target is log pane, not toolbar

---

## High DPI Issues

**Symptom**: UI too small/large, blurry text, incorrect scaling

### Conditions

- Fractional scaling (125%, 150%, 175%)
- Multiple monitors with different DPI
- Wayland vs X11

### Possible Causes

| Cause | Verified |
|-------|----------|
| WebView DPI detection | Yes |
| Fractional scaling | Yes |
| Multi-monitor DPI mismatch | Yes |

### Diagnosis

1. Check OS display scaling setting
2. Test on single monitor
3. Try `GDK_SCALE=2 GDK_DPI_SCALE=0.5` (Linux)

### Resolution

1. Use integer scaling (100%, 200%) if possible
2. On Linux Wayland: set `GDK_SCALE` env var
3. Font size in Settings → Appearance compensates partially

---

## WebView Crashes

**Symptom**: GoFLE window closes unexpectedly / "WebView crashed" in logs

### Conditions

- Outdated WebKitGTK (Linux)
- Outdated WebView2 (Windows)
- Memory exhaustion
- JavaScript error in frontend

### Possible Causes

| Cause | Verified |
|-------|----------|
| Old WebKitGTK | Yes |
| Old WebView2 | Yes |
| OOM (large file + word wrap) | Yes |
| Frontend JS error | Possible |

### Diagnosis

1. Check `~/.config/gofle/gofle.log` for "WebView" errors
2. Linux: `webkit2gtk-4.1` version — need 2.40+
3. Windows: WebView2 runtime version
4. Disable Word Wrap, close tabs, retry

### Resolution

1. Update system: `sudo apt upgrade libwebkit2gtk-4.1-0` (Linux)
2. Update WebView2: Windows Update or manual installer
3. Disable Word Wrap, reduce open tabs
4. Report crash with log excerpt

---

## Related Information

- [Settings UI](../configuration/settings-ui.md)
- [Keyboard Shortcuts](../user-guide/keyboard-shortcuts.md)
- [Performance](performance.md)
- [Prerequisites](../getting-started/prerequisites.md)