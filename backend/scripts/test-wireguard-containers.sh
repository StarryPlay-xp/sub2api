#!/usr/bin/env bash
# Validates Linux userspace compatibility, not separate host kernels.
# Remote checks are opt-in. Never pass key material as a command argument.
set -euo pipefail
cd "$(dirname "$0")/.."
work=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-wg-check.XXXXXX")
container="sub2api-wg-check-$$"
cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  rm -f "$work/wg.test"
  rmdir "$work" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
arch=${SUB2API_WG_TEST_ARCH:-$(go env GOHOSTARCH)}
case "$arch" in amd64|arm64) ;; *) printf 'Unsupported test architecture: %s\n' "$arch" >&2; exit 2 ;; esac
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go test -c -o "$work/wg.test" ./internal/pkg/wgtunnel
mounts=(--mount "type=bind,src=$work/wg.test,dst=/wg.test,readonly")
if [[ -n "${SUB2API_WG_TEST_GODEBUG:-}" ]]; then
  mounts+=(--env "GODEBUG=$SUB2API_WG_TEST_GODEBUG")
fi
if [[ -n "${SUB2API_WG_TEST_CONFIG:-}" ]]; then
  : "${SUB2API_WG_TEST_CA_FILE:?remote container checks require a PEM CA bundle path}"
  mounts+=(--mount "type=bind,src=$SUB2API_WG_TEST_CONFIG,dst=/wg.conf,readonly"
    --env SUB2API_WG_TEST_CONFIG=/wg.conf
    --mount "type=bind,src=$SUB2API_WG_TEST_CA_FILE,dst=/ca.crt,readonly"
    --env SSL_CERT_FILE=/ca.crt)
  if [[ -n "${SUB2API_WG_TEST_EXPECTED_EXIT:-}" ]]; then
    mounts+=(--mount "type=bind,src=$SUB2API_WG_TEST_EXPECTED_EXIT,dst=/expected-exit,readonly"
      --env SUB2API_WG_TEST_EXPECTED_EXIT=/expected-exit)
  fi
fi
for image in ubuntu:22.04 ubuntu:24.04 debian:13-slim; do
  printf 'Testing %s (%s) without elevated capabilities\n' "$image" "$arch"
  docker run --rm --name "$container" --label sub2api.task=wireguard \
    --platform "linux/$arch" --user "$(id -u):$(id -g)" \
    --cap-drop ALL --security-opt no-new-privileges --read-only \
    --tmpfs /tmp:rw,noexec,nosuid,size=16m,mode=1777 \
    --pids-limit 256 --memory 512m "${mounts[@]}" "$image" /wg.test -test.v
 done
