#!/bin/bash
set -e

echo "Setting up CI environment..."

# Display Go version
echo "Go version:"
go version

# Only genuinely-private modules bypass the proxy: everything else in luxfi is
# public, and GOPRIVATE would disable checksum verification for it.
# shellcheck disable=SC2125 # The asterisk is intentional Go module syntax, not a glob
export GOPRIVATE='github.com/lux-private/*'
export GONOSUMDB='github.com/lux-private/*'

# Configure git to use GITHUB_TOKEN for authentication
if [ -n "$GITHUB_TOKEN" ]; then
    echo "Configuring git credentials..."
    git config --global url."https://x-access-token:${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"
fi

# Download dependencies from published packages
echo "Downloading dependencies..."
go mod download

echo "CI environment setup complete."
