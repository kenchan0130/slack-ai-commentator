#!/bin/sh
set -eu

mode=${1:?use --check or --write}
root=${2:-.}
docker_go=$(sed -nE 's/^FROM([[:space:]]+--platform=[^[:space:]]+)?[[:space:]]+golang:([0-9]+\.[0-9]+)(\.[0-9]+)?[[:space:]].*$/\2/p' "$root/Dockerfile" | head -n 1)
module_go=$(sed -nE 's/^go ([0-9]+\.[0-9]+)(\.[0-9]+)?$/\1/p' "$root/go.mod")

if [ -z "$docker_go" ] || [ -z "$module_go" ]; then
  echo 'Could not find Go versions in Dockerfile and go.mod' >&2
  exit 1
fi
[ "$docker_go" = "$module_go" ] && exit 0

if [ "$mode" = --check ]; then
  echo "go.mod must declare go $docker_go to match Dockerfile" >&2
  exit 1
fi
[ "$mode" = --write ] || exit 2

docker_major=${docker_go%%.*}
docker_minor=${docker_go#*.}
module_major=${module_go%%.*}
module_minor=${module_go#*.}
if [ "$docker_major" -lt "$module_major" ] || {
  [ "$docker_major" -eq "$module_major" ] && [ "$docker_minor" -lt "$module_minor" ];
}; then
  echo 'Docker builder is older than go.mod; refusing to lower the required Go version' >&2
  exit 1
fi

temp_file=$(mktemp)
trap 'rm -f "$temp_file"' EXIT HUP INT TERM
sed -E "s/^go [0-9]+\.[0-9]+(\.[0-9]+)?$/go $docker_go/" "$root/go.mod" > "$temp_file"
cat "$temp_file" > "$root/go.mod"
echo "Updated go.mod to go $docker_go"
