# Open Remote Log via SSH

> **Content type**: Task — connect to a remote server via SSH and open a log file.
> **Audience**: SREs, DevOps engineers, developers
> **Prerequisites**: [First Run](first-run.md) — GoFLE running; SSH access to target host
> **Verified**: 2026-10-29 — tested with OpenSSH servers

## Purpose

Stream a remote log file over SSH into GoFLE. The file is copied to a local temporary file, indexed, and behaves like a local file (search, follow, export).

## Prerequisites

- SSH server accessible from your machine
- Valid SSH credentials (password or private key)
- Remote file path known (e.g., `/var/log/nginx/access.log`)
- Network: port 22 (default) or custom SSH port open

## Procedure

### 1. Open SSH Connection Dialog

- Click the **Remote** button (cloud icon) in the toolbar, OR
- Press `Ctrl+Shift+O` (if configured), OR
- Use command line: `./GoFLE ssh://user@host:22/path/to/file`

### 2. Fill Connection Form

| Field | Description | Required |
|-------|-------------|----------|
| **Host** | IP address or domain name | Yes |
| **Port** | SSH port (default: 22) | Yes |
| **Username** | Remote user account | Yes |
| **Auth Type** | Password or Private Key | Yes |
| **Password** | User password (if Password auth) | If Password |
| **Private Key Path** | Path to `id_ed25519`, `id_rsa`, etc. (if Key auth) | If Key |
| **Key Passphrase** | Passphrase for encrypted key (optional) | If Key encrypted |
| **Remote File Path** | Absolute path to log file on server | Yes |

### 3. (Optional) Browse Remote Files

1. Click **Browse...** next to Remote File Path.
2. Navigate the remote directory tree.
3. Select a file and click **Open Selected**.

### 4. Connect

Click **Connect**.

- First connection to unknown host: **Host Key Verification** dialog appears.
  - Verify the SHA256 fingerprint matches your server.
  - Click **Trust Host** to save to `known_hosts` and proceed.
- Connection establishes. Progress shows: "Connecting..." → "Downloading log..." → "Indexing..."
- File opens in a new tab with SSH badge.

### 5. Use Like Local File

- Search, follow, export work identically
- Follow polls remote every 2 seconds for new lines
- Tab shows SSH badge in Recent list

## Command Line Alternative

```bash
# Interactive viewer
./GoFLE ssh://user@host:22/var/log/nginx/access.log

# With key auth
./GoFLE ssh://user@host:22/var/log/nginx/access.log -k ~/.ssh/id_ed25519

# Dump to stdout (for piping)
./GoFLE ssh://user@host:22/var/log/nginx/access.log --dump | grep ERROR
```

## Expected Result

- New tab with SSH badge opens
- Remote file content appears in upper pane
- Status bar shows remote path (e.g., `ssh://user@host/var/log/nginx/access.log`)
- Follow mode active: "Tail: ACTIVE"
- Search works on remote content

## Verification

1. Open a known remote log (e.g., `/var/log/syslog` on test server)
2. Confirm lines appear with timestamps
3. Type a search term — results appear
4. Press `F` — follow toggles
5. Wait for new remote lines — they appear automatically

## Host Key Verification

- GoFLE uses OpenSSH `known_hosts` format at `~/.config/gofle/known_hosts`
- On first connect: fingerprint shown, explicit confirmation required
- On subsequent connects: key verified automatically
- If key changes: connection rejected (possible MITM), manual intervention needed

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| "Connection refused" | SSH port closed/firewall | Check port 22 (or custom) open on server |
| "Authentication failed" | Wrong password/key | Verify credentials; try `ssh user@host` manually |
| "Host key verification failed" | Key changed or MITM | Verify with server admin; remove old key from `known_hosts` |
| "Permission denied (publickey)" | Key not authorized | Add public key to `~/.ssh/authorized_keys` on server |
| "Remote file not found" | Wrong path | Use **Browse...** to navigate and select |
| Slow initial load | Large file, slow network | Wait for indexing; follow works after initial sync |
| Follow not updating | Network issue / server not appending | Check SSH connection alive; increase timeout in Settings |

## Security Notes

- Passwords/keys encrypted at rest with AES-GCM in `recent.json`
- Private key passphrase never stored
- Temp file: `os.CreateTemp("", "GoFLE-ssh-*.log")` — auto-deleted on tab close
- No telemetry, no background connections

## Related Information

- [Open Remote (FTP/FTPS)](open-remote-ftp.md)
- [SSH Settings](../../configuration/settings-ui.md#remote-settings)
- [Troubleshooting SSH](../troubleshooting/ssh-connection.md)
- [CLI SSH Command](../../cli-reference/cli-commands.md#ssh)