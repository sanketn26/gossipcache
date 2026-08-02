#!/usr/bin/env bash
# Generate Go gRPC stubs from api/proto into api/gen.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO_ROOT="${ROOT}/api/proto"
OUT_DIR="${ROOT}/api/gen"
PATH="$(go env GOPATH)/bin:${PATH}"
export PATH

if ! command -v protoc >/dev/null 2>&1; then
  echo "protoc not found; install protocolbuffers/protobuf" >&2
  exit 1
fi
if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "protoc-gen-go not found; run: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest" >&2
  exit 1
fi
if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then
  echo "protoc-gen-go-grpc not found; run: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest" >&2
  exit 1
fi

rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"

protoc \
  -I "${PROTO_ROOT}" \
  --go_out="${OUT_DIR}" \
  --go_opt=paths=source_relative \
  --go-grpc_out="${OUT_DIR}" \
  --go-grpc_opt=paths=source_relative \
  "${PROTO_ROOT}/gossipcache/v1/"*.proto

echo "generated stubs under ${OUT_DIR}"
