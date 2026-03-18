#!/bin/bash
# test-grpc-shell.sh - 测试 gRPC Shell 双向流
#
# 用法: ./test-grpc-shell.sh [host:port]
#
# 示例:
#   ./test-grpc-shell.sh                    # 默认 localhost:2280
#   ./test-grpc-shell.sh 192.168.1.100:2280

HOST="${1:-localhost:2280}"
USER="testuser"
SESSION="shell-test-$$"

echo "=== gRPC Shell Test ==="
echo "Server: $HOST"
echo "User: $USER"
echo "Session: $SESSION"
echo ""

# 使用 grpcurl 测试（需要先安装: go install github.com/fullstorydev/grpcurl/...@latest）
# 或者使用 grpcc (go install github.com/jhump/protoreflect/cmd/grpcc@latest)

if ! command -v grpcurl &> /dev/null; then
    echo "Error: grpcurl not found"
    echo "Install: go install github.com/fullstorydev/grpcurl/...@latest"
    exit 1
fi

echo "--- Test 1: Create Session ---"
grpcurl -plaintext -d "{\"user_id\":\"$USER\",\"session_id\":\"${SESSION}-1\"}" \
    $HOST sandbox.SandboxService/CreateSession

echo ""
echo "--- Test 2: Exec Command ---"
grpcurl -plaintext -d "{\"user_id\":\"$USER\",\"session_id\":\"${SESSION}-1\",\"command\":\"echo hello from grpc\"}" \
    $HOST sandbox.SandboxService/Exec

echo ""
echo "--- Test 3: Stats ---"
grpcurl -plaintext -d "{}" $HOST sandbox.SandboxService/Stats

echo ""
echo "--- Test 4: Close Session ---"
grpcurl -plaintext -d "{\"user_id\":\"$USER\",\"session_id\":\"${SESSION}-1\"}" \
    $HOST sandbox.SandboxService/CloseSession

echo ""
echo "=== Done ==="
