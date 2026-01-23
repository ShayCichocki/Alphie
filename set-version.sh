#!/bin/bash
# Update version across all VERSION files and rebuild

if [ -z "$1" ]; then
    echo "Usage: ./set-version.sh <version>"
    echo "Example: ./set-version.sh 0.1.1-ux-fix"
    exit 1
fi

VERSION="$1"

echo "Setting version to: $VERSION"
echo "$VERSION" > cmd/alphie/VERSION
echo "$VERSION" > internal/version/VERSION

echo "Rebuilding binary..."
go build -o alphie cmd/alphie/*.go

echo "Done! Verify with: ./alphie --version"
./alphie --version
