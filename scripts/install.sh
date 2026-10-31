#!/bin/bash
set -e

APP="GoFLE"
CLI_APP="gofle"

# 1. Precise directory resolution
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# 2. Detect current platform for binary selection
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

# 3. Find binary for current platform (with fallback to generic name)
find_binary() {
    local app_name="$1"
    local search_dirs=("$SCRIPT_DIR" "$PROJECT_DIR/dist" "$PROJECT_DIR")
    
    # Platform-specific binary names to try (in order of preference)
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
    
    # Return first candidate as default for error message
    echo "${search_dirs[0]}/${candidates[0]}"
    return 1
}

BIN_SRC="$(find_binary "$APP")"
CLI_BIN_SRC="$(find_binary "$CLI_APP")"

# 4. Flexible icon search (build/ in project root or build/ next to script)
if [ -f "$PROJECT_DIR/build/appicon.png" ]; then
    ICON_SRC="$PROJECT_DIR/build/appicon.png"
elif [ -f "$SCRIPT_DIR/build/appicon.png" ]; then
    ICON_SRC="$SCRIPT_DIR/build/appicon.png"
else
    ICON_SRC="$PROJECT_DIR/build/appicon.png" # Default for error message
fi

# Default values
MODE="user"
APP_NAME="GoFLE"
APP_COMMENT="Go Fast Log Explorer"
APP_TERMINAL="false"
SELECTED_CATEGORIES=("Development" "Utility" "TextEditor")
INTERACTIVE=false
INSTALL_CLI=false

# Available standard Freedesktop categories
ALL_CATEGORIES=("Development" "Utility" "TextEditor" "System" "Office" "Graphics" "Network" "AudioVideo")

usage() {
    cat <<EOF
Usage: $0 [options]

Installation modes:
  --user                Install for current user only (~/.local, default)
  --system              Install system-wide (requires sudo/root)

Customization flags:
  -i, --interactive     Launch interactive setup wizard
  --name "NAME"         Custom application name
  --comment "TEXT"      Custom description/comment
  --terminal            Run application inside terminal (Terminal=true)
  --categories "C1;C2;" Custom categories separated by ';'

CLI options:
  --install-cli         Also install the 'gofle' CLI companion binary

General:
  -h, --help            Show this help message
EOF
}

# Argument parsing
while [[ $# -gt 0 ]]; do
    case "$1" in
        --user) MODE="user"; shift;;
        --system) MODE="system"; shift;;
        -i|--interactive) INTERACTIVE=true; shift;;
        --name) APP_NAME="$2"; shift 2;;
        --comment) APP_COMMENT="$2"; shift 2;;
        --terminal) APP_TERMINAL="true"; shift;;
        --categories) 
            IFS=';' read -r -a SELECTED_CATEGORIES <<< "$2"
            shift 2;;
        --install-cli) INSTALL_CLI=true; shift;;
        -h|--help) usage; exit 0;;
        *) echo "Unknown argument: $1" >&2; usage; exit 1;;
    esac
done

# File existence check with informative output
if [ ! -f "$BIN_SRC" ]; then
    echo "Error: Executable file '$APP' not found!" >&2
    echo "Checked locations for platform '$PLATFORM':" >&2
    echo "  - $SCRIPT_DIR/GoFLE-${PLATFORM_OS}-${PLATFORM_ARCH}" >&2
    echo "  - $SCRIPT_DIR/GoFLE" >&2
    echo "  - $PROJECT_DIR/dist/GoFLE-${PLATFORM_OS}-${PLATFORM_ARCH}" >&2
    echo "  - $PROJECT_DIR/dist/GoFLE" >&2
    echo "Run ./build.sh first (or ./build.sh -os all -arch all for multi-platform)." >&2
    exit 1
fi

if [ "$INSTALL_CLI" = true ] && [ ! -f "$CLI_BIN_SRC" ]; then
    echo "Warning: CLI binary '$CLI_APP' not found for platform '$PLATFORM', skipping CLI install." >&2
    echo "Checked locations:" >&2
    echo "  - $SCRIPT_DIR/gofle-${PLATFORM_OS}-${PLATFORM_ARCH}" >&2
    echo "  - $SCRIPT_DIR/gofle" >&2
    echo "  - $PROJECT_DIR/dist/gofle-${PLATFORM_OS}-${PLATFORM_ARCH}" >&2
    echo "  - $PROJECT_DIR/dist/gofle" >&2
    INSTALL_CLI=false
fi

if [ ! -f "$ICON_SRC" ]; then
    echo "Error: Icon 'appicon.png' not found!" >&2
    echo "Checked locations:" >&2
    echo "  - $PROJECT_DIR/build/appicon.png" >&2
    echo "  - $SCRIPT_DIR/build/appicon.png" >&2
    exit 1
fi

# Interactive setup wizard
if [ "$INTERACTIVE" = true ]; then
    echo "=== Desktop Entry Setup ==="
    
    # 1. Text input with default values
    read -p "Application name [$APP_NAME]: " input_name
    APP_NAME="${input_name:-$APP_NAME}"

    read -p "Application description [$APP_COMMENT]: " input_comment
    APP_COMMENT="${input_comment:-$APP_COMMENT}"

    # 2. Toggle/Checkbox for terminal
    read -p "Run in terminal? (y/N): " input_term
    if [[ "$input_term" =~ ^[Yy]$ ]]; then
        APP_TERMINAL="true"
    else
        APP_TERMINAL="false"
    fi

    # 3. Category selection from options
    echo ""
    echo "Select categories (enter numbers separated by space, e.g. 1 2 3):"
    for i in "${!ALL_CATEGORIES[@]}"; do
        echo "  $((i+1))) ${ALL_CATEGORIES[$i]}"
    done
    read -p "Your choice [default 1 2 3]: " input_cats

    if [ -n "$input_cats" ]; then
        SELECTED_CATEGORIES=()
        for num in $input_cats; do
            idx=$((num-1))
            if [ $idx -ge 0 ] && [ $idx -lt ${#ALL_CATEGORIES[@]} ]; then
                SELECTED_CATEGORIES+=("${ALL_CATEGORIES[$idx]}")
            fi
        done
    fi
fi

# Construct categories string
CATEGORIES_STRING=""
for cat in "${SELECTED_CATEGORIES[@]}"; do
    [ -n "$cat" ] && CATEGORIES_STRING="${CATEGORIES_STRING}${cat};"
done

# Function to generate .desktop file
generate_desktop_file() {
    local exec_path="$1"
    local icon_path="$2"
    cat <<EOF
[Desktop Entry]
Name=$APP_NAME
Comment=$APP_COMMENT
Exec=$exec_path %F
Icon=$icon_path
Terminal=$APP_TERMINAL
Type=Application
Categories=$CATEGORIES_STRING
MimeType=text/plain;text/x-log;
EOF
}

# Installation
if [ "$MODE" = "user" ]; then
    PREFIX="$HOME/.local"
    BIN_DIR="$PREFIX/bin"
    ICON_DIR="$PREFIX/share/icons/hicolor/512x512/apps"
    APP_DIR="$PREFIX/share/applications"
    
    mkdir -p "$BIN_DIR" "$ICON_DIR" "$APP_DIR"
    
    install -Dm755 "$BIN_SRC" "$BIN_DIR/$APP"
    install -Dm644 "$ICON_SRC" "$ICON_DIR/$APP.png"
    
    generate_desktop_file "$BIN_DIR/$APP" "$ICON_DIR/$APP.png" > "$APP_DIR/$APP.desktop"
    chmod 644 "$APP_DIR/$APP.desktop"

    # Install CLI if requested
    if [ "$INSTALL_CLI" = true ] && [ -f "$CLI_BIN_SRC" ]; then
        install -Dm755 "$CLI_BIN_SRC" "$BIN_DIR/$CLI_APP"
        echo "Installed CLI '$CLI_APP' to $BIN_DIR"
    fi
    
    echo "Installed $APP ($APP_NAME) for current user into $PREFIX"
else
    if [ "$(id -u)" -ne 0 ]; then
        if command -v sudo >/dev/null 2>&1; then
            echo "System-wide install requires root. Re-running with sudo..."
            
            TERM_FLAG=""
            if [ "$APP_TERMINAL" = "true" ]; then
                TERM_FLAG="--terminal"
            fi
            
            CLI_FLAG=""
            if [ "$INSTALL_CLI" = true ]; then
                CLI_FLAG="--install-cli"
            fi
            
            exec sudo "$0" --system \
                ${INTERACTIVE:+--interactive} \
                --name "$APP_NAME" \
                --comment "$APP_COMMENT" \
                $TERM_FLAG \
                $CLI_FLAG \
                --categories "$CATEGORIES_STRING"
        else
            echo "Error: system-wide install needs root privileges, but 'sudo' was not found." >&2
            exit 1
        fi
    fi

    BIN_DEST="/usr/local/bin/$APP"
    ICON_DEST="/usr/share/icons/hicolor/512x512/apps/$APP.png"
    APP_DEST="/usr/share/applications/$APP.desktop"

    install -Dm755 "$BIN_SRC" "$BIN_DEST"
    install -Dm644 "$ICON_SRC" "$ICON_DEST"
    
    generate_desktop_file "$BIN_DEST" "$APP.png" > "$APP_DEST"
    chmod 644 "$APP_DEST"
    
    # Install CLI if requested
    if [ "$INSTALL_CLI" = true ] && [ -f "$CLI_BIN_SRC" ]; then
        install -Dm755 "$CLI_BIN_SRC" "/usr/local/bin/$CLI_APP"
        echo "Installed CLI '$CLI_APP' to /usr/local/bin"
    fi
    
    gtk-update-icon-cache -f /usr/share/icons/hicolor 2>/dev/null || true

    echo "Installed $APP ($APP_NAME) system-wide."
fi