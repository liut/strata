#!/bin/bash
# test-grpc.sh - 测试 gRPC API
#
# 用法: ./test-grpc.sh [host:port]
#
# 示例:
#   ./test-grpc.sh                    # 默认 localhost:2280
#   ./test-grpc.sh saturn.hyyl.xyz:8080

HOST="${1:-localhost:2280}"
USER="testuser"
SESSION="shell-test-$$"
PROTO="${STRATA_PROTO:-./pkg/proto/sandbox/sandbox.proto}"

echo "=== gRPC Test ==="
echo "Server: $HOST"
echo "User: $USER"
echo "Session: $SESSION"
echo ""

if ! command -v grpcurl &> /dev/null; then
    echo "Error: grpcurl not found"
    echo "Install: go install github.com/fullstorydev/grpcurl/...@latest"
    exit 1
fi

echo "--- Test 1: Create Session ---"
grpcurl -plaintext -proto "$PROTO" -d "{\"ownerID\":\"$USER\",\"sessionID\":\"${SESSION}-1\"}" \
    $HOST sandbox.SandboxService/CreateSession

echo ""
echo "--- Test 2: Exec Command ---"
grpcurl -plaintext -proto "$PROTO" -d "{\"ownerID\":\"$USER\",\"sessionID\":\"${SESSION}-1\",\"command\":\"echo hello from grpc\"}" \
    $HOST sandbox.SandboxService/Exec

echo ""
echo "--- Test 3: Stats ---"
grpcurl -plaintext -proto "$PROTO" -d "{}" $HOST sandbox.SandboxService/Stats

echo ""
echo "--- Test 4: Close Session ---"
grpcurl -plaintext -proto "$PROTO" -d "{\"ownerID\":\"$USER\",\"sessionID\":\"${SESSION}-1\"}" \
    $HOST sandbox.SandboxService/CloseSession

echo ""
echo "=== Done ==="
