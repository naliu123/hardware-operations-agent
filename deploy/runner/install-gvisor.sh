#!/usr/bin/env bash
set -euo pipefail
# Run only on the dedicated Linux runner host, after Docker and systemd exist.
# The entire multi-file release is required by runsc releases after July 2026.
test "$(uname -s)" = Linux
test "$(id -u)" = 0
command -v docker >/dev/null
command -v systemctl >/dev/null
command -v bzip2 >/dev/null || { apt-get update; apt-get install -y bzip2; }
release=20260928.0
arch="$(uname -m)"
case "$arch" in
  aarch64) sha256=b7e11d27cbd69370ed7addb6c0d1c33e70e57a153cee57b5fdc5344bf303c7eb ;;
  x86_64) sha256=f3ed9131bc252259df150e270154180188f9df56b73a2312940325e1f522a2d6 ;;
  *) exit 1 ;;
esac
download="$(mktemp -d)"
trap 'rm -rf "$download"' EXIT
cd "$download"
base="https://storage.googleapis.com/gvisor/releases/release/$release/$arch"
curl -fL --retry 3 "$base/gvisor.tar.bz2" -o gvisor.tar.bz2
curl -fL --retry 3 "$base/gvisor.tar.bz2.sha512" -o gvisor.tar.bz2.sha512
echo "$sha256  gvisor.tar.bz2" | sha256sum -c -
sha512sum -c gvisor.tar.bz2.sha512
tar -xjf gvisor.tar.bz2 -C /usr/local/bin
/usr/local/bin/runsc install --runtime=hwops-runsc -- --network=none --platform=systrap
systemctl restart docker
/usr/local/bin/runsc --version
