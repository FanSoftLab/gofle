# Prerequisites

> **Content type**: Reference — system requirements for building and running GoFLE.
> **Audience**: All users (end users, developers, contributors)
> **Verified**: 2026-10-29 — verified against build scripts and Wails documentation

## Purpose

This page lists all verified requirements to build and run GoFLE on each platform.

## All Platforms

| Requirement | Version | Verified | Notes |
|-------------|---------|----------|-------|
| Go | 1.27+ | Yes | Required for backend compilation |
| Node.js | 18+ | Yes | Required for frontend build |
| npm | 9+ | Yes | Bundled with Node.js |
| Wails CLI v2 | latest | Yes | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| Git | any | Yes | For cloning repository |

## Linux (Native Build)

### System Libraries

The WebKitGTK webview requires these development packages:

**Debian / Ubuntu**
```bash
sudo apt-get update
sudo apt-get install -y build-essential pkg-config \
    libgtk-3-dev libwebkit2gtk-4.1-dev
```

**Fedora**
```bash
sudo dnf install gtk3-devel webkit2gtk4.1-devel gcc pkgconf-pkg-config
```

**Arch Linux**
```bash
sudo pacman -S gtk3 webkit2gtk-4.1 base-devel pkgconf
```

### CGO

CGO must be enabled (default on Linux). The `build.sh` script handles this automatically.

## macOS

| Requirement | Command | Verified |
|-------------|---------|----------|
| Xcode Command Line Tools | `xcode-select --install` | Yes |

No additional system libraries required. WebView is provided by macOS.

## Windows

| Requirement | Source | Verified |
|-------------|--------|----------|
| C Compiler (MSVC or MinGW) | [Visual Studio Build Tools](https://visualstudio.microsoft.com/downloads/#build-tools-for-visual-studio-2022) or [MinGW-w64](https://www.mingw-w64.org/) | Yes |
| WebView2 Runtime | [Microsoft Edge WebView2](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) | Yes |

Wails v2 uses WebView2 on Windows. The runtime is included in Windows 11; install manually on Windows 10.

## Docker Build (All Platforms)

The `./build.sh` script uses a Docker image with all dependencies pre-installed.

| Requirement | Version | Verified |
|-------------|---------|----------|
| Docker | 20.10+ | Yes |
| Docker BuildKit | Enabled | Yes |

No host Go, Node.js, or system libraries required for Docker builds.

## Verification

Run these commands to verify your environment:

```bash
go version        # Should show 1.27+
node --version    # Should show v18+
npm --version     # Should show 9+
wails version     # Should show v2.x
docker --version  # For Docker builds
```

## Related Information

- [Docker Build Procedure](install-docker.md)
- [Local Build Procedure](install-local.md)
- [Linux Package Install](install-linux.md)
- [First Run](first-run.md)