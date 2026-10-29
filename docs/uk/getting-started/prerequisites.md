# Передумови

> **Тип контенту**: Reference — системні вимоги для білду та запуску GoFLE.
> **Аудиторія**: Всі користувачі (кільцевий користувач, розробник, контриб'ютор)
> **Verified**: 2026-10-29 — перевірено проти білд-скриптів та Wails документації

## Призначення

Ця сторінка перелічує всі перевірені вимоги для білду та запуску GoFLE на кожній платформі.

## Усі платформи

| Вимога | Версія | Verified | Примітки |
|----------|--------|----------|----------|
| Go | 1.27+ | Yes | Потрібен для компіляції бекенду |
| Node.js | 18+ | Yes | Потрібен для білду фронтенду |
| npm | 9+ | Yes | В комплекті з Node.js |
| Wails CLI v2 | latest | Yes | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| Git | any | Yes | Для клонування репозиторію |

## Linux (Native Build)

### Системні бібліотеки

WebKitGTK webview вимагає ці development пакети:

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

CGO має бути увімкненим (за замовчуванням на Linux). Скрипт `build.sh` обробляє це автоматично.

## macOS

| Вимога | Команда | Verified |
|----------|---------|----------|
| Xcode Command Line Tools | `xcode-select --install` | Yes |

Додаткові системні бібліотеки не потрібні. WebView надається macOS.

## Windows

| Вимога | Джерело | Verified |
|----------|---------|----------|
| C Compiler (MSVC або MinGW) | [Visual Studio Build Tools](https://visualstudio.microsoft.com/downloads/#build-tools-for-visual-studio-2022) або [MinGW-w64](https://www.mingw-w64.org/) | Yes |
| WebView2 Runtime | [Microsoft Edge WebView2](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) | Yes |

Wails v2 використовує WebView2 на Windows. Runtime включений у Windows 11; встановіть вручну на Windows 10.

## Docker Build (Усі платформи)

Скрипт `./build.sh` використовує Docker образ з усіма залежностями, попередньо встановленими.

| Вимога | Версія | Verified |
|----------|--------|----------|
| Docker | 20.10+ | Yes |
| Docker BuildKit | Enabled | Yes |

Жодних хостових Go, Node.js, або системних бібліотек не потрібно для Docker білдів.

## Перевірка

Запустіть ці команди для перевірки вашого середовища:

```bash
go version        # Повинен показати 1.27+
node --version    # Повинен показати v18+
npm --version     # Повинен показати 9+
wails version     # Повинен показати v2.x
docker --version  # Для Docker білдів
```

## Пов'язані теми

- [Процедура білду Docker](install-docker.md)
- [Процедура локального білду](install-local.md)
- [Встановлення пакета Linux](install-linux.md)
- [Перший запуск](first-run.md)