# FTP/FTPS Connection Troubleshooting

> **Content type**: Troubleshooting — diagnose and resolve FTP/FTPS connection issues.
> **Audience**: All users
> **Severity**: High (blocks remote access)
> **Verified**: 2026-10-29 — tested against common FTP/FTPS failure modes

## Purpose

Resolve FTP/FTPS connection, authentication, certificate trust, TLS, and follow issues.

---

## Symptoms Index

| Symptom | Jump To |
|---------|---------|
| "Connection refused" | [Connection Refused](#connection-refused) |
| "Authentication failed" | [Authentication Failed](#authentication-failed) |
| "Certificate trust failed" | [Certificate Trust Failed](#certificate-trust-failed) |
| "TLS handshake failed" | [TLS Handshake Failed](#tls-handshake-failed) |
| "550 Permission denied" | [Permission Denied](#permission-denied) |
| Follow not updating | [Follow Not Updating](#follow-not-updating) |
| Slow download | [Slow Download](#slow-download) |
| Remote file not found | [Remote File Not Found](#remote-file-not-found) |

---

## Connection Refused

**Symptom**: `dial tcp X:21: connect: connection refused`

### Conditions

- FTP port (21/990) not reachable
- FTP service not running
- Firewall blocking

### Possible Causes

| Cause | Verified |
|-------|----------|
| FTP server not running | Yes |
| Wrong port | Yes |
| Firewall (server/network) | Yes |
| Passive mode ports blocked | Yes |

### Diagnosis

1. Test with standard FTP: `ftp host` or `lftp host`
2. Check port: `nc -zv host 21` (or 990 for implicit)
3. Verify server FTP status: `systemctl status vsftpd` / `proftpd`

### Resolution

1. Start FTP server
2. Use correct port: `--port` flag (21 default, 990 implicit)
3. Open firewall for control port AND passive range
4. Check server passive port config (e.g., `pasv_min_port`/`pasv_max_port`)

---

## Authentication Failed

**Symptom**: "530 Login incorrect" or repeated password prompts

### Conditions

- Wrong username/password
- User not allowed (FTP user list)
- Anonymous login disabled

### Possible Causes

| Cause | Verified |
|-------|----------|
| Incorrect password | Yes |
| Incorrect username | Yes |
| User not in FTP user list | Yes |
| Anonymous disabled | Yes |

### Diagnosis

1. Test manually: `ftp host` — same credentials
2. Check server auth log: `/var/log/vsftpd.log` / `/var/log/proftpd.log`
3. Verify username in GoFLE (case-sensitive)
4. Check if user in `/etc/vsftpd.userlist` or similar

### Resolution

1. Correct password/username
2. Add user to FTP allowed list
3. Enable anonymous if needed: `anonymous_enable=YES`
4. Check user shell: must be valid (e.g., `/bin/false` OK for vsftpd)

---

## Certificate Trust Failed

**Symptom**: "Unknown FTP server certificate" / fingerprint mismatch / "Certificate changed"

### Conditions

- First FTPS connection (unknown cert)
- Server certificate renewed/changed
- MITM attack (rare)

### Possible Causes

| Cause | Verified |
|-------|----------|
| First connection | Yes |
| Cert renewed (Let's Encrypt, etc.) | Yes |
| Self-signed cert changed | Yes |
| MITM | Possible |

### Diagnosis

1. Check fingerprint in GoFLE dialog — compare with server admin
2. Manual verify: `openssl s_client -connect host:21 -starttls ftp` (explicit) or `openssl s_client -connect host:990` (implicit)
3. Check `~/.config/gofle/ftp_certs` for existing entry

### Resolution

**First connection (legitimate)**:
1. Verify fingerprint with server admin
2. Click **Trust Host** in GoFLE dialog
3. Cert saved to `ftp_certs`

**Cert renewed (legitimate)**:
1. Remove old entry: edit `~/.config/gofle/ftp_certs`, delete line for `host:port`
2. Reconnect — new cert prompted, verify, trust

**Suspected MITM**:
1. Do NOT trust
2. Contact server admin
3. Verify via independent channel

---

## TLS Handshake Failed

**Symptom**: "TLS handshake failed" / "SSL routines:...:sslv3 alert handshake failure"

### Conditions

- Wrong TLS mode (explicit vs implicit)
- Server doesn't support requested TLS
- Protocol version mismatch
- Cipher mismatch

### Possible Causes

| Cause | Verified |
|-------|----------|
| Wrong TLS mode | Yes |
| Server TLS not configured | Yes |
| Min TLS version too high | Yes |
| Certificate/key mismatch | Yes |

### Diagnosis

1. Verify TLS mode matches server:
   - Plain FTP: `--tls none` (default)
   - Explicit FTPS: `--tls explicit` (port 21)
   - Implicit FTPS: `--tls implicit` (port 990)
2. Test with `lftp`: `lftp -e "set ftp:ssl-force true; open host"`
3. Check server TLS config (vsftpd: `ssl_enable=YES`, `ssl_tlsv1=YES`)

### Resolution

1. Match TLS mode to server configuration
2. Use correct port for mode (21 explicit, 990 implicit)
3. Update server TLS config to support TLS 1.2+
4. Regenerate server certificate if key/cert mismatch

---

## Permission Denied

**Symptom**: "550 Permission denied" / "550 Failed to open file"

### Conditions

- FTP user lacks read permission on file/directory
- File outside chroot jail
- SELinux/AppArmor blocking

### Possible Causes

| Cause | Verified |
|-------|----------|
| No read permission | Yes |
| File outside chroot | Yes |
| SELinux/AppArmor | Yes |

### Diagnosis

1. Test manually: `ftp host` → `get /path/file`
2. Check file perms on server: `ls -la /path/file`
3. Check FTP user home/chroot: `grep user /etc/passwd`
4. Check SELinux: `getsebool -a | grep ftp`

### Resolution

1. Fix file permissions: `chmod +r /path/file`
2. Ensure file within FTP user's chroot directory
3. Allow FTP read: `setsebool -P allow_ftpd_full_access on` (SELinux)
4. Use Browse... in GoFLE to navigate to accessible path

---

## Follow Not Updating

**Symptom**: Follow enabled but new remote lines don't appear

### Conditions

- FTP control connection dropped
- Poll interval too long (default 30s)
- File not growing / rotated
- Data connection fails

### Possible Causes

| Cause | Verified |
|-------|----------|
| Control connection timeout | Yes |
| Poll interval (30s default) | Yes |
| Log rotation | Yes |
| Passive mode data port blocked | Yes |

### Diagnosis

1. Check GoFLE log: `~/.config/gofle/gofle.log` for FTP errors
2. Test manual: `lftp host` → `tail -f /path/log`
3. Check file rotation on server
4. Verify passive ports open in firewall

### Resolution

1. Reduce poll interval: Settings → Remote → (not currently configurable, fixed 30s)
2. Increase server FTP timeout: vsftpd `idle_session_timeout`, `data_connection_timeout`
3. For log rotation: GoFLE reopens on size decrease (automatic)
4. Open passive port range in firewall

---

## Slow Download

**Symptom**: Initial download very slow / stalls

### Conditions

- Large file + slow link
- Stall timeout triggered (default 30s no bytes)
- Server throttling
- Passive mode issues

### Possible Causes

| Cause | Verified |
|-------|----------|
| Slow network | Yes |
| Stall timeout too low | Yes |
| Server bandwidth limit | Possible |
| Passive mode port issues | Yes |

### Diagnosis

1. Check file size: `ftp host` → `size /path/file`
2. Test download speed: `lftp -e "get /path/file -o /dev/null; quit" host`
3. Check GoFLE log for stall timeout messages

### Resolution

1. Increase stall timeout: Settings → Remote → Download Silence Timeout
2. Use Range filter: "Last N lines" to avoid full download
3. Check server bandwidth limits
4. Verify passive mode ports open

---

## Remote File Not Found

**Symptom**: "550 Failed to open file" / "No such file"

### Conditions

- Wrong path
- File moved/rotated
- Case sensitivity (Linux servers)

### Possible Causes

| Cause | Verified |
|-------|----------|
| Typo in path | Yes |
| Case mismatch | Yes |
| File rotated/removed | Yes |

### Diagnosis

1. Use **Browse...** in FTP dialog to navigate
2. Test manually: `ftp host` → `ls /path/`
3. Check exact case: Linux paths case-sensitive

### Resolution

1. Correct path (absolute, case-sensitive)
2. Use Browse to select current file
3. For rotated logs: use date pattern in path

---

## Related Information

- [Open Remote (FTP/FTPS)](../user-guide/open-remote-ftp.md)
- [Remote Settings](../configuration/settings-ui.md#remote-settings)
- [SSH Connection](ssh-connection.md)
- [Data Recovery](data-recovery.md)