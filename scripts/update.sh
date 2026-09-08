#!/bin/bash
set -e

APP="GoFLE"

# 1. Визначення шляхів до директорій проєкту
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

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
PASSTHROUGH_ARGS=()

usage() {
    cat <<EOF
Usage: $0 [options]

Update options:
  --user              Update for current user only (~/.local, default)
  --system            Update system-wide (requires sudo/root)
  --no-build          Skip running build script before update

Customization flags (passed to installer):
  -i, --interactive   Launch interactive setup wizard during update
  --name "NAME"       Custom application name
  --comment "TEXT"    Custom description/comment
  --terminal          Run application inside terminal (Terminal=true)
  --categories "C1;"  Custom categories

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
        -i|--interactive|--terminal)
            PASSTHROUGH_ARGS+=("$1")
            shift;;
        --name|--comment|--categories)
            PASSTHROUGH_ARGS+=("$1" "$2")
            shift 2;;
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

# 3. Делегування оновлення (перевстановлення) через install.sh або пряме копіювання
if [ -n "$INSTALL_SCRIPT" ]; then
    echo "Re-installing/Updating $APP using $INSTALL_SCRIPT..."
    bash "$INSTALL_SCRIPT" "${PASSTHROUGH_ARGS[@]}"
else
    echo "Warning: install.sh not found. Performing fallback update..."

    # Пошук джерел бінарного файлу та іконки
    if [ -f "$SCRIPT_DIR/$APP" ]; then
        BIN_SRC="$SCRIPT_DIR/$APP"
    else
        BIN_SRC="$PROJECT_DIR/$APP"
    fi

    if [ -f "$PROJECT_DIR/build/appicon.png" ]; then
        ICON_SRC="$PROJECT_DIR/build/appicon.png"
    else
        ICON_SRC="$SCRIPT_DIR/build/appicon.png"
    fi

    if [ ! -f "$BIN_SRC" ]; then
        echo "Error: Executable '$APP' not found for update!" >&2
        exit 1
    fi

    if [ "$MODE" = "user" ]; then
        PREFIX="$HOME/.local"
        install -Dm755 "$BIN_SRC" "$PREFIX/bin/$APP"
        [ -f "$ICON_SRC" ] && install -Dm644 "$ICON_SRC" "$PREFIX/share/icons/hicolor/512x512/apps/$APP.png"
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
        gtk-update-icon-cache -f /usr/share/icons/hicolor 2>/dev/null || true
        echo "Updated $APP system-wide."
    fi
fi

echo "Successfully updated $APP!"