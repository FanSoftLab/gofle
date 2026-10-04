# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 2.0.x   | :white_check_mark: |
| < 2.0   | :x:                |

Only the latest major version (2.x) receives security updates. Users are
strongly encouraged to upgrade to the latest release.

## Reporting a Vulnerability

**Please do NOT open a public issue for security vulnerabilities.**

If you discover a security vulnerability in GoFLE, please report it
responsibly by emailing **nikolya@proton.me** with the following details:

1. **Description** — A clear description of the vulnerability and its impact
2. **Steps to reproduce** — Minimal steps to demonstrate the issue
3. **Affected versions** — Which version(s) you tested (output of `gofle version`)
4. **Proof of concept** — If applicable, a minimal PoC (code, commands, or screenshots)
5. **Suggested fix** — If you have ideas for remediation

You will receive an acknowledgment within **48 hours**. We aim to provide a
preliminary assessment within **7 days** and a fix timeline within **14 days**.

## Disclosure Policy

- We follow **coordinated vulnerability disclosure**: the reporter gets credit
  (unless they request anonymity) once a fix is released.
- We ask reporters to allow us **90 days** to address the issue before public
  disclosure, with possible extensions for complex fixes.
- If a vulnerability is actively exploited in the wild, we may expedite the
  timeline and publish an advisory sooner.

## Security Features in GoFLE

GoFLE is a local-first desktop log viewer. Its security model assumes the
local user is trusted. Key security measures include:

| Area | Implementation |
|------|----------------|
| **SSH credentials** | Encrypted at rest with AES-GCM (256-bit key, random per-install) |
| **SSH host keys** | Verified against OpenSSH `known_hosts`; TOFU on first connect with fingerprint confirmation |
| **FTPS certificates** | Pinned in `~/.config/GoFLE/ftp_certs`; unknown certs require explicit trust |
| **Temp files** | Remote logs streamed to `os.CreateTemp()` with `GoFLE-ssh-*.log` prefix; auto-deleted on tab close |
| **No telemetry** | Zero network calls except explicit SSH/FTP connections initiated by user |
| **No auto-update** | No background update checks or binary downloads |

## Known Limitations

- **Local user trust model**: Anyone with access to your user account can read the config directory (`~/.config/GoFLE/`) and decrypt stored SSH credentials.
- **No sandboxing**: The app runs with your user privileges; a compromised log file (e.g., malicious escape sequences) could theoretically exploit terminal parsing bugs in the frontend. We mitigate this by sanitizing output and using a webview renderer.
- **WebView dependencies**: Linux uses WebKitGTK; Windows uses WebView2. Keep your OS updated for WebView security patches.

## Security-Related Configuration

Users can enhance security by:

- Setting restrictive permissions on `~/.config/GoFLE/` (`chmod 700`)
- Using SSH key auth instead of passwords
- Regularly reviewing `known_hosts` and `ftp_certs` for unexpected entries
- Running the app under a dedicated low-privilege user if processing untrusted logs

## Contact

For security questions or concerns not covered here, email **nikolya@proton.me**.