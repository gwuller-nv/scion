#!/bin/bash

# Determine version
# Only set VERSION if we are exactly on a tag (semver-ish)
if git describe --tags --exact-match >/dev/null 2>&1; then
    VERSION=$(git describe --tags --exact-match)
else
    VERSION=""
fi

# Determine commit hash
COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown")

# Determine build time
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Package to inject variables into
PKG="github.com/ptone/scion-agent/pkg/version"

# Construct ldflags
LDFLAGS="-X ${PKG}.Commit=${COMMIT} -X ${PKG}.BuildTime=${BUILD_TIME}"

if [ -n "$VERSION" ]; then
    LDFLAGS="${LDFLAGS} -X ${PKG}.Version=${VERSION}"
fi

echo "${LDFLAGS}"
