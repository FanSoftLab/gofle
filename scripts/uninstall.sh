#!/bin/bash
set -e

APP="GoFLE"
MODE="user"

usage() {
    cat <<EOF
Usage: $0 [options]

Uninstall options:
  --user    Uninstall for current user only (~/.local, default)
  --system  Uninstall system-wide (requires sudo/root)
  -h, --help  Show this help message
EOF
}

# Argument parsing
while [[ $# -gt 0 ]]; do
    case "$1" in
        --user) MODE="user"; shift;;
        --system) MODE="system"; shift;;
        -h|--help) usage; exit 0;;
        *) echo "Unknown argument: $1" >&2; usage; exit 1;;
    esac
done

# Function to remove a file if it exists
remove_file() {
    local file_path="$1"
    if [ -f "$file_path" ]; then
        rm -f "$file_path"
        echo "Removed: $file_path"
    else
        echo "Not found (skipped): $file_path"
    fi
}

if [ "$MODE" = "user" ]; then
    PREFIX="$HOME/.local"
    BIN_FILE="$PREFIX/bin/$APP"
    ICON_FILE="$PREFIX/share/icons/hicolor/512x512/apps/$APP.png"
    APP_FILE="$PREFIX/share/applications/$APP.desktop"

    echo "Uninstalling $APP for current user..."
    remove_file "$BIN_FILE"
    remove_file "$ICON_FILE"
    remove_file "$APP_FILE"

    echo "Successfully uninstalled $APP from $PREFIX"
else
    if [ "$(id -u)" -ne 0 ]; then
        if command -v sudo >/dev/null 2>&1; then
            echo "System-wide uninstall requires root. Re-running with sudo..."
            exec sudo "$0" --system "$@"
        else
            echo "Error: system-wide uninstall needs root privileges, but 'sudo' was not found." >&2
            echo "Run this script as root, or use '$0 --user' for user-only uninstall." >&2
            exit 1
        fi
    fi

    BIN_FILE="/usr/local/bin/$APP"
    ICON_FILE="/usr/share/icons/hicolor/512x512/apps/$APP.png"
    APP_FILE="/usr/share/applications/$APP.desktop"

    echo "Uninstalling $APP system-wide..."
    remove_file "$BIN_FILE"
    remove_file "$ICON_FILE"
    remove_file "$APP_FILE"

    gtk-update-icon-cache -f /usr/share/icons/hicolor 2>/dev/null || true

    echo "Successfully uninstalled $APP system-wide."
fi