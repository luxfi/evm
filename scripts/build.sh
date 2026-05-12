#!/usr/bin/env bash
#
# Build the Lux EVM plugin.
#
#   ./scripts/build.sh                     # → build/evm
#   ./scripts/build.sh /custom/path        # → /custom/path
#   INSTALL=1 ./scripts/build.sh           # also installs to $LUX_PLUGIN_DIR/<vmid>
#
# The VM ID for the install path is derived at runtime from luxfi/constants.EVMID
# via cmd/vmid — no base58 strings are hardcoded in shell.

set -o errexit
set -o nounset
set -o pipefail

EVM_PATH=$(cd "$(dirname "${BASH_SOURCE[0]}")"; cd .. && pwd)
source "$EVM_PATH"/scripts/constants.sh

BINARY_PATH="${1:-$EVM_PATH/build/evm}"

echo "Building EVM @ GitCommit: $EVM_COMMIT at $BINARY_PATH"
mkdir -p "$(dirname "$BINARY_PATH")"
go build \
    -ldflags "-X github.com/luxfi/evm/plugin/evm.GitCommit=$EVM_COMMIT $STATIC_LD_FLAGS" \
    -o "$BINARY_PATH" \
    "$EVM_PATH"/plugin/*.go

if [[ "$(uname -s)" == "Darwin" ]]; then
    codesign --force --sign - "$BINARY_PATH" >/dev/null 2>&1 || true
fi

if [[ "${INSTALL:-}" == "1" ]]; then
    mkdir -p "$LUX_PLUGIN_DIR"
    install -m 0755 "$BINARY_PATH" "$LUX_PLUGIN_DIR/$EVM_VMID"
    if [[ "$(uname -s)" == "Darwin" ]]; then
        codesign --force --sign - "$LUX_PLUGIN_DIR/$EVM_VMID" >/dev/null 2>&1 || true
    fi
    echo "Installed $LUX_PLUGIN_DIR/$EVM_VMID"
fi
