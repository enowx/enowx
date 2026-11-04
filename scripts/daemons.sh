#!/bin/sh
# Builds the Linux enowxd the app embeds, to install on servers.
set -eu
cd "$(dirname "$0")/.."
# Bootstrap on this host before applying target GOOS/GOARCH to the builds.
go=${HOST_GO:-go}
patch=$(mktemp -d "${TMPDIR:-/tmp}/enowx-terminalpatch.XXXXXX")
trap 'rm -rf "$patch"' EXIT
flags=$(sh scripts/terminalpatch.sh -out "$patch/go.mod" -print-go-flags)
export GOFLAGS="$flags"
for arch in amd64 arm64; do
	GOOS=linux GOARCH=$arch CGO_ENABLED=0 "$go" build -trimpath -ldflags "-s -w" -o hostconn/daemons/enowxd-linux-$arch ./cmd/enowxd
	gzip -9f hostconn/daemons/enowxd-linux-$arch
done
