# First Run

> **Content type**: Task — launch GoFLE and open your first log file.
> **Audience**: All users
> **Prerequisites**: [Installation complete](install-docker.md) — binary available at `./GoFLE` or `build/bin/GoFLE`
> **Verified**: 2026-10-29 — tested on Linux, macOS, Windows

## Purpose

Launch GoFLE for the first time and open a log file using the GUI or command line.

## Procedure

### Option 1: Launch GUI and Open File

1. Start GoFLE:
   ```bash
   ./GoFLE
   # or
   ./build/bin/GoFLE
   ```

2. The main window opens with an empty state showing:
   - "Open a log to get started"
   - Drop zone for drag-and-drop
   - Toolbar with Open, Recent, Remote, Go to Line, Help, Settings

3. Open a log file:
   - **Drag and drop** a `.log` or `.txt` file onto the window, OR
   - Press `Ctrl+O` and select one or more files, OR
   - Click the **Open** toolbar button (folder icon)

4. The file opens in a new tab. The line-offset index builds instantly (shows "Indexing..." briefly). The log content appears in the upper pane.

### Option 2: Open File from Command Line

```bash
# Open one or more files directly
./GoFLE /var/log/syslog /home/user/app.log

# Open remote file via SSH
./GoFLE ssh://user@host:22/var/log/nginx/access.log

# Open remote file via FTP
./GoFLE ftp://user@host:21/path/to/log.txt
```

Each argument opens in a separate tab.

### Option 3: Web UI Mode

```bash
# Start HTTP server on port 8080
./GoFLE -web

# Custom port
./GoFLE -web :9090
```

Open `http://localhost:8080/` in a browser. UI is identical to desktop app.

## Expected Result

- GoFLE window opens (or browser tab for `-web`)
- Selected file(s) load in tabs
- Status bar shows file name, size, line count
- Follow button shows "Tail: ACTIVE" (new lines appear automatically)

## Verification

1. Open a known log file (e.g., `/var/log/syslog` or any `.log` file)
2. Confirm log lines appear in upper pane
3. Type a search term in the search box (bottom toolbar)
4. Press `Enter` — results appear in lower pane
5. Press `F` — follow toggles (status bar changes)

## Next Steps

- [Search Basics](../user-guide/search-basics.md)
- [Follow (Live Tail)](../user-guide/follow-tail.md)
- [Open Remote (SSH)](../user-guide/open-remote-ssh.md)
- [Keyboard Shortcuts](../user-guide/keyboard-shortcuts.md)

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| Window opens but empty | No file opened | Use `Ctrl+O` or drag file |
| "Failed to open file" | Permission denied | Check file read permissions |
| Binary crashes on start | Missing system libs (Linux) | Install prerequisites from [Prerequisites](prerequisites.md) |
| `-web` port already in use | Port 8080 occupied | Use `-web :9090` for different port |

## Related Information

- [Open Files](../user-guide/open-files.md)
- [Search Basics](../user-guide/search-basics.md)
- [Configuration](../../configuration/settings-ui.md)