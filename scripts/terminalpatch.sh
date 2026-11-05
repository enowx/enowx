#!/bin/sh
# Compile the preparer for this host without changing its child's target.
set -eu
go=${HOST_GO:-go}
hostos=$("$go" env GOHOSTOS)
hostarch=$("$go" env GOHOSTARCH)
work=$(mktemp -d "${TMPDIR:-/tmp}/enowx-terminal-bootstrap.XXXXXX")
trap 'rm -rf "$work"' EXIT
GOOS=$hostos GOARCH=$hostarch "$go" build -o "$work/terminalpatch.exe" ./scripts/terminalpatch
"$work/terminalpatch.exe" -go "$go" "$@"
