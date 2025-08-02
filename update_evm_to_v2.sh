#!/bin/bash
# Update all imports from github.com/luxfi/evm/ to github.com/luxfi/evm/v2/

echo "Updating EVM imports to v2..."

# Find all Go files and update imports
find . -name "*.go" -type f -not -path "./vendor/*" -not -path "./.git/*" | while read -r file; do
    # Update imports from github.com/luxfi/evm/ to github.com/luxfi/evm/v2/
    sed -i '' 's|"github.com/luxfi/evm/|"github.com/luxfi/evm/v2/|g' "$file"
done

echo "EVM import update complete!"