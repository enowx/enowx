#!/usr/bin/env bash
# Builds release artifacts locally (no CI minutes): a macOS universal app,
# ad-hoc signed, and enowxd for Linux servers. Usage: scripts/release-local.sh 0.0.1-alpha.1
# Needs: Go 1.27+, zip, and rcodesign (cargo install apple-codesign) when not on macOS.
set -euo pipefail

version="${1:?usage: $0 <version>}"
root="$(cd "$(dirname "$0")/.." && pwd)"
dist="$root/dist"
mkdir -p "$dist"

cd "$root"
make daemons
patch=$(mktemp -d "${TMPDIR:-/tmp}/enowx-terminalpatch.XXXXXX")
trap 'rm -rf "$patch"' EXIT
flags=$(sh scripts/terminalpatch.sh -out "$patch/go.mod" -print-go-flags)
export GOFLAGS="$flags"
"${HOST_GO:-go}" tool mygo build -platform darwin/universal cmd/enowx

app="$root/build/darwin-universal/enowX.app"
if [[ "$(uname)" == "Darwin" ]]; then
  codesign --force --deep --sign - "$app"
else
  rcodesign sign "$app"
fi
(cd "$root/build/darwin-universal" && zip -qry "$dist/enowX-$version-macos-universal.zip" enowX.app)

for arch in amd64 arm64; do
  mkdir -p "$dist/linux-$arch"
  GOOS=linux GOARCH="$arch" CGO_ENABLED=0 "${HOST_GO:-go}" build -trimpath -ldflags "-s -w" -o "$dist/linux-$arch/enowxd" ./cmd/enowxd
  tar -C "$dist/linux-$arch" -czf "$dist/enowxd-$version-linux-$arch.tar.gz" enowxd
done

(cd "$dist" && sha256sum enowX-"$version"-*.zip enowxd-"$version"-*.tar.gz > SHA256SUMS)
echo "Artifacts in $dist"
