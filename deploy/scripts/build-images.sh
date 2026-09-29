#!/usr/bin/env bash
set -euo pipefail
if [[ $# -lt 2 || $# -gt 3 || ( $# -eq 3 && $3 != --push ) ]]; then
  echo "Usage: $0 REGISTRY_PATH TAG [--push]" >&2
  exit 2
fi
repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
registry=${1%/}
tag=$2
[[ -n "$registry" && -n "$tag" ]] || exit 2
output=--load
[[ ${3:-} != --push ]] || output=--push
for component in control agent postgres; do
  docker buildx build --platform "${PLATFORM:-linux/amd64}" "$output" \
    -f "$repo_dir/docker/$component.Dockerfile" \
    -t "$registry/$component:$tag" "$repo_dir"
done
