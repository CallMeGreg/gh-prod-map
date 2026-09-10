#!/usr/bin/env bash
set -euo pipefail

release_tag="${1:?usage: script/build-release.sh <release-tag>}"
repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="$repo_root/dist"
build_parent="$repo_root/build"

default_platforms=(
  darwin-amd64
  darwin-arm64
  freebsd-386
  freebsd-amd64
  freebsd-arm64
  linux-386
  linux-amd64
  linux-arm
  linux-arm64
  windows-386
  windows-amd64
  windows-arm64
)

if [[ -n "${GH_PROD_MAP_BUILD_PLATFORMS:-}" ]]; then
  read -r -a platforms <<<"$GH_PROD_MAP_BUILD_PLATFORMS"
else
  platforms=("${default_platforms[@]}")
fi

supportsEmbeddedRuntime() {
  case "$1/$2" in
    darwin/amd64 | darwin/arm64 | linux/amd64 | linux/arm64 | windows/amd64 | windows/arm64)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

if [[ -e "$dist_dir" && ! -d "$dist_dir" ]]; then
  echo "error: $dist_dir exists and is not a directory" >&2
  exit 1
fi

if [[ -e "$build_parent" && ! -d "$build_parent" ]]; then
  echo "error: $build_parent exists and is not a directory" >&2
  exit 1
fi

rm -rf -- "$dist_dir"
mkdir -p "$dist_dir"

build_parent_created=false
if [[ ! -d "$build_parent" ]]; then
  mkdir -p "$build_parent"
  build_parent_created=true
fi

build_dir="$(mktemp -d "$build_parent/copilot-release.XXXXXX")"
build_dir_name="$(basename "$build_dir")"
cleanup() {
  rm -rf -- "$build_dir"
  if [[ "$build_parent_created" == true ]]; then
    rmdir "$build_parent" 2>/dev/null || true
  fi
}
trap cleanup EXIT

supported_platforms="$(go tool dist list)"

echo "Building gh-prod-map $release_tag"
for platform in "${platforms[@]}"; do
  goos="${platform%-*}"
  goarch="${platform#*-}"

  if ! grep -Fxq "$goos/$goarch" <<<"$supported_platforms"; then
    echo "warning: skipping unsupported Go platform $platform" >&2
    continue
  fi

  extension=""
  if [[ "$goos" == "windows" ]]; then
    extension=".exe"
  fi

  package_path="."
  if supportsEmbeddedRuntime "$goos" "$goarch"; then
    bundle_dir="$build_dir/$platform"
    bundle_path="./build/$build_dir_name/$platform"
    mkdir -p "$bundle_dir"
    cp "$repo_root/main.go" "$bundle_dir/main.go"

    echo "Bundling Copilot runtime for $goos/$goarch"
    (
      cd "$repo_root"
      go tool bundler --platform "$goos/$goarch" --output "$bundle_dir"
    )
    package_path="$bundle_path"
  else
    echo "warning: building $platform without an embedded Copilot runtime; --ai requires COPILOT_CLI_PATH" >&2
  fi

  echo "Compiling $platform"
  (
    cd "$repo_root"
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
      go build -trimpath -ldflags="-s -w" -o "$dist_dir/$platform$extension" "$package_path"
  )

  if supportsEmbeddedRuntime "$goos" "$goarch"; then
    rm -rf -- "$bundle_dir"
  fi
done
