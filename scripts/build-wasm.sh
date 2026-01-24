#!/bin/bash

# AcousticDNA WASM Build Script
# Compiles the Go WASM module and copies required runtime files

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🔨 Building AcousticDNA WASM module...${NC}"
echo ""

# Get Go root directory
GOROOT=$(go env GOROOT)
if [ -z "$GOROOT" ]; then
    echo -e "${RED}❌ Error: Could not find Go installation${NC}"
    exit 1
fi

# Create output directories
echo -e "${YELLOW}📁 Creating output directories...${NC}"
mkdir -p web/public
mkdir -p web/src/api

# Build WASM binary
echo -e "${YELLOW}🔧 Compiling WASM binary...${NC}"
GOOS=js GOARCH=wasm go build \
    -ldflags="-s -w" \
    -o web/public/fingerprint.wasm \
    cmd/wasm/main.go

if [ $? -ne 0 ]; then
    echo -e "${RED}❌ WASM build failed${NC}"
    exit 1
fi

# Copy wasm_exec.js from Go SDK
echo -e "${YELLOW}📋 Copying WASM runtime (wasm_exec.js)...${NC}"

# Try multiple possible locations for wasm_exec.js
WASM_EXEC_SRC=""
POSSIBLE_PATHS=(
    "$GOROOT/misc/wasm/wasm_exec.js"
    "$GOROOT/lib/wasm/wasm_exec.js"
    "/usr/lib/go/misc/wasm/wasm_exec.js"
    "/usr/local/go/misc/wasm/wasm_exec.js"
)

for path in "${POSSIBLE_PATHS[@]}"; do
    if [ -f "$path" ]; then
        WASM_EXEC_SRC="$path"
        break
    fi
done

if [ -z "$WASM_EXEC_SRC" ] || [ ! -f "$WASM_EXEC_SRC" ]; then
    # Try to find it dynamically
    WASM_EXEC_SRC=$(find "$GOROOT" -name "wasm_exec.js" 2>/dev/null | head -1)
fi

if [ -z "$WASM_EXEC_SRC" ] || [ ! -f "$WASM_EXEC_SRC" ]; then
    echo -e "${RED}❌ Error: Could not find wasm_exec.js${NC}"
    echo -e "${YELLOW}   Searched in:${NC}"
    for path in "${POSSIBLE_PATHS[@]}"; do
        echo -e "${YELLOW}   - $path${NC}"
    done
    echo -e "${YELLOW}   Make sure you have Go installed correctly${NC}"
    exit 1
fi

cp "$WASM_EXEC_SRC" web/public/wasm_exec.js
echo -e "${GREEN}   Found at: $WASM_EXEC_SRC${NC}"

# Display build information
WASM_SIZE=$(du -h web/public/fingerprint.wasm | cut -f1)
WASM_SIZE_BYTES=$(stat -f%z web/public/fingerprint.wasm 2>/dev/null || stat -c%s web/public/fingerprint.wasm 2>/dev/null)

echo ""
echo -e "${GREEN}✅ Build completed successfully!${NC}"
echo ""
echo -e "${BLUE}📊 Build Information:${NC}"
echo -e "   WASM Binary: ${GREEN}web/public/fingerprint.wasm${NC}"
echo -e "   Size:        ${YELLOW}${WASM_SIZE}${NC} (${WASM_SIZE_BYTES} bytes)"
echo -e "   Runtime:     ${GREEN}web/public/wasm_exec.js${NC}"
echo ""

# Size warnings
if [ "$WASM_SIZE_BYTES" -gt 10485760 ]; then  # > 10 MB
    echo -e "${RED}⚠️  Warning: WASM binary is very large (>10 MB)${NC}"
    echo -e "${YELLOW}   Consider optimizing build flags or reducing dependencies${NC}"
elif [ "$WASM_SIZE_BYTES" -gt 5242880 ]; then  # > 5 MB
    echo -e "${YELLOW}⚠️  Warning: WASM binary is larger than recommended (>5 MB)${NC}"
    echo -e "${YELLOW}   This may slow down initial page load${NC}"
fi

# Optional: Compress with gzip to show potential network transfer size
if command -v gzip &> /dev/null; then
    cp web/public/fingerprint.wasm web/public/fingerprint.wasm.tmp
    gzip -9 web/public/fingerprint.wasm.tmp
    GZIP_SIZE=$(du -h web/public/fingerprint.wasm.tmp.gz | cut -f1)
    rm web/public/fingerprint.wasm.tmp.gz
    echo -e "${BLUE}   Gzipped:     ${GREEN}~${GZIP_SIZE}${NC} (estimated network transfer)"
fi

echo ""
echo -e "${GREEN}🚀 Next Steps:${NC}"
echo -e "   1. Serve the web directory:"
echo -e "      ${YELLOW}cd web && npx serve public${NC}"
echo -e "      ${YELLOW}# or: cd web/public && python3 -m http.server 8000${NC}"
echo ""
echo -e "   2. Open browser:"
echo -e "      ${YELLOW}http://localhost:8000${NC}"
echo ""
echo -e "   3. Test in browser console:"
echo -e "      ${YELLOW}generateFingerprint([0.1, 0.2, ...], 44100, 1)${NC}"
echo ""
