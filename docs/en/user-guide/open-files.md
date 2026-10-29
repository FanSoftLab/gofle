# Open Files

> **Content type**: Task — open local log files in GoFLE.
> **Audience**: All users
> **Prerequisites**: [First Run](first-run.md) completed — GoFLE running
> **Verified**: 2026-10-29 — tested on Linux, macOS, Windows

## Purpose

Open one or more local log files using the GUI, command line, drag-and-drop, or file associations.

## Prerequisites

- GoFLE installed and running
- Read access to target log file(s)
- File size: any (tested up to 10+ GB)

## Procedure

### Method 1: Toolbar Open Button

1. Click the **Open** button (folder icon) in the toolbar, or press `Ctrl+O`.
2. In the file dialog, select one or more files.
3. Click **Open**.
4. Each file opens in a new tab.

### Method 2: Drag and Drop

1. Drag a log file from your file manager.
2. Drop it onto the GoFLE window.
3. The file opens in a new tab.

### Method 3: Command Line Arguments

```bash
# Single file
./GoFLE /var/log/syslog

# Multiple files
./GoFLE /var/log/syslog /var/log/auth.log /home/user/app.log

# With GUI already running
# Files open in new tabs in existing instance
```

### Method 4: File Association (Linux/macOS)

1. Right-click a `.log` file in file manager.
2. Select **Open With** → **GoFLE**.
3. Set as default if desired.

### Method 5: Recent Files

1. Click **Recent** button (clock icon) or press `Ctrl+R`.
2. Select the **Local** tab.
3. Double-click a file or select and click **Open**.

## Expected Result

- New tab created for each file
- Tab shows file name (truncated if long)
- Upper pane displays log content with line numbers
- Status bar shows: file path, size, line count
- Follow mode: "Tail: ACTIVE" (new lines appear automatically)

## Verification

1. Open a file with known content (e.g., `/var/log/syslog`)
2. Confirm first and last lines visible
3. Scroll — lines render smoothly
4. Check status bar shows correct line count

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| "Permission denied" | No read access | Run `chmod +r /path/to/file` or use `sudo` |
| File opens but empty | Binary file or encoding | GoFLE reads as UTF-8; binary files show replacement chars |
| Tab opens then closes | File removed during index | Ensure file exists and is stable |
| Drag-and-drop not working | Wayland/permissions | Use `Ctrl+O` instead |

## Related Information

- [Open Remote (SSH)](open-remote-ssh.md)
- [Open Remote (FTP/FTPS)](open-remote-ftp.md)
- [Tabs and Groups](tabs-groups.md)
- [Recent Files](../configuration/config-files.md#recentjson)