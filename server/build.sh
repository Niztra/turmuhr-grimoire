#!/bin/bash
# Baut den Turmuhr-Server für Linux und Windows. Die App (../index.html) und die Klänge werden eingebettet.
set -e
cd "$(dirname "$0")"
GO=${GO:-go}; command -v "$GO" >/dev/null || GO=~/.local/share/gotool/go/bin/go
rm -rf web && mkdir -p web dist
cp ../index.html ../manifest.webmanifest ../icon-192.png ../icon-512.png web/
cp -r ../audio web/audio
V=${VERSION:-$(date +%Y-%m-%d)}
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -trimpath -ldflags "-s -w -X main.version=$V" -o dist/turmuhr-server-linux .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$GO" build -trimpath -ldflags "-s -w -X main.version=$V" -o dist/turmuhr-server-windows.exe .
ls -lh dist
