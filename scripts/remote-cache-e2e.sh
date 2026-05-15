#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d)"
cleanup() {
  if [[ -n "${server_pid:-}" ]]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" >/dev/null 2>&1 || true
  fi
  rm -f "$root/plugins/go/bu1ld-go-plugin"
  rm -rf "$work"
}
trap cleanup EXIT

bin="$work/bin"
mkdir -p "$bin"

(cd "$root" && go build -o "$bin/bu1ld" ./cmd/bu1ld)
(cd "$root" && go build -o "$bin/bu1ld-server" ./cmd/server)
go build -C "$root/plugins/go" -o bu1ld-go-plugin ./cmd/bu1ld-go-plugin

server_cache="$work/server-cache"
server_project="$work/server-project"
mkdir -p "$server_project"
listen="127.0.0.1:19876"
remote_url="http://$listen"
"$bin/bu1ld-server" --project-dir "$server_project" --cache-dir "$server_cache" coordinator --listen "$listen" >"$work/server.log" 2>&1 &
server_pid="$!"

health_key="0000000000000000000000000000000000000000000000000000000000000000"
ready=0
for _ in $(seq 1 100); do
  status="$(curl -sS -o /dev/null -w '%{http_code}' "$remote_url/v1/actions/$health_key" || true)"
  if [[ "$status" == "404" ]]; then
    ready=1
    break
  fi
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    cat "$work/server.log" >&2
    echo "remote cache server exited before it became ready" >&2
    exit 1
  fi
  sleep 0.1
done
if [[ "$ready" -ne 1 ]]; then
  cat "$work/server.log" >&2
  echo "remote cache server did not become ready" >&2
  exit 1
fi

project="$work/go-project"
cp -R "$root/examples/go-project" "$project"
go_manifest="$(printf '%s\n' "$root/plugins/go/plugin.toml" | sed 's/[\/&]/\\&/g')"
sed -i.bak "s|../../plugins/go/plugin.toml|$go_manifest|" "$project/build.bu1ld"
rm -f "$project/build.bu1ld.bak"

run_build() {
  local cache_dir="$1"
  local log_path="$2"
  BU1LD_REMOTE_CACHE__URL="$remote_url" \
  BU1LD_REMOTE_CACHE__PULL=true \
  BU1LD_REMOTE_CACHE__PUSH=true \
  BU1LD_GO_CACHEPROG_LOG="$log_path" \
  BU1LD_GO__CACHEPROG="$root/plugins/go/bu1ld-go-plugin cacheprog --remote-cache-url $remote_url --remote-cache-pull=true --remote-cache-push=true --cache-dir $cache_dir" \
    "$bin/bu1ld" --project-dir "$project" --no-cache build
}

run_build "$work/local-cache-1" "$work/cacheprog-1.log"

if [[ ! -d "$server_cache/go/actions" ]]; then
  cat "$work/cacheprog-1.log" >&2 || true
  echo "remote cache did not receive Go cache actions" >&2
  exit 1
fi

rm -rf "$project/dist" "$project/build"
run_build "$work/local-cache-2" "$work/cacheprog-2.log"

if ! grep -q "remote_hit" "$work/cacheprog-2.log"; then
  cat "$work/cacheprog-1.log" >&2 || true
  cat "$work/cacheprog-2.log" >&2 || true
  echo "second build did not report a remote Go cache hit" >&2
  exit 1
fi
