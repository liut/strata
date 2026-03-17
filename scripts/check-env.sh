#!/usr/bin/env bash
#
# Strata 环境依赖检查脚本
# 运行此脚本确保目标机器满足所有运行时依赖

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "╔════════════════════════════════════════════════════════╗"
echo "║           Strata Environment Check                                         ║"
echo "╚════════════════════════════════════════════════════════╝"
echo

ERRORS=0

check() {
    local name="$1"
    local cmd="$2"
    local fix="$3"

    if eval "$cmd" &>/dev/null; then
        echo -e "${GREEN}✓${NC} $name"
    else
        echo -e "${RED}✗${NC} $name"
        if [ -n "$fix" ]; then
            echo -e "  ${YELLOW}→$fix${NC}"
        fi
        ((ERRORS++))
    fi
}

check_with_output() {
    local name="$1"
    local cmd="$2"
    local fix="$3"

    local out
    out=$(eval "$cmd" 2>&1) || true
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓${NC} $name"
    else
        echo -e "${RED}✗${NC} $name"
        echo "  $out"
        if [ -n "$fix" ]; then
            echo -e "  ${YELLOW}→$fix${NC}"
        fi
        ((ERRORS++))
    fi
}

echo "━━━ System Requirements ━━━"
check "bash ≥ 4.0" '[ ${BASH_VERSION:0:1} -ge 4 ]'
check "Linux kernel" '[[ "$(uname)" == "Linux" ]]'

echo
echo "━━━ Core Dependencies ━━━"
check "bubblewrap (bwrap)" "command -v bwrap" "apt install bubblewrap"
check "fusermount" "command -v fusermount" "apt install fuse"

echo
echo "━━━ Overlay Driver ━━━"
if command -v fuse-overlayfs &>/dev/null; then
    echo -e "${GREEN}✓${NC} fuse-overlayfs: $(fuse-overlayfs --version 2>&1 | head -1)"
else
    echo -e "${RED}✗${NC} fuse-overlayfs not found"
    echo -e "  ${YELLOW}→ apt install fuse-overlayfs${NC}"
    ((ERRORS++))
fi

# FUSE device
if [ -c /dev/fuse ]; then
    echo -e "${GREEN}✓${NC} /dev/fuse exists"
else
    echo -e "${RED}✗${NC} /dev/fuse not available"
    echo -e "  ${YELLOW}→ modprobe fuse (requires root)${NC}"
    ((ERRORS++))
fi

echo
echo "━━━ User Namespace (Optional) ━━━"
if [ "$(cat /proc/sys/kernel/unprivileged_userns_clone 2>/dev/null)" = "1" ]; then
    echo -e "${GREEN}✓${NC} unprivileged user namespace enabled"
else
    echo -e "${YELLOW}⚠${NC} unprivileged user namespace disabled (some features may not work)"
    echo -e "  ${YELLOW}→ sysctl -w kernel.unprivileged_userns_clone=1 (requires root)${NC}"
fi

# subuid/subgid
if grep -q "^$(whoami):" /etc/subuid 2>/dev/null; then
    echo -e "${GREEN}✓${NC} /etc/subuid configured for $(whoami)"
else
    echo -e "${YELLOW}⚠${NC} /etc/subuid not configured (may be needed for user namespaces)"
    echo -e "  ${YELLOW}→ usermod --add-subuids 100000-165535 $(whoami)${NC}"
fi

echo
echo "━━━ Network Tools ━━━"
check "iproute2 (ip command)" "command -v ip"
check "util-linux (unshare)" "command -v unshare"

echo
echo "━━━ Shell Availability ━━━"
check "bash" "command -v bash"
check "sh (dash/bash)" "command -v sh"

echo
echo "━━━ Kernel Features ━━━"
check "CONFIG_USER_NS (user namespaces)" "grep -q 'CONFIG_USER_NS=y' /boot/config-$(uname -r) 2>/dev/null || [ -e /proc/sys/kernel/unprivileged_userns_clone ]"

echo
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $ERRORS -eq 0 ]; then
    echo -e "${GREEN}All checks passed! Strata should run fine.${NC}"
    exit 0
else
    echo -e "${RED}$ERRORS check(s) failed.${NC}"
    echo "Please fix the above issues before running Strata."
    exit 1
fi
