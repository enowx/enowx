#!/bin/sh
# Builds enowX's installers on this machine, without publishing anything:
# the macOS app and disk image (signed ad hoc unless mygo.json names a
# Developer ID), the Linux .deb and tarball with install.sh, and the Windows
# setup, with SHA256SUMS. Usage: scripts/package.sh [platforms]
# (default darwin/universal,linux/amd64,linux/arm64,windows/amd64).
set -eu
cd "$(dirname "$0")/.."
platforms=${1:-darwin/universal,linux/amd64,linux/arm64,windows/amd64}
version=$(sed -n 's/.*"version": "\(.*\)".*/\1/p' cmd/enowx/mygo.json)
dist="$PWD/dist/$version"
# Disk images are made by mounting them, which macOS refuses on some
# external volumes: build under the home folder, then copy.
mkdir -p "$HOME/.cache"
work=$(mktemp -d "$HOME/.cache/enowx-package.XXXXXX")
trap 'rm -rf "$work"' EXIT
scripts/daemons.sh
sh scripts/terminalpatch.sh -- tool mygo build -o "$work" -platform "$platforms" cmd/enowx
mkdir -p "$dist"
find "$work" -maxdepth 2 -type f \( -name '*.dmg' -o -name '*.deb' -o -name '*.tar.gz' -o -name '*Setup*.exe' -o -name install.sh \) | while read -r f; do
	case "$f" in
	*/install.sh) cp "$f" "$dist/install-$(basename "$(dirname "$f")").sh" ;;
	*) cp "$f" "$dist/" ;;
	esac
done
sum="shasum -a 256"
command -v sha256sum >/dev/null && sum=sha256sum
(cd "$dist" && find . -maxdepth 1 -type f ! -name SHA256SUMS -exec $sum {} + | sed 's| \./| |' > SHA256SUMS)
echo "Installers in $dist (not published):"
ls -1 "$dist"
