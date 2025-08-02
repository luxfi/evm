#!/bin/bash
# Fix EVM internal imports to NOT use /v2

echo "Fixing EVM internal imports to remove /v2..."

# Find all Go files and fix internal imports
find . -name "*.go" -type f -not -path "./vendor/*" -not -path "./.git/*" | while read -r file; do
    # Remove /v2 from internal EVM imports
    sed -i '' 's|"github.com/luxfi/evm/v2/|"github.com/luxfi/evm/|g' "$file"
done

echo "EVM internal import fix complete!"