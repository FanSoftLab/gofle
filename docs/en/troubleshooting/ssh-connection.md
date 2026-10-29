# SSH Connection Troubleshooting

> **Content type**: Troubleshooting — diagnose and resolve SSH connection issues.
> **Audience**: All users
> **Severity**: High (blocks remote access)
> **Verified**: 2026-10-29 — tested against common SSH failure modes

## Purpose

Resolve SSH connection, authentication, host key, and follow issues.

---

## Symptoms Index

| Symptom | Jump To |
|---------|---------|
| "Connection refused" | [Connection Refused](#connection-refused) |
| "Connection timeout" | [Connection Timeout](#connection-timeout) |
| "Authentication failed" | [Authentication Failed](#authentication-failed) |
| "Host key verification failed" | [Host Key Verification Failed](#host-key-verification-failed) |
| "Permission denied (publickey)" | [Public Key Rejected](#public-key-rejected) |
| Follow not updating | [Follow Not Updating](#follow-not-updating) |
| Slow initial load | [Slow Initial Load](#slow-initial-load) |
| Remote file not found | [Remote File Not Found](#remote-file-not-found) |

---

## Connection Refused

**Symptom**: `ssh: connect to host X port 22: Connection refused`

### Conditions

- SSH port (22 or custom) not reachable
- Firewall blocking
- SSH service not running on server

### Possible Causes

| Cause | Verified |
|-------|----------|
| SSH server not running | Yes |
| Wrong port | Yes |
| Firewall (server or network) | Yes |
| Host IP changed | Yes |

### Diagnosis

1. Test with standard SSH: `ssh user@host`
   - **Works**: GoFLE config issue
   - **Fails**: Network/server issue
2. Check port: `nc -zv host 22` (or custom port)
3. Verify server SSH status: `systemctl status sshd` (on server)

### Resolution

1. Start SSH server: `sudo systemctl start sshd`
2. Use correct port: `-p <port>` in GoFLE
3. Open firewall: `sudo ufw allow 22/tcp` (or cloud security group)
4. Verify host IP in GoFLE connection form

---

## Connection Timeout

**Symptom**: "Connecting to server..." → timeout after N seconds (default 15s)

### Conditions

- High latency / slow network
- Server under load
- Connection timeout setting too low

### Possible Causes

| Cause | Verified |
|-------|----------|
| Network latency > timeout | Yes |
| Server busy | Yes |
| Timeout setting too low | Yes |

### Diagnosis

1. Ping host: `ping host` — check latency
2. Test SSH manually: `ssh -o ConnectTimeout=30 user@host`
3. Check GoFLE timeout setting: Settings → Remote → Connection Timeout

### Resolution

1. Increase timeout: Settings → Remote → Connection Timeout (max 120s)
2. Use faster network / VPN
3. Check server load: `top` / `htop` on server

---

## Authentication Failed

**Symptom**: "Authentication failed" or repeated password prompts

### Conditions

- Wrong password
- Wrong username
- Account locked / expired
- SSH server config rejects method

### Possible Causes

| Cause | Verified |
|-------|----------|
| Incorrect password | Yes |
| Incorrect username | Yes |
| Password auth disabled on server | Yes |
| Account locked | Yes |

### Diagnosis

1. Test manually: `ssh user@host` — same credentials
2. Check server auth log: `/var/log/auth.log` (Linux)
3. Verify username in GoFLE (case-sensitive)
4. Try key auth if password fails

### Resolution

1. Correct password/username
2. Enable password auth on server: `PasswordAuthentication yes` in `/etc/ssh/sshd_config`
3. Unlock account: `passwd -u user` (Linux)
4. Use SSH key auth instead (more reliable)

---

## Host Key Verification Failed

**Symptom**: "Host key verification failed" / "Key changed" / fingerprint mismatch

### Conditions

- First connection to host (TOFU)
- Server reinstalled / SSH reinstalled
- MITM attack (rare but possible)
- Multiple IPs for same hostname

### Possible Causes

| Cause | Verified |
|-------|----------|
| First connection (unknown key) | Yes |
| Server reinstalled | Yes |
| Legitimate key rotation | Yes |
| MITM attack | Possible |

### Diagnosis

1. Check fingerprint in GoFLE dialog — compare with server admin
2. Manual verify: `ssh-keyscan -t ed25519 host` → compare SHA256
3. Check `~/.config/gofle/known_hosts` for existing entry

### Resolution

**First connection (legitimate)**:
1. Verify fingerprint with server admin (out-of-band)
2. Click **Trust Host** in GoFLE dialog
3. Key saved to `known_hosts`

**Key changed (legitimate rotation)**:
1. Remove old key: edit `~/.config/gofle/known_hosts`, delete line for host
2. Reconnect — new key prompted, verify, trust

**Suspected MITM**:
1. Do NOT trust
2. Contact server admin immediately
3. Verify via independent channel

---

## Public Key Rejected

**Symptom**: "Permission denied (publickey)" / "No supported authentication methods"

### Conditions

- Key not in `authorized_keys`
- Wrong key file
- Key permissions too open
- Server rejects key type

### Possible Causes

| Cause | Verified |
|-------|----------|
| Public key not authorized | Yes |
| Wrong private key path | Yes |
| Key permissions > 600 | Yes |
| Key type not supported | Yes |

### Diagnosis

1. Test manually: `ssh -i /path/to/key user@host`
2. Check key permissions: `ls -l ~/.ssh/id_*` — must be 600
3. Verify public key in server `~/.ssh/authorized_keys`
4. Check server log: `/var/log/auth.log`

### Resolution

1. Fix key path in GoFLE (absolute path required)
2. Set key permissions: `chmod 600 ~/.ssh/id_ed25519`
3. Add public key to server `authorized_keys`
4. Use supported key type: Ed25519, RSA (2048+), ECDSA

---

## Follow Not Updating

**Symptom**: Follow enabled ("Tail: ACTIVE") but new remote lines don't appear

### Conditions

- SSH connection dropped silently
- Remote file rotated (logrotate)
- Poll interval too long
- Server not appending

### Possible Causes

| Cause | Verified |
|-------|----------|
| SSH session died | Yes |
| Log rotation | Yes |
| Poll interval (2s default) | Yes |
| File not growing | Yes |

### Diagnosis

1. Check GoFLE log: `~/.config/gofle/gofle.log` for SSH errors
2. Test manual tail: `ssh user@host "tail -f /path/log"`
3. Check file rotation: `ls -la /path/log` on server
4. Verify Follow button shows "Tail: ACTIVE"

### Resolution

1. Reconnect: close tab, reopen from Recent
2. Increase SSH keepalive: server `ClientAliveInterval 30` in `sshd_config`
3. For log rotation: ensure GoFLE reopens file (automatic on size decrease)
4. Reduce poll interval: not currently configurable (fixed 2s)

---

## Slow Initial Load

**Symptom**: "Downloading log..." takes very long / stalls

### Conditions

- Very large remote file (GB+)
- Slow network
- Stall timeout triggered (default 30s no bytes)

### Possible Causes

| Cause | Verified |
|-------|----------|
| Large file + slow link | Yes |
| Stall timeout too low | Yes |
| Server throttling | Possible |

### Diagnosis

1. Check file size on server: `ssh user@host "ls -lh /path/log"`
2. Monitor network: `iftop` / `nethogs`
3. Check GoFLE log for "stall timeout" messages

### Resolution

1. Increase stall timeout: Settings → Remote → Download Silence Timeout
2. Use Range filter: "Last N lines" to avoid full download
3. Compress on server: `ssh user@host "gzip -c /path/log"` then download (manual)

---

## Remote File Not Found

**Symptom**: "Remote file not found" or "No such file"

### Conditions

- Wrong path
- File moved/rotated
- Permission denied (shows as not found sometimes)

### Possible Causes

| Cause | Verified |
|-------|----------|
| Typo in path | Yes |
| File rotated/removed | Yes |
| No read permission | Yes |

### Diagnosis

1. Use **Browse...** button in SSH dialog to navigate
2. Test manually: `ssh user@host "ls -la /path/"`
3. Check permissions: `ssh user@host "test -r /path/log && echo OK"`

### Resolution

1. Correct path (absolute path required)
2. Use Browse to select current file
3. Fix permissions on server: `chmod +r /path/log`

---

## Related Information

- [Open Remote (SSH)](../user-guide/open-remote-ssh.md)
- [Remote Settings](../configuration/settings-ui.md#remote-settings)
- [FTP Connection](ftp-connection.md)
- [Data Recovery](data-recovery.md)