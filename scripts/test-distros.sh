#!/bin/sh
# Runs enowX's daemon and its setup's system packages step in a container
# of each Linux distribution it supports, with docker. Usage:
#   scripts/test-distros.sh [image ...]
set -eu
cd "$(dirname "$0")/.."
images=${*:-"ubuntu:24.04 debian:12 fedora:41 archlinux:latest alpine:3.21 opensuse/tumbleweed"}
arch=$(docker info --format '{{.Architecture}}')
case "$arch" in
x86_64 | amd64) goarch=amd64 ;;
aarch64 | arm64) goarch=arm64 ;;
*) echo "unknown docker architecture $arch" >&2; exit 1 ;;
esac
# Under the home folder: docker VMs (colima, Docker Desktop) share it.
mkdir -p "$HOME/.cache"
out=$(mktemp -d "$HOME/.cache/enowx-distros.XXXXXX")
trap 'rm -rf "$out"' EXIT
go run ./scripts/terminalpatch -out "$out/go.mod"
export GOFLAGS="${GOFLAGS:-} -modfile=$out/go.mod"
CGO_ENABLED=0 GOOS=linux GOARCH=$goarch go build -o "$out/enowxd" ./cmd/enowxd
CGO_ENABLED=0 GOOS=linux GOARCH=$goarch go test -c -o "$out/setup.test" ./core/setup
cp hostconn/prepare.sh "$out/"
# Images there are no native build of run emulated.
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$out/enowxd-amd64" ./cmd/enowxd
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -o "$out/setup-amd64.test" ./core/setup
failed=""
for image in $images; do
	platform="" suffix=""
	if [ "$image" = archlinux:latest ] && [ "$goarch" = arm64 ]; then
		platform="--platform=linux/amd64" suffix="-amd64" # Arch has no arm64 image
	fi
	echo "== $image $platform"
	# The daemon serves and answers its command line; then the setup step
	# installs what it needs, as root, and a second plan has nothing left.
	if ! docker run --rm $platform -v "$out:/enowx:ro" -e ENOWX_DISTRO_TEST=1 -e SUFFIX="$suffix" "$image" sh -c '
		set -e
		if [ -n "$SUFFIX" ] && [ -f /etc/pacman.conf ]; then
			# Emulated, pacman cannot sandbox its downloads (seccomp).
			sed -i "s/^\[options\]/[options]\nDisableSandbox/" /etc/pacman.conf
		fi
		sh /enowx/prepare.sh
		cp /enowx/enowxd$SUFFIX /tmp/enowxd && cp /enowx/setup$SUFFIX.test /tmp/setup.test
		export ENOWX_HOME=/tmp/enx HOME=/root SHELL=/bin/sh
		/tmp/enowxd serve >/tmp/serve.log 2>&1 &
		for i in $(seq 1 150); do /tmp/enowxd ls >/dev/null 2>&1 && break; sleep 0.2; done
		mkdir -p /tmp/p
		id=$(/tmp/enowxd spawn --agent shell --project /tmp/p "echo terminal-works")
		for i in $(seq 1 150); do /tmp/enowxd read "$id" | grep -q "^terminal-works" && break; sleep 0.2; done
		/tmp/enowxd read "$id" | grep -q "^terminal-works"
		/tmp/enowxd ls
		/tmp/enowxd stop
		rc=0; /tmp/setup.test -test.run TestSystemPackagesOnThisDistro -test.v >/tmp/test.log 2>&1 || rc=$?
		tail -5 /tmp/test.log
		exit $rc
	'; then
		failed="$failed $image"
	fi
done
if [ -n "$failed" ]; then
	echo "failed:$failed" >&2
	exit 1
fi
echo "all passed"
