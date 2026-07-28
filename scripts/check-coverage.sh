#!/usr/bin/env bash
set -euo pipefail

GO_BIN="${GO:-go}"
overall_profile="$(mktemp)"
production_profile="$(mktemp)"
trap 'rm -f "$overall_profile" "$production_profile" "${pkg_profiles[@]:-}"' EXIT

"$GO_BIN" test -covermode=atomic -coverprofile="$overall_profile" ./...
awk 'NR == 1 || $1 !~ /\/internal\/testutil\//' "$overall_profile" >"$production_profile"
overall="$("$GO_BIN" tool cover -func="$production_profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
awk -v pct="$overall" 'BEGIN { exit !(pct >= 70) }'

pkg_profiles=()
for pkg in internal/policy internal/workspace internal/scanner internal/execx; do
	pkg_profile="$(mktemp)"
	pkg_profiles+=("$pkg_profile")
	"$GO_BIN" test -coverprofile="$pkg_profile" "./$pkg"
	pkg_coverage="$("$GO_BIN" tool cover -func="$pkg_profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
	awk -v pkg="$pkg" -v pct="$pkg_coverage" 'BEGIN { exit !(pct >= 80) }'
done
