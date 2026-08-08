#!/usr/bin/env bash
#
# push-url-ab-test.sh — MANUAL A/B/C regression for P2b (explicit-credential push
# builds wrong package root). See docs/hc-open-issues-fix-plan.md §3 and
# docs/push-url-derivation.md.
#
# THIS SCRIPT IS RUN MANUALLY — it is not wired into CI. It pushes a disposable
# NuGet package to a DEV registry three times:
#
#   Leg A: explicit --account/--api-url/--token, NO --pkg-url
#          -> must SUCCEED (PkgURL derived from --api-url via /system/info).
#             Before the fix this failed with 404 "Root not found: <account>".
#   Leg B: explicit credentials PLUS --pkg-url
#          -> must succeed (flag still supported; wins over derivation).
#   Leg C: saved config only (hc auth login), no explicit flags, no --pkg-url
#          -> must succeed (unchanged behavior).
#
# Each leg uses a unique package version so re-runs never collide, and asserts
# on the CLI exit code plus the "Successfully uploaded" output line.
#
# Prereqs: bash, zip, and a built hc binary (go build -o hc ./cmd/hc).
#
# Usage:
#   HC_ACCOUNT_ID=<acct> HC_TOKEN=<token> HC_REGISTRY=<registry> \
#       ./scripts/push-url-ab-test.sh
#
# Optional overrides:
#   HC_BIN      path to the hc binary            (default: ./hc, fallback: hc on PATH)
#   HC_API_URL  DEV API base URL                 (required — e.g. https://<your-dev>.harness.io)
#   HC_PKG_URL  DEV package host for leg B       (default: same host as HC_API_URL)
#
# NOTE on isolation: legs A/B run with HOME pointed at a throwaway directory so a
# stale ~/.harness/auth.json cannot mask the explicit-credential flow (derivation
# only kicks in when no package URL is configured). Leg C uses your real HOME and
# requires a prior 'hc auth login' against the SAME DEV environment.

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration (placeholders — override via environment)
# ---------------------------------------------------------------------------
HC_BIN="${HC_BIN:-./hc}"
ACCOUNT_ID="${HC_ACCOUNT_ID:-<your-dev-account-id>}"
API_URL="${HC_API_URL:-<your-dev-api-url>}"
TOKEN="${HC_TOKEN:-<your-dev-api-token>}"
PKG_URL="${HC_PKG_URL:-${API_URL}}"
REGISTRY="${HC_REGISTRY:-<your-test-registry>}"

if [[ "$ACCOUNT_ID" == "<your-dev-account-id>" || "$TOKEN" == "<your-dev-api-token>" || "$REGISTRY" == "<your-test-registry>" || "$API_URL" == "<your-dev-api-url>" ]]; then
	echo "ERROR: set HC_ACCOUNT_ID, HC_API_URL, HC_TOKEN and HC_REGISTRY (see script header)." >&2
	exit 2
fi
if [[ ! -x "$HC_BIN" ]]; then
	if command -v hc >/dev/null 2>&1; then
		HC_BIN=hc
	else
		echo "ERROR: hc binary not found at '$HC_BIN' and not on PATH." >&2
		exit 2
	fi
fi

# ---------------------------------------------------------------------------
# Build a disposable NuGet package (.nupkg is a zip with a .nuspec)
# ---------------------------------------------------------------------------
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

PKG_ID="hc-p2b-probe"
STAMP="$(date +%Y%m%d%H%M%S)"

build_nupkg() {
	local version="$1" out="$2"
	local dir="$WORKDIR/pkg-$version"
	mkdir -p "$dir"
	cat >"$dir/$PKG_ID.nuspec" <<XML
<?xml version="1.0"?>
<package xmlns="http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd">
  <metadata>
    <id>$PKG_ID</id>
    <version>$version</version>
    <authors>hc-p2b-regression</authors>
    <description>Disposable probe package for the hc P2b A/B/C regression (safe to delete)</description>
  </metadata>
</package>
XML
	echo "probe $version" >"$dir/probe.txt"
	(cd "$dir" && zip -q -r "$out" .)
}

# ---------------------------------------------------------------------------
# Leg runner
# ---------------------------------------------------------------------------
RESULTS=()

run_leg() {
	local name="$1"; shift
	local version="$1"; shift
	local home_dir="$1"; shift
	local nupkg="$WORKDIR/$PKG_ID.$version.nupkg"
	build_nupkg "$version" "$nupkg"

	echo "==================================================================="
	echo "Leg $name — pushing $PKG_ID $version to registry '$REGISTRY'"
	echo "==================================================================="
	local output rc
	set +e
	output="$(HOME="$home_dir" "$HC_BIN" artifact push nuget "$REGISTRY" "$nupkg" "$@" 2>&1)"
	rc=$?
	set -e
	echo "$output"

	if [[ $rc -eq 0 && "$output" == *"Successfully uploaded"* ]]; then
		echo "Leg $name: PASS"
		RESULTS+=("PASS")
	else
		echo "Leg $name: FAIL (exit $rc)"
		RESULTS+=("FAIL")
	fi
	echo
}

# Throwaway HOME for the explicit-credential legs (no saved auth.json).
FAKE_HOME="$WORKDIR/home"
mkdir -p "$FAKE_HOME"

run_leg "A (explicit creds, no --pkg-url — must succeed via derivation)" \
	"0.0.$STAMP-a" "$FAKE_HOME" \
	--account "$ACCOUNT_ID" --api-url "$API_URL" --token "$TOKEN"

run_leg "B (explicit creds + --pkg-url)" \
	"0.0.$STAMP-b" "$FAKE_HOME" \
	--account "$ACCOUNT_ID" --api-url "$API_URL" --token "$TOKEN" \
	--pkg-url "$PKG_URL"

if [[ -f "$HOME/.harness/auth.json" ]]; then
	run_leg "C (saved config only)" \
		"0.0.$STAMP-c" "$HOME"
else
	echo "Leg C: SKIP — ~/.harness/auth.json not found; run 'hc auth login' against DEV first."
	RESULTS+=("SKIP")
	echo
fi

echo "==================================================================="
echo "SUMMARY:  A=${RESULTS[0]}  B=${RESULTS[1]}  C=${RESULTS[2]}"
echo "Expected: A=PASS B=PASS C=PASS (or C=SKIP when not logged in)"
echo "==================================================================="
[[ "${RESULTS[0]}" == "PASS" && "${RESULTS[1]}" == "PASS" && "${RESULTS[2]}" != "FAIL" ]]
