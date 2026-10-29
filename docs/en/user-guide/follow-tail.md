# Follow (Live Tail)

> **Content type**: Task — enable and use live tail/follow mode for real-time log updates.
> **Audience**: All users
> **Prerequisites**: [Open Files](open-files.md) or [Open Remote](open-remote-ssh.md) — file loaded in tab
> **Verified**: 2026-10-29 — tested local and remote (SSH/FTP)

## Purpose

Keep the log view at the end of the file as new lines are appended (local) or arrive (remote).

## Prerequisites

- Log file open in active tab
- Follow button visible in toolbar (default)

## Procedure

### Enable Follow

- Click the **Follow** button (down arrows icon) in toolbar, OR
- Press `F` key

Button state changes: icon fills, tooltip shows "Tail: ACTIVE"
Status bar shows: **Tail: ACTIVE**

### Disable Follow

- Click **Follow** button again, OR
- Press `F` key, OR
- Scroll up in the log pane (automatically pauses follow)

Button state: icon outline, tooltip shows "Tail: PAUSED"
Status bar shows: **Tail: PAUSED**

### Behavior

| Source | Follow Mechanism |
|--------|------------------|
| **Local file** | 500 ms poller checks file size; reads new bytes via `pread` |
| **SSH** | 2 second poller runs `tail -c +N` on remote; streams new lines |
| **FTP/FTPS** | 30 second poller (configurable) checks file size; streams new bytes |

New lines appear at bottom of log pane automatically.

### Follow and Scroll Interaction

| Action | Follow State |
|--------|--------------|
| Scroll down to bottom | Follow resumes (Tail: ACTIVE) |
| Scroll up (away from bottom) | Follow pauses (Tail: PAUSED) |
| Click result in results pane | Follow pauses |
| Press `F` | Toggles follow on/off |
| New lines arrive while scrolled up | Lines buffered; follow resumes when scroll to bottom |

### Follow with Search Active

- If search is active, new lines are searched incrementally
- Matching new lines appended to results pane
- Non-matching lines ignored for results (but visible in full log)

### Default Follow State

Configure in **Settings → Viewer → Follow (tail)**:
- **On** — follow enabled when opening any file (default)
- **Off** — follow disabled by default

## Expected Result

- Follow button toggles between ACTIVE/PAUSED
- New lines appear at bottom without manual refresh
- Status bar reflects current follow state
- Scrolling up pauses follow automatically

## Verification

1. Open a log file that is actively written (e.g., `tail -f /var/log/syslog` in another terminal)
2. Enable Follow (`F`)
3. Confirm "Tail: ACTIVE" in status bar
4. Write a line to the file: `echo "TEST LINE" >> /var/log/syslog`
5. Confirm "TEST LINE" appears at bottom automatically
6. Scroll up — confirm "Tail: PAUSED"
7. Scroll to bottom — confirm "Tail: ACTIVE"

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| Follow not updating (local) | File not growing / permissions | Verify file appended; check read permissions |
| Follow not updating (SSH) | SSH connection dropped | Reconnect; check SSH timeout settings |
| Follow not updating (FTP) | Poll interval too long | Reduce `streamProbeInterval` in Settings → Remote |
| High CPU with follow | Very high log rate | Disable follow; use search with Range: Last N lines |
| Follow pauses unexpectedly | Clicked in results pane | Scroll to bottom in log pane to resume |

## Related Information

- [Open Files](open-files.md)
- [Open Remote (SSH)](open-remote-ssh.md)
- [Open Remote (FTP/FTPS)](open-remote-ftp.md)
- [Viewer Settings](../../configuration/settings-ui.md#viewer-settings)
- [Remote Settings](../../configuration/settings-ui.md#remote-settings)