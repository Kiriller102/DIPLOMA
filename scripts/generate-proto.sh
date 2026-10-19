#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
docker run --rm -v "$PWD:/work" -w /work golang:1.25-alpine sh -ec '
  apk add --no-cache protobuf protobuf-dev curl >/dev/null
  GOBIN=/tmp/proto-tools go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.5
  GOBIN=/tmp/proto-tools go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
  mkdir -p /tmp/proto-includes/google/rpc
  curl -fsSL https://raw.githubusercontent.com/googleapis/googleapis/2a8bb824f639c5e3b9567ea95a4e905d313fa6cf/google/rpc/status.proto -o /tmp/proto-includes/google/rpc/status.proto
  for proto in services/Database_Service/api/grpc/proto/*.proto; do
    name="${proto##*/}"
    name="${name%.proto}"
    PATH="/tmp/proto-tools:$PATH" protoc \
      -I services/Database_Service/api/grpc/proto -I /tmp/proto-includes -I /usr/include \
      --go_out="services/Database_Service/api/grpc/pb/$name" --go_opt=paths=source_relative \
      --go-grpc_out="services/Database_Service/api/grpc/pb/$name" --go-grpc_opt=paths=source_relative "$proto"
  done
'
