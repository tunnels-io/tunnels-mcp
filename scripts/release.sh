#!/usr/bin/env bash
# Build the release artefacts for tunnels-mcp.
#
#   scripts/release.sh 0.0.2
#
# Writes dist/:
#   tunnels-mcp_<v>_<os>_<arch>.tar.gz (.zip on Windows)  six plain binaries
#   tunnels-mcp-<v>.mcpb                                  one bundle for MCPB clients
#   server.json                                           the MCP Registry entry, with the bundle's SHA-256
#   SHA256SUMS
#
# Needs macOS. The bundle's macOS binary is a universal build made with lipo,
# because MCPB selects a binary by OS only, never by CPU.

set -euo pipefail

cd "$(dirname "$0")/.."

V="${1:-}"
V="${V#v}"
if [[ ! "$V" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
    echo "usage: scripts/release.sh <semver>   e.g. 0.0.2" >&2
    exit 2
fi
command -v lipo >/dev/null || { echo "lipo not found: run this on macOS" >&2; exit 1; }
command -v zip  >/dev/null || { echo "zip not found" >&2; exit 1; }

REPO="https://github.com/tunnels-io/tunnels-mcp"
PKG="github.com/tunnels-io/tunnels-mcp"
BIN="tunnels-mcp"
DIST="dist"
LDFLAGS="-s -w -X ${PKG}/internal/tunnels.version=${V}"

rm -rf "$DIST"
mkdir -p "$DIST/build" "$DIST/bundle/server"

build() {
    local os="$1" arch="$2" out="$3"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
        go build -trimpath -ldflags "$LDFLAGS" -o "$out" "./cmd/$BIN"
}

# ---- plain binaries, one archive per target
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
    os="${target%/*}"; arch="${target#*/}"
    exe="$BIN"; [ "$os" = windows ] && exe="$BIN.exe"
    dir="$DIST/build/${os}_${arch}"
    mkdir -p "$dir"
    build "$os" "$arch" "$dir/$exe"
    cp README.md LICENSE "$dir/"
    name="${BIN}_${V}_${os}_${arch}"
    if [ "$os" = windows ]; then
        (cd "$dir" && zip -q -X "../../$name.zip" "$exe" README.md LICENSE)
    else
        tar -C "$dir" -czf "$DIST/$name.tar.gz" "$exe" README.md LICENSE
    fi
    echo "built $name"
done

# ---- the MCPB bundle
# macOS: one universal binary covers Apple Silicon and Intel.
# Windows: amd64. Windows on Arm runs it under emulation.
# Linux: amd64. No MCPB client runs on Linux today; arm64 users take the tarball.
lipo -create -output "$DIST/bundle/server/$BIN" \
    "$DIST/build/darwin_amd64/$BIN" "$DIST/build/darwin_arm64/$BIN"
cp "$DIST/build/windows_amd64/$BIN.exe" "$DIST/bundle/server/$BIN.exe"
cp "$DIST/build/linux_amd64/$BIN" "$DIST/bundle/server/$BIN-linux"
cp LICENSE "$DIST/bundle/"

cat > "$DIST/bundle/manifest.json" <<EOF
{
  "manifest_version": "0.3",
  "name": "$BIN",
  "display_name": "tunnels.io",
  "version": "$V",
  "description": "Read a tunnels.io account and put local ports online.",
  "long_description": "Lists tunnels, usage, metrics, tokens, invoices, domains, notifications and cloud apps, and opens or closes a tunnel through the tunnels CLI. Only the tools the token was granted are listed. Nothing here changes a plan, buys a domain or deploys anything.",
  "author": { "name": "Maziar Sojoudian", "url": "https://tunnels.io" },
  "homepage": "https://tunnels.io",
  "repository": { "type": "git", "url": "$REPO" },
  "support": "$REPO/issues",
  "license": "MIT",
  "keywords": ["tunnels", "tunnel", "ngrok", "localhost", "port forwarding"],
  "privacy_policies": ["https://tunnels.io/privacy"],
  "server": {
    "type": "binary",
    "entry_point": "server/$BIN",
    "mcp_config": {
      "command": "\${__dirname}/server/$BIN",
      "args": [],
      "env": { "TUNNELS_TOKEN": "\${user_config.token}" },
      "platform_overrides": {
        "win32": { "command": "\${__dirname}/server/$BIN.exe" },
        "linux": { "command": "\${__dirname}/server/$BIN-linux" }
      }
    }
  },
  "tools": [
    { "name": "list_tunnels", "description": "Tunnels running right now, with their public URLs and local addresses." },
    { "name": "get_usage", "description": "Bandwidth, requests and quota left for the current billing period." },
    { "name": "get_metrics", "description": "Live traffic right now." },
    { "name": "get_account", "description": "Email, name and current plan." },
    { "name": "list_tokens", "description": "CLI tokens by name, prefix and permissions. Never the secrets." },
    { "name": "list_invoices", "description": "Invoices with amounts, dates and payment status." },
    { "name": "list_domains", "description": "Domains bought through tunnels.io, with status and renewal dates." },
    { "name": "list_notifications", "description": "In-app notifications, newest first." },
    { "name": "list_cloud_apps", "description": "Serverless apps, with status and hostnames." },
    { "name": "start_tunnel", "description": "Expose a local port and return the public URL. Needs the tunnels CLI." },
    { "name": "stop_tunnel", "description": "Close a tunnel opened by start_tunnel." }
  ],
  "tools_generated": false,
  "user_config": {
    "token": {
      "type": "string",
      "title": "tunnels.io token",
      "description": "Create one at tunnels.io/account/tokens under \"Connect an AI assistant\". It starts tnl_.",
      "sensitive": true,
      "required": true
    }
  },
  "compatibility": { "platforms": ["darwin", "win32", "linux"] }
}
EOF

BUNDLE="$BIN-$V.mcpb"
(cd "$DIST/bundle" && zip -q -X -r "../$BUNDLE" manifest.json LICENSE server)
SHA="$(shasum -a 256 "$DIST/$BUNDLE" | cut -d' ' -f1)"
echo "built $BUNDLE ($SHA)"

# ---- the MCP Registry entry
# Everything static comes from the committed server.json. Only the version, the
# download URL and the bundle hash are release-specific.
python3 - "$V" "$REPO/releases/download/v$V/$BUNDLE" "$SHA" <<'PY'
import json, sys
v, url, sha = sys.argv[1:4]
with open("server.json") as f:
    s = json.load(f)
s["version"] = v
pkg = s["packages"][0]
pkg["identifier"] = url
pkg["version"] = v
pkg["fileSha256"] = sha
with open("dist/server.json", "w") as f:
    json.dump(s, f, indent=2)
    f.write("\n")
PY

(cd "$DIST" && shasum -a 256 ./*.tar.gz ./*.zip ./*.mcpb | sed 's| \./| |' > SHA256SUMS)
rm -rf "$DIST/build" "$DIST/bundle"
echo "dist/ ready for v$V"
