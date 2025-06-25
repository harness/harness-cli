#!/bin/bash
# Script to update all import paths from "harness/" to "github.com/harness/harness-cli/"

# Use find and grep to locate all files with "harness/" imports
find . -type f -name "*.go" -exec grep -l "\"harness/" {} \; | while read -r file; do
  echo "Processing $file"
  # Use sed to replace all imports from "harness/" to "github.com/harness/harness-cli/"
  sed -i '' 's|"harness/|"github.com/harness/harness-cli/|g' "$file"
done

echo "Import paths updated. Now run 'go mod tidy' to fix dependencies."
