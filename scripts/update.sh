#!/bin/bash
set -e

APP="gofle"
CLI_APP="gofle_cli"

# 1. Визначення шляхів до директорій проєкту
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Detect current platform for binary selection
detect_platform() {
    local os arch
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"
    
    case "$arch" in
        x86_64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        armv7l) arch="arm" ;;
        i386|i686) arch="386" ;;
    esac
    
    echo "${os}/${arch}"
}

PLATFORM="$(detect_platform)"
PLATFORM_OS="${PLATFORM%/*}"
PLATFORM_ARCH="${PLATFORM#*/}"

# Find binary for current platform (with fallback to generic name)
find_binary() {
    local app_name="$1"
    local search_dirs=("$SCRIPT_DIR" "$PROJECT_DIR/dist" "$PROJECT_DIR")
    
    local candidates=()
    
    if [ "$PLATFORM_OS" = "windows" ] || [ "$PLATFORM_OS" = "mingw" ] || [ "$PLATFORM_OS" = "msys" ]; then
        candidates+=("${app_name}-${PLATFORM_OS}-${PLATFORM_ARCH}.exe")
        candidates+=("${app_name}.exe")
    else
        candidates+=("${app_name}-${PLATFORM_OS}-${PLATFORM_ARCH}")
        candidates+=("${app_name}")
    fi
    
    for dir in "${search_dirs[@]}"; do
        for candidate in "${candidates[@]}"; do
            if [ -f "$dir/$candidate" ]; then
                echo "$dir/$candidate"
                return 0
            fi
        done
    done
    
    echo "${search_dirs[0]}/${candidates[0]}"
    return 1
}

# Get version from CLI binary (GUI doesn't expose --version)
get_binary_version() {
    local binary_path="$1"
    if [ -f "$binary_path" ] && [ -x "$binary_path" ]; then
        "$binary_path" --version 2>/dev/null | head -1 | awk '{print $2}' || echo "unknown"
    else
        echo "unknown"
    fi
}

# Get version using CLI binary (both GUI and CLI share same version)
get_version_from_cli() {
    local cli_binary="$1"
    if [ -f "$cli_binary" ] && [ -x "$cli_binary" ]; then
        "$cli_binary" --version 2>/dev/null | head -1 | awk '{print $2}' || echo "unknown"
    else
        echo "unknown"
    fi
}

# Пошук build.sh
BUILD_SCRIPT=""
if [ -f "$SCRIPT_DIR/build.sh" ]; then
    BUILD_SCRIPT="$SCRIPT_DIR/build.sh"
elif [ -f "$PROJECT_DIR/build.sh" ]; then
    BUILD_SCRIPT="$PROJECT_DIR/build.sh"
fi

# Знаходження install.sh для делегування встановлення
INSTALL_SCRIPT=""
if [ -f "$SCRIPT_DIR/install.sh" ]; then
    INSTALL_SCRIPT="$SCRIPT_DIR/install.sh"
elif [ -f "$PROJECT_DIR/install.sh" ]; then
    INSTALL_SCRIPT="$PROJECT_DIR/install.sh"
fi

# Значення за замовчуванням
MODE="user"
NO_BUILD=false
FORCE_UPDATE=false
PASSTHROUGH_ARGS=()

usage() {
    cat <<EOF
Usage: $0 [options]

Update options:
  --user              Update for current user only (~/.local, default)
  --system            Update system-wide (requires sudo/root)
  --no-build          Skip running build script before update
  --force             Force update even if version is the same

Customization flags (passed to installer):
  -i, --interactive   Launch interactive setup wizard during update
  --name "NAME"       Custom application name
  --comment "TEXT"    Custom description/comment
  --terminal          Run application inside terminal (Terminal=true)
  --categories "C1;"  Custom categories

CLI options:
  --install-cli       Also install/update the 'gofle' CLI companion binary

General:
  -h, --help          Show this help message
EOF
}

# Парсинг аргументів
while [[ $# -gt 0 ]]; do
    case "$1" in
        --user) 
            MODE="user"
            PASSTHROUGH_ARGS+=("--user")
            shift;;
        --system) 
            MODE="system"
            PASSTHROUGH_ARGS+=("--system")
            shift;;
        --no-build) 
            NO_BUILD=true
            shift;;
        --force) 
            FORCE_UPDATE=true
            shift;;
        -i|--interactive|--terminal)
            PASSTHROUGH_ARGS+=("$1")
            shift;;
        --name|--comment|--categories)
            PASSTHROUGH_ARGS+=("$1" "$2")
            shift 2;;
        --install-cli)
            PASSTHROUGH_ARGS+=("--install-cli")
            shift;;
        -h|--help) 
            usage
            exit 0;;
        *) 
            echo "Unknown argument: $1" >&2
            usage
            exit 1;;
    esac
done

echo "=== Updating $APP ==="

# Determine installation paths to check current version
if [ "$MODE" = "user" ]; then
    CURRENT_BIN="$HOME/.local/bin/$APP"
    CURRENT_CLI_BIN="$HOME/.local/bin/$CLI_APP"
else
    CURRENT_BIN="/usr/local/bin/$APP"
    CURRENT_CLI_BIN="/usr/local/bin/$CLI_APP"
fi

# Get current installed version (use CLI binary since GUI doesn't expose --version)
CURRENT_VERSION="$(get_version_from_cli "$CURRENT_CLI_BIN")"
if [ "$CURRENT_VERSION" != "unknown" ]; then
    echo "Current installed version: $CURRENT_VERSION"
else
    echo "Current version: not installed or unable to determine"
fi

# 2. Збірка проєкту (якщо не передано --no-build)
if [ "$NO_BUILD" = false ]; then
    if [ -n "$BUILD_SCRIPT" ] && [ -x "$BUILD_SCRIPT" ]; then
        echo "Running build script: $BUILD_SCRIPT"
        "$BUILD_SCRIPT"
    elif [ -n "$BUILD_SCRIPT" ]; then
        echo "Running build script via bash: $BUILD_SCRIPT"
        bash "$BUILD_SCRIPT"
    else
        echo "Warning: build.sh not found. Proceeding with existing binaries..."
    fi
fi

# Find new binaries after build
BIN_SRC="$(find_binary "$APP")"
CLI_BIN_SRC="$(find_binary "$CLI_APP")"

# Get new version (use CLI binary since GUI doesn't expose --version)
NEW_VERSION="$(get_version_from_cli "$CLI_BIN_SRC")"
if [ "$NEW_VERSION" != "unknown" ]; then
    echo "New version: $NEW_VERSION"
else
    echo "New version: unable to determine"
fi

# Version comparison - skip if same version and not forced
if [ "$CURRENT_VERSION" != "unknown" ] && [ "$NEW_VERSION" != "unknown" ] && [ "$CURRENT_VERSION" = "$NEW_VERSION" ] && [ "$FORCE_UPDATE" = false ]; then
    echo "Version unchanged ($CURRENT_VERSION). Use --force to update anyway."
    exit 0
fi

# 3. Делегування оновлення (перевстановлення) через install.sh або пряме копіювання
if [ -n "$INSTALL_SCRIPT" ]; then
    echo "Re-installing/Updating $APP using $INSTALL_SCRIPT..."
    bash "$INSTALL_SCRIPT" "${PASSTHROUGH_ARGS[@]}"
else
    echo "Warning: install.sh not found. Performing fallback update..."

    # Пошук джерел бінарного файлу та іконки
    if [ ! -f "$BIN_SRC" ]; then
        echo "Error: Executable '$APP' not found for update!" >&2
        echo "Checked locations for platform '$PLATFORM':" >&2
        echo "  - $SCRIPT_DIR/GoFLE-${PLATFORM_OS}-${PLATFORM_ARCH}" >&2
        echo "  - $SCRIPT_DIR/GoFLE" >&2
        echo "  - $PROJECT_DIR/dist/GoFLE-${PLATFORM_OS}-${PLATFORM_ARCH}" >&2
        echo "  - $PROJECT_DIR/dist/GoFLE" >&2
        exit 1
    fi

    if [ -f "$PROJECT_DIR/build/appicon.png" ]; then
        ICON_SRC="$PROJECT_DIR/build/appicon.png"
    else
        ICON_SRC="$SCRIPT_DIR/build/appicon.png"
    fi

    if [ "$MODE" = "user" ]; then
        PREFIX="$HOME/.local"
        install -Dm755 "$BIN_SRC" "$PREFIX/bin/$APP"
        [ -f "$ICON_SRC" ] && install -Dm644 "$ICON_SRC" "$PREFIX/share/icons/hicolor/512x512/apps/$APP.png"
        
        if [ -f "$CLI_BIN_SRC" ]; then
            install -Dm755 "$CLI_BIN_SRC" "$PREFIX/bin/$CLI_APP"
        fi
        
        echo "Updated $APP for current user in $PREFIX"
    else
        if [ "$(id -u)" -ne 0 ]; then
            if command -v sudo >/dev/null 2>&1; then
                echo "System-wide update requires root. Re-running with sudo..."
                exec sudo "$0" --system "$@"
            else
                echo "Error: system-wide update needs root privileges, but 'sudo' was not found." >&2
                exit 1
            fi
        fi

        install -Dm755 "$BIN_SRC" "/usr/local/bin/$APP"
        [ -f "$ICON_SRC" ] && install -Dm644 "$ICON_SRC" "/usr/share/icons/hicolor/512x512/apps/$APP.png"
        
        if [ -f "$CLI_BIN_SRC" ]; then
            install -Dm755 "$CLI_BIN_SRC" "/usr/local/bin/$CLI_APP"
        fi
        
        gtk-update-icon-cache -f /usr/share/icons/hicolor 2>/dev/null || true
        echo "Updated $APP system-wide."
    fi
fi

echo "Successfully updated $APP!"
