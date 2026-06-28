#!/usr/bin/env bash
# install.sh - Installer for Linux & macOS

set -euo pipefail

# ANSI color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

# Check if running via sudo
if [ -n "${SUDO_USER:-}" ]; then
  echo -e "${RED}Error: This script should NOT be run with sudo or as root via sudo.${RESET}"
  echo -e "${YELLOW}The statusline installs locally in your home directory (~/.antigravity).${RESET}"
  echo -e "${YELLOW}Running with sudo will cause file permission issues for your user account.${RESET}"
  echo -e "Please run the installation command without sudo:"
  echo -e "${GREEN}curl -fsSL https://raw.githubusercontent.com/bradly0cjw/agycli-statusline/main/install.sh | bash${RESET}"
  exit 1
fi

echo -e "${BLUE}====================================================${RESET}"
echo -e "${GREEN}  Installing Antigravity CLI Statusline (Linux/Mac) ${RESET}"
echo -e "${BLUE}====================================================${RESET}"

# 1. Dependency checks
echo -e "Checking dependencies..."
if ! command -v go &> /dev/null; then
  echo -e "${RED}Error: 'go' (Go compiler) is not installed. Please install Go 1.21+ first.${RESET}"
  exit 1
fi
if ! command -v git &> /dev/null; then
  echo -e "${YELLOW}Warning: 'git' is not installed. Git branch info will not be available.${RESET}"
fi
echo -e "${GREEN}✓ Dependencies checked.${RESET}"

# 2. Setup directory
INSTALL_DIR="$HOME/.antigravity"
echo -e "Creating directory ${INSTALL_DIR}..."
mkdir -p "$INSTALL_DIR"

# 3. Compile and Copy files
SCRIPT_TARGET="${INSTALL_DIR}/statusline"
UNINSTALL_TARGET="${INSTALL_DIR}/uninstall.sh"

LOCAL_DIR=""
if [ -f "$(dirname "$0")/main.go" ]; then
  LOCAL_DIR="$(dirname "$0")"
fi

if [ -n "$LOCAL_DIR" ]; then
  echo -e "Installing from local files..."
  echo -e "Compiling statusline binary..."
  (
    cd "$LOCAL_DIR"
    COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "")
    go build -ldflags="-s -w -X main.commitHash=$COMMIT_HASH" -o "$SCRIPT_TARGET"
  )
  if [ -f "${LOCAL_DIR}/uninstall.sh" ]; then
    cp "${LOCAL_DIR}/uninstall.sh" "$UNINSTALL_TARGET"
    chmod +x "$UNINSTALL_TARGET"
  fi
else
  echo -e "Installing from remote repository..."
  TEMP_DIR=$(mktemp -d)
  git clone --depth 1 https://github.com/bradly0cjw/agycli-statusline.git "$TEMP_DIR"
  echo -e "Compiling statusline binary..."
  (
    cd "$TEMP_DIR"
    COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "")
    go build -ldflags="-s -w -X main.commitHash=$COMMIT_HASH" -o "$SCRIPT_TARGET"
  )
  cp "${TEMP_DIR}/uninstall.sh" "$UNINSTALL_TARGET"
  chmod +x "$UNINSTALL_TARGET"
  rm -rf "$TEMP_DIR"
fi

# 5. Create refresh.command (clickable refresh button target)
REFRESH_COMMAND="${INSTALL_DIR}/refresh.command"
cat > "$REFRESH_COMMAND" << EOF
#!/usr/bin/env bash
# agycli-statusline refresh script
# Clicking this from a terminal hyperlink forces an immediate update check.
"${INSTALL_DIR}/statusline" --refresh
echo "Statusline update cache refreshed."
EOF
chmod +x "$REFRESH_COMMAND"

# 4. Configure settings.json
SETTINGS_FILE="$HOME/.gemini/antigravity-cli/settings.json"
SETTINGS_DIR="$(dirname "$SETTINGS_FILE")"

echo -e "Configuring settings file: ${SETTINGS_FILE}..."
if [ -f "$SETTINGS_FILE" ]; then
  # Make a backup
  cp "$SETTINGS_FILE" "${SETTINGS_FILE}.bak"
  echo -e "${YELLOW}Backed up original settings to ${SETTINGS_FILE}.bak${RESET}"

  # Merge using inline Go script (removes jq dependency)
  TEMP_GO=$(mktemp).go
  cat << 'EOF' > "$TEMP_GO"
package main
import (
	"encoding/json"
	"os"
)
func main() {
	file := os.Args[1]
	target := os.Args[2]
	data, err := os.ReadFile(file)
	var config map[string]interface{}
	if err == nil {
		json.Unmarshal(data, &config)
	}
	if config == nil {
		config = make(map[string]interface{})
	}
	config["statusLine"] = map[string]interface{}{
		"type": "",
		"command": target,
		"enabled": true,
	}
	newData, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(file, newData, 0644)
}
EOF
  go run "$TEMP_GO" "$SETTINGS_FILE" "$SCRIPT_TARGET"
  rm -f "$TEMP_GO"
else
  # Create directory if missing
  mkdir -p "$SETTINGS_DIR"
  # Write new config
  echo -e "Creating a new settings.json configuration..."
  cat <<EOF > "$SETTINGS_FILE"
{
  "statusLine": {
    "type": "",
    "command": "${SCRIPT_TARGET}",
    "enabled": true
  }
}
EOF
fi

echo -e "${BLUE}====================================================${RESET}"
echo -e "${GREEN}🎉 Installation completed successfully!${RESET}"
echo -e "Start a new antigravity session to see your new statusline."
echo -e "Uninstaller copied to: ${UNINSTALL_TARGET}"
echo -e "${BLUE}====================================================${RESET}"
