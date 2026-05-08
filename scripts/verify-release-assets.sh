#!/usr/bin/env bash
set -euo pipefail

release_dir="${1:-dist/release}"

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
