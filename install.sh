#!/usr/bin/env bash

# Aether Installer for Linux / macOS / Unix
set -e

# ANSI escape codes for beautiful prints
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

echo -e "${BOLD}${BLUE}=====================================================${NC}"
echo -e "${BOLD}${BLUE}            🌀 Aether Installer Utility 🌀           ${NC}"
echo -e "${BOLD}${BLUE}=====================================================${NC}"

# 1. Compile the binary if not present
if [ ! -f "aether" ]; then
    echo -e "${CYAN}ℹ️  'aether' binary not found. Compiling with Go...${NC}"
    if command -v go &> /dev/null; then
        go build -o aether
        echo -e "${GREEN}✅ Compilation successful!${NC}"
    else
        echo -e "${RED}❌ 'go' compiler not found on system. Please compile the 'aether' binary first.${NC}"
        exit 1
    fi
fi

# 2. Setup user local binary folder
BIN_DIR="$HOME/.local/bin"
echo -e "${CYAN}📁 Creating local binary directory at '${BIN_DIR}'...${NC}"
mkdir -p "${BIN_DIR}"

echo -e "${CYAN}🚀 Installing 'aether' executable...${NC}"
cp aether "${BIN_DIR}/aether"
chmod +x "${BIN_DIR}/aether"
echo -e "${GREEN}✅ Executable installed successfully!${NC}"

# 3. Setup configuration folder
CONFIG_DIR="$HOME/.config/aether"
echo -e "${CYAN}📁 Creating config folder at '${CONFIG_DIR}'...${NC}"
mkdir -p "${CONFIG_DIR}"

if [ -f "config.json" ]; then
    echo -e "${CYAN}⚙️  Copying config.json configuration file...${NC}"
    if [ -f "${CONFIG_DIR}/config.json" ]; then
        echo -e "${CYAN}⚠️  Configuration config.json already exists in target. Backing up...${NC}"
        cp "${CONFIG_DIR}/config.json" "${CONFIG_DIR}/config.json.bak"
    fi
    cp config.json "${CONFIG_DIR}/config.json"
    echo -e "${GREEN}✅ Config file copied successfully!${NC}"
else
    echo -e "${CYAN}⚙️  No config.json found in current directory. Creating a default config...${NC}"
    cat <<EOF > "${CONFIG_DIR}/config.json"
{
  "surreal_url": "ws://localhost:8000",
  "surreal_user": "root",
  "surreal_pass": "root",
  "surreal_ns": "sts",
  "surreal_db": "test",
  "embedding_provider": "gemini",
  "embedding_model": "gemini-embedding-2",
  "embedding_dim": 768
}
EOF
    echo -e "${GREEN}✅ Default config file created at ${CONFIG_DIR}/config.json${NC}"
fi

# 4. Check Path environment variable
PATH_INCLUDED=false
case ":$PATH:" in
    *:"$BIN_DIR":*) PATH_INCLUDED=true ;;
esac

echo -e "${BOLD}${BLUE}=====================================================${NC}"
echo -e "${BOLD}${GREEN}🎉 Aether has been successfully installed!${NC}"
echo -e "${BOLD}${BLUE}=====================================================${NC}"
echo -e "Binary Path:      ${BOLD}${BIN_DIR}/aether${NC}"
echo -e "Config Path:      ${BOLD}${CONFIG_DIR}/config.json${NC}"
echo -e "${BOLD}${BLUE}=====================================================${NC}"

if [ "$PATH_INCLUDED" = false ]; then
    echo -e "${BOLD}${RED}⚠️  Warning: ${BIN_DIR} is NOT currently in your PATH.${NC}"
    echo -e "To run 'aether' from any directory, add this line to your shell config file (e.g. ~/.bashrc or ~/.zshrc):"
    echo -e "  ${BOLD}export PATH=\"\$HOME/.local/bin:\$PATH\"${NC}"
    echo -e "Then reload your shell: ${BOLD}source ~/.bashrc${NC} (or config of choice)."
else
    echo -e "${GREEN}✨ Excellent: ~/.local/bin is already in your PATH!${NC}"
    echo -e "You can now run ${BOLD}aether seed${NC} or ${BOLD}aether query \"my query\"${NC} globally."
fi
echo -e "${BOLD}${BLUE}=====================================================${NC}"
