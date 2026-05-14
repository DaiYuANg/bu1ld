#!/usr/bin/env bash
set -euo pipefail

release_dir="${1:-dist/release}"
version="${GITHUB_REF_NAME:-}"
version="${version#v}"

if [[ ! -d "$release_dir" ]]; then
  echo "release directory does not exist: $release_dir" >&2
  exit 1
fi

found=0
if [[ -f "$release_dir/checksums.txt" ]]; then
  found=1
  (cd "$release_dir" && sha256sum -c checksums.txt)
fi

shopt -s nullglob
for checksum_file in "$release_dir"/*.sha256; do
  found=1
  (cd "$release_dir" && sha256sum -c "$(basename "$checksum_file")")
done

if [[ "$found" -eq 0 ]]; then
  echo "no checksums.txt or *.sha256 files found in $release_dir" >&2
  exit 1
fi

if [[ -z "$version" ]]; then
  first_asset="$(find "$release_dir" -maxdepth 1 -type f -name 'bu1ld_*_linux_amd64.tar.gz' | head -n 1)"
  first_asset="$(basename "$first_asset")"
  version="${first_asset#bu1ld_}"
  version="${version%_linux_amd64.tar.gz}"
fi

if [[ -z "$version" ]]; then
  echo "unable to determine release version" >&2
  exit 1
fi

manifest_value() {
  local manifest="$1"
  local key="$2"
  sed -nE "s/^[[:space:]]*${key}[[:space:]]*=[[:space:]]*\"([^\"]*)\"[[:space:]]*$/\1/p" "$manifest" | head -n 1
}

extract_asset() {
  local asset="$1"
  local target="$2"
  mkdir -p "$target"
  case "$asset" in
    *.tar.gz)
      tar -xzf "$asset" -C "$target"
      ;;
    *.zip)
      unzip -q "$asset" -d "$target"
      ;;
    *)
      echo "unsupported release asset format: $asset" >&2
      exit 1
      ;;
  esac
}

require_file() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    echo "required file is missing: $path" >&2
    exit 1
  fi
}

require_executable_or_windows_file() {
  local path="$1"
  local goos="$2"
  if [[ "$goos" == "windows" ]]; then
    require_file "$path"
    return
  fi
  if [[ ! -x "$path" ]]; then
    echo "required executable is missing or not executable: $path" >&2
    exit 1
  fi
}

validate_binary_archive() {
  local asset="$1"
  local goos="$2"
  local tmp
  tmp="$(mktemp -d)"
  extract_asset "$asset" "$tmp"
  for binary in bu1ld bu1ld-server bu1ld-daemon bu1ld-lsp; do
    local path="$tmp/$binary"
    if [[ "$goos" == "windows" ]]; then
      path="$path.exe"
    fi
    require_executable_or_windows_file "$path" "$goos"
  done
  rm -rf "$tmp"
}

validate_plugin_archive() {
  local asset="$1"
  local plugin_id="$2"
  local namespace="$3"
  local goos="$4"
  local tmp
  tmp="$(mktemp -d)"
  extract_asset "$asset" "$tmp"

  local manifest="$tmp/plugin.toml"
  require_file "$manifest"

  local got_id got_namespace got_version binary binary_path
  got_id="$(manifest_value "$manifest" id)"
  got_namespace="$(manifest_value "$manifest" namespace)"
  got_version="$(manifest_value "$manifest" version)"
  binary="$(manifest_value "$manifest" binary)"

  if [[ "$got_id" != "$plugin_id" ]]; then
    echo "$asset manifest id = $got_id, want $plugin_id" >&2
    exit 1
  fi
  if [[ "$got_namespace" != "$namespace" ]]; then
    echo "$asset manifest namespace = $got_namespace, want $namespace" >&2
    exit 1
  fi
  if [[ "$got_version" != "$version" ]]; then
    echo "$asset manifest version = $got_version, want $version" >&2
    exit 1
  fi
  if [[ -z "$binary" ]]; then
    echo "$asset manifest binary is empty" >&2
    exit 1
  fi

  binary_path="$tmp/$binary"
  if [[ "$goos" == "windows" && ! -f "$binary_path" && "$binary" != *.exe ]]; then
    binary_path="$binary_path.exe"
  fi
  require_executable_or_windows_file "$binary_path" "$goos"
  rm -rf "$tmp"
}

validate_expected_asset() {
  local asset="$1"
  require_file "$asset"
}

for goos in linux darwin; do
  for goarch in amd64 arm64; do
    asset="$release_dir/bu1ld_${version}_${goos}_${goarch}.tar.gz"
    validate_expected_asset "$asset"
    validate_binary_archive "$asset" "$goos"

    asset="$release_dir/bu1ld-go-plugin_${version}_${goos}_${goarch}.tar.gz"
    validate_expected_asset "$asset"
    validate_plugin_archive "$asset" "org.bu1ld.go" "go" "$goos"
  done
done

asset="$release_dir/bu1ld_${version}_windows_amd64.zip"
validate_expected_asset "$asset"
validate_binary_archive "$asset" "windows"

asset="$release_dir/bu1ld-go-plugin_${version}_windows_amd64.zip"
validate_expected_asset "$asset"
validate_plugin_archive "$asset" "org.bu1ld.go" "go" "windows"

asset="$release_dir/bu1ld-node-plugin_${version}.tar.gz"
validate_expected_asset "$asset"
validate_plugin_archive "$asset" "org.bu1ld.node" "node" "linux"

for target in "linux amd64 tar.gz" "darwin amd64 tar.gz" "darwin arm64 tar.gz" "windows amd64 zip"; do
  read -r goos goarch extension <<<"$target"
  asset="$release_dir/bu1ld-java-plugin_${version}_${goos}_${goarch}.${extension}"
  validate_expected_asset "$asset"
  validate_plugin_archive "$asset" "org.bu1ld.java" "java" "$goos"
done
