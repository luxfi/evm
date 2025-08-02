#!/bin/bash
# Fix EVM imports to use node/v2 properly

echo "Fixing EVM imports to use node/v2..."

# Find all Go files and update imports
find . -name "*.go" -type f -not -path "./vendor/*" -not -path "./.git/*" | while read -r file; do
    # Update node imports to v2
    sed -i '' 's|"github.com/luxfi/node/|"github.com/luxfi/node/v2/|g' "$file"
done

echo "EVM import fix complete!"