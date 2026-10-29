# Open Remote Log via FTP/FTPS

> **Content type**: Task — connect to a remote server via FTP/FTPS and open a log file.
> **Audience**: SREs, DevOps engineers, developers
> **Prerequisites**: [First Run](first-run.md) — GoFLE running; FTP/FTPS access to target host
> **Verified**: 2026-10-29 — tested with vsftpd, ProFTPD, FileZilla Server

## Purpose

Stream a remote log file over FTP or FTPS (explicit/implicit) into GoFLE. The file is copied to a local temporary file, indexed, and behaves like a local file.

## Prerequisites

- FTP/FTPS server accessible from your machine
- Valid FTP credentials (username + password)
- Remote file path known
- Network: port 21 (FTP/Explicit FTPS) or 990 (Implicit FTPS) open

## Procedure

### 1. Open Remote Connection Dialog

- Click the **Remote** button (cloud icon) in the toolbar
- Select **FTP / FTPS** from the Protocol dropdown

### 2. Fill Connection Form

| Field | Description | Required |
|-------|-------------|----------|
| **Host** | IP address or domain name | Yes |
| **Port** | FTP port (default: 21 for FTP/Explicit, 990 for Implicit) | Yes |
| **Username** | FTP user account | Yes |
| **Password** | User password | Yes |
| **TLS Mode** | `None` (plain FTP), `Explicit` (FTPS), `Implicit` (FTPS) | Yes |
| **Remote File Path** | Absolute path to log file on server | Yes |

### 3. TLS Mode Details

| Mode | Port | Description |
|------|------|-------------|
| **None** | 21 | Plain FTP, no encryption |
| **Explicit** | 21 | FTP with `AUTH TLS` — control connection encrypted, data after |
| **Implicit** | 990 | FTPS — TLS from first byte on control connection |

### 4. (Optional) Browse Remote Files

1. Click **Browse...** next to Remote File Path.
2. Navigate the remote directory tree.
3. Select a file and click **Open Selected**.

### 5. Connect

Click **Connect**.

- First connection to unknown FTPS server: **Certificate Trust** dialog appears.
  - Verify the SHA256 fingerprint matches your server.
  - Click **Trust Host** to save to `ftp_certs` and proceed.
- Connection establishes. Progress shows: "Connecting..." → "Downloading log..." → "Indexing..."
- File opens in a new tab with FTP badge.

### 6. Use Like Local File

- Search, follow, export work identically
- Follow polls remote every 30 seconds for new lines (configurable)
- Tab shows FTP badge in Recent list

## Command Line Alternative

```bash
# Interactive viewer (plain FTP)
./GoFLE ftp://user@host:21/path/to/log.txt

# Explicit FTPS
./GoFLE ftp://user@host:21/path/to/log.txt --tls explicit

# Implicit FTPS
./GoFLE ftp://user@host:990/path/to/log.txt --tls implicit

# Dump to stdout
./GoFLE ftp://user@host:21/path/to/log.txt --dump
```

## Expected Result

- New tab with FTP badge opens
- Remote file content appears in upper pane
- Status bar shows remote path (e.g., `ftp://user@host/path/to/log.txt`)
- Follow mode active: "Tail: ACTIVE" (polls every 30s)
- Search works on remote content

## Verification

1. Open a known remote log on FTP/FTPS server
2. Confirm lines appear with timestamps
3. Type a search term — results appear
4. Press `F` — follow toggles
5. Wait for new remote lines — they appear on next poll (30s default)

## Certificate Trust (FTPS Only)

- GoFLE pins server certificates in `~/.config/gofle/ftp_certs`
- Format: `host:port <sha256-fingerprint>` (one per line)
- On first connect: fingerprint shown, explicit confirmation required
- On subsequent connects: certificate verified against store
- If certificate changes: connection rejected (possible MITM)

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| "Connection refused" | FTP port closed/firewall | Check port 21 (or 990) open on server |
| "Authentication failed" | Wrong username/password | Verify credentials; try `ftp host` manually |
| "Certificate verification failed" | Cert changed or MITM | Verify with server admin; remove old entry from `ftp_certs` |
| "TLS handshake failed" | Wrong TLS mode | Match server config: Explicit vs Implicit |
| "Remote file not found" | Wrong path | Use **Browse...** to navigate and select |
| "550 Permission denied" | FTP user lacks read access | Check server permissions for FTP user |
| Slow initial load | Large file, slow network | Wait for indexing; follow works after initial sync |
| Follow not updating | Poll interval too long / server not appending | Reduce `streamProbeInterval` in settings (default 30s) |

## Security Notes

- Passwords encrypted at rest with AES-GCM in `recent.json`
- FTPS encrypts control channel; data channel encrypted after `AUTH TLS` (Explicit) or from start (Implicit)
- Plain FTP sends credentials in cleartext — avoid on untrusted networks
- Temp file: `os.CreateTemp("", "GoFLE-ftp-*.log")` — auto-deleted on tab close

## Related Information

- [Open Remote (SSH)](open-remote-ssh.md)
- [FTP Settings](../../configuration/settings-ui.md#remote-settings)
- [Troubleshooting FTP](../troubleshooting/ftp-connection.md)
- [CLI FTP Command](../../cli-reference/cli-commands.md#ftp)