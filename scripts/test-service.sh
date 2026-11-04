#!/bin/sh
# Runs enowxd as a systemd user service in an Ubuntu container booted with
# systemd, with docker: lingering set up as root (hostconn/linger.sh), the
# service started by `enowxd bridge`, then the daemon and a session back
# after a reboot without anyone connecting. Usage: scripts/test-service.sh
set -eu
cd "$(dirname "$0")/.."
case $(docker info --format '{{.Architecture}}') in
x86_64 | amd64) goarch=amd64 ;;
*) goarch=arm64 ;;
esac
mkdir -p "$HOME/.cache"
out=$(mktemp -d "$HOME/.cache/enowx-service.XXXXXX")
name=enowx-service-test
trap 'docker rm -f $name >/dev/null 2>&1; rm -rf "$out"' EXIT
go run ./scripts/terminalpatch -out "$out/go.mod"
export GOFLAGS="${GOFLAGS:-} -modfile=$out/go.mod"
CGO_ENABLED=0 GOOS=linux GOARCH=$goarch go build -o "$out/enowxd" ./cmd/enowxd
cp hostconn/linger.sh "$out/"
cat >"$out/Dockerfile" <<'DOCKERFILE'
FROM ubuntu:24.04
RUN apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq systemd systemd-sysv dbus dbus-user-session libpam-systemd git >/dev/null && useradd -m -s /bin/bash ada
CMD ["/sbin/init"]
DOCKERFILE
docker build -q -t $name "$out" >/dev/null
booted() {
	for _ in $(seq 1 60); do
		docker exec $name systemctl is-system-running 2>/dev/null | grep -qE 'running|degraded' && return 0
		sleep 1
	done
	echo "the container didn't boot" >&2
	return 1
}
docker run -d --name $name --privileged --cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:rw -v "$out:/enowx:ro" $name >/dev/null
booted
docker exec $name sh -c 'cp /enowx/enowxd /home/ada/enowxd && chown ada /home/ada/enowxd && set -- ada && . /enowx/linger.sh'
docker exec $name su - ada -c '
	set -e
	echo | ./enowxd bridge >/dev/null
	XDG_RUNTIME_DIR=/run/user/$(id -u) systemctl --user is-active --quiet enowxd
	mkdir -p p && git -C p init -q
	./enowxd spawn --agent shell --project ~/p --name kept >/dev/null'
docker restart $name >/dev/null
booted
docker exec $name su - ada -c 'sleep 2; ./enowxd ls' | grep -q kept
echo "ok: enowxd runs as a user service and comes back after a reboot"
