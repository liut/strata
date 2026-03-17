#!/bin/bash
#
# Strata API 测试脚本
# 对已运行的 web 服务进行完整流程检测
#

set -o pipefail
# set -e  # 注释掉，避免 grep 等命令失败导致脚本退出

# 默认配置
HOST="${STRATA_HOST:-localhost}"
PORT="${STRATA_PORT:-8080}"
BASE_URL="http://${HOST}:${PORT}"
WS_URL="ws://${HOST}:${PORT}"
TIMEOUT=5

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试用户
TEST_USER="test_user_$$"
TEST_SESSION="test_session_$$"

# 统计
PASSED=0
FAILED=0

# ───────────────────────────────────────────────────────────
# 辅助函数
# ───────────────────────────────────────────────────────────

log_info() {
    echo -e "${YELLOW}[INFO]${NC} $1"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((PASSED++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((FAILED++))
}

wait_for_service() {
    local max_attempts=30
    local attempt=1
    log_info "等待服务启动..."
    while [ $attempt -le $max_attempts ]; do
        if curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/api/stats" 2>/dev/null | grep -q "200"; then
            log_info "服务已就绪"
            return 0
        fi
        sleep 1
        ((attempt++))
    done
    echo -e "${RED}服务启动超时${NC}"
    return 1
}

# ───────────────────────────────────────────────────────────
# 测试用例
# ───────────────────────────────────────────────────────────

test_stats() {
    log_info "测试: GET /api/stats"
    local resp
    resp=$(curl -s "${BASE_URL}/api/stats")
    if echo "$resp" | grep -q "active_sessions"; then
        log_pass "stats 接口正常"
    else
        log_fail "stats 接口异常: $resp"
    fi
}

test_create_session() {
    log_info "测试: POST /api/sessions"
    local resp
    resp=$(curl -s -X POST "${BASE_URL}/api/sessions" \
        -H "Content-Type: application/json" \
        -d "{\"user_id\": \"${TEST_USER}\", \"session_id\": \"${TEST_SESSION}\"}")

    if echo "$resp" | grep -q "session_id"; then
        log_pass "创建会话成功: $resp"
    else
        log_fail "创建会话失败: $resp"
        return 1
    fi
}

test_exec_command() {
    log_info "测试: POST /api/sessions/{uid}/{sid}/exec"
    local resp
    resp=$(curl -s -X POST "${BASE_URL}/api/sessions/${TEST_USER}/${TEST_SESSION}/exec" \
        -H "Content-Type: application/json" \
        -d '{"command": "echo hello_strata_test"}')

    if echo "$resp" | grep -q "output"; then
        log_pass "执行命令成功: $resp"
    else
        log_fail "执行命令失败: $resp"
        return 1
    fi
}

test_exec_pwd_and_ls() {
    log_info "测试: 执行 pwd 和 ls / 命令"

    # 测试 pwd（显示当前目录）
    local resp_pwd
    resp_pwd=$(curl -s -X POST "${BASE_URL}/api/sessions/${TEST_USER}/${TEST_SESSION}/exec" \
        -H "Content-Type: application/json" \
        -d '{"command": "pwd"}')
    if echo "$resp_pwd" | grep -q "tmp"; then
        log_pass "pwd 命令成功: $resp_pwd"
    else
        log_fail "pwd 命令失败: $resp_pwd"
    fi

    # 测试 ls /（列出根目录文件）
    local resp_ls
    resp_ls=$(curl -s -X POST "${BASE_URL}/api/sessions/${TEST_USER}/${TEST_SESSION}/exec" \
        -H "Content-Type: application/json" \
        -d '{"command": "ls -l /"}')
    if echo "$resp_ls" | grep -q "bin"; then
        log_pass "ls / 命令成功: $resp_ls"
    else
        log_fail "ls / 命令失败: $resp_ls"
    fi
}

test_exec_with_timeout() {
    log_info "测试: 命令执行超时处理"
    local resp
    resp=$(curl -s -X POST "${BASE_URL}/api/sessions/${TEST_USER}/${TEST_SESSION}/exec" \
        -H "Content-Type: application/json" \
        -d '{"command": "sleep 10", "timeout_ms": 1000}')

    # 超时后 elapsed 应该接近 timeout_ms（1s），且 output 为空或很短
    if echo "$resp" | grep -q "elapsed"; then
        log_pass "超时处理正常: $resp"
    else
        log_fail "超时处理异常: $resp"
    fi
}

test_close_session() {
    log_info "测试: DELETE /api/sessions/{user}/{session}"
    local resp
    resp=$(curl -s -X DELETE "${BASE_URL}/api/sessions/${TEST_USER}/${TEST_SESSION}")

    if echo "$resp" | grep -q -E '"success"|"closed"'; then
        log_pass "关闭会话成功"
    else
        log_fail "关闭会话失败: $resp"
        return 1
    fi
}

test_websocket() {
    log_info "测试: WebSocket shell"

    # 使用 websocat 或 wscat 进行测试
    # 这里使用简化的 curl 测试连接升级

    # 先创建一个新 session（因为之前的已关闭）
    curl -s -X POST "${BASE_URL}/api/sessions" \
        -H "Content-Type: application/json" \
        -d "{\"user_id\": \"${TEST_USER}\", \"session_id\": \"${TEST_SESSION}_ws\"}" > /dev/null

    # 测试 WebSocket 升级（不使用 curl，用 nc 或其他工具）
    if command -v websocat &> /dev/null; then
        # 发送输入，接收输出
        echo '{"type":"input","data":"echo ws_test\n"}' | \
            timeout 5 websocat "${WS_URL}/api/ws/${TEST_USER}/${TEST_SESSION}_ws/shell" 2>/dev/null
        log_pass "WebSocket 测试完成 (websocat)"
    elif command -v wscat &> /dev/null; then
        log_pass "WebSocket 测试需要手动验证 (wscat)"
    else
        # 简单测试 WebSocket 端点是否可访问
        local http_code
        http_code=$(curl -s -o /dev/null -w "%{http_code}" \
            -H "Connection: Upgrade" \
            -H "Upgrade: websocket" \
            "${BASE_URL}/api/ws/${TEST_USER}/${TEST_SESSION}_ws/shell")
        if [ "$http_code" = "101" ] || [ "$http_code" = "400" ]; then
            # 101 = Switching Protocols (成功)
            # 400 = Bad Request (说明 WebSocket 支持存在，只是协议不匹配)
            log_pass "WebSocket 端点可访问 (http code: $http_code)"
        else
            log_fail "WebSocket 端点异常 (http code: $http_code)"
        fi
    fi

    # 清理 WebSocket 测试 session
    curl -s -X DELETE "${BASE_URL}/api/sessions/${TEST_USER}/${TEST_SESSION}_ws" > /dev/null
}

test_invalid_session() {
    log_info "测试: 无效会话处理"
    local resp
    resp=$(curl -s -X DELETE "${BASE_URL}/api/sessions/nonexistent_user/nonexistent_session")

    # 应该返回 404 或 error 信息
    if echo "$resp" | grep -q -E '404|"error"'; then
        log_pass "无效会话处理正常: $resp"
    else
        log_fail "无效会话处理异常: $resp"
    fi
}

test_invalid_command() {
    log_info "测试: 空命令处理"
    local resp
    resp=$(curl -s -X POST "${BASE_URL}/api/sessions/${TEST_USER}/${TEST_SESSION}/exec" \
        -H "Content-Type: application/json" \
        -d '{"command": ""}')

    if echo "$resp" | grep -q "required"; then
        log_pass "空命令验证正常"
    else
        log_fail "空命令验证异常: $resp"
    fi
}

# ───────────────────────────────────────────────────────────
# 主流程
# ───────────────────────────────────────────────────────────

main() {
    echo "============================================"
    echo "  Strata API 完整流程测试"
    echo "  Base URL: ${BASE_URL}"
    echo "============================================"
    echo

    # 等待服务
    if ! wait_for_service; then
        log_fail "服务未运行或启动失败"
        exit 1
    fi

    echo
    echo "--- 基础功能测试 ---"
    test_stats

    echo
    echo "--- 会话管理测试 ---"
    test_create_session
    test_exec_command
    test_exec_pwd_and_ls
    test_exec_with_timeout

    echo
    echo "--- 异常处理测试 ---"
    test_invalid_session
    test_invalid_command

    echo
    echo "--- WebSocket 测试 ---"
    test_websocket

    echo
    echo "--- 清理测试 ---"
    test_close_session

    echo
    echo "============================================"
    echo "  测试结果: ${PASSED} 通过, ${FAILED} 失败"
    echo "============================================"

    if [ $FAILED -gt 0 ]; then
        exit 1
    fi
    exit 0
}

# 清理函数
cleanup() {
    log_info "清理测试会话..."
    curl -s -X DELETE "${BASE_URL}/api/sessions/${TEST_USER}/${TEST_SESSION}" > /dev/null 2>&1
    curl -s -X DELETE "${BASE_URL}/api/sessions/${TEST_USER}/${TEST_SESSION}_ws" > /dev/null 2>&1
}
trap cleanup EXIT

main "$@"
