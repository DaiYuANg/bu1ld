#!/usr/bin/env bash
set -euo pipefail

release_dir="${1:-dist/release}"
version="${GITHUB_REF_NAME:-}"
version="${version#v}"

if [[ ! -d "$release_dir" ]]; then
  echo "release directory does not exist: $release_dir" >&2
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

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

extract_tar() {
  local asset="$1"
  local target="$2"
  mkdir -p "$target"
  tar -xzf "$asset" -C "$target"
}

main_dir="$work/bu1ld"
go_plugin_asset="$release_dir/bu1ld-go-plugin_${version}_linux_amd64.tar.gz"
java_plugin_asset="$release_dir/bu1ld-java-plugin_${version}_linux_amd64.tar.gz"
extract_tar "$release_dir/bu1ld_${version}_linux_amd64.tar.gz" "$main_dir"

bu1ld="$main_dir/bu1ld"
if [[ ! -x "$bu1ld" ]]; then
  echo "release bu1ld binary is missing or not executable: $bu1ld" >&2
  exit 1
fi

registry="$work/registry"
mkdir -p "$registry/plugins" "$registry/assets"
cp "$go_plugin_asset" "$registry/assets/"
cp "$java_plugin_asset" "$registry/assets/"
cat > "$registry/plugins.toml" <<EOF_REGISTRY
version = 1

[[plugins]]
id = "org.bu1ld.go"
file = "plugins/org.bu1ld.go.toml"

[[plugins]]
id = "org.bu1ld.java"
file = "plugins/org.bu1ld.java.toml"
EOF_REGISTRY

cat > "$registry/plugins/org.bu1ld.go.toml" <<EOF_GO
id = "org.bu1ld.go"
namespace = "go"

[[versions]]
version = "$version"
status = "approved"
manifest = "plugin.toml"

[[versions.assets]]
os = "linux"
arch = "amd64"
url = "../assets/$(basename "$go_plugin_asset")"
format = "tar.gz"
EOF_GO

cat > "$registry/plugins/org.bu1ld.java.toml" <<EOF_JAVA
id = "org.bu1ld.java"
namespace = "java"

[[versions]]
version = "$version"
status = "approved"
manifest = "plugin.toml"

[[versions.assets]]
os = "linux"
arch = "amd64"
url = "../assets/$(basename "$java_plugin_asset")"
format = "tar.gz"
EOF_JAVA

copy_example() {
  local name="$1"
  local target="$work/$name"
  cp -R "examples/$name" "$target"
  find "$target" -name build.bu1ld -exec sed -i.bak '/^[[:space:]]*path[[:space:]]*=/d' {} +
  find "$target" -name '*.bak' -delete
  echo "$target"
}

install_plugin() {
  local project="$1"
  local plugin="$2"
  BU1LD_PLUGIN_REGISTRY="$registry" "$bu1ld" --project-dir "$project" plugins install "$plugin@$version" --force
}

go_project="$(copy_example go-project)"
install_plugin "$go_project" org.bu1ld.go
"$bu1ld" --project-dir "$go_project" --no-cache build

java_project="$(copy_example java-project)"
install_plugin "$java_project" org.bu1ld.java
"$bu1ld" --project-dir "$java_project" --no-cache build

monorepo="$(copy_example multilang-monorepo)"
install_plugin "$monorepo" org.bu1ld.go
install_plugin "$monorepo" org.bu1ld.java
"$bu1ld" --project-dir "$monorepo" --no-cache build
