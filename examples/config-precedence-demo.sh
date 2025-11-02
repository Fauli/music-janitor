#!/bin/bash
# Demo: Configuration Precedence in MLC
# Shows how flags override environment variables and config files

set -e

echo "=== MLC Configuration Precedence Demo ==="
echo ""

# Create test directories
mkdir -p demo-config/music
touch demo-config/music/test.mp3

# Create a test config file
cat > demo-config/test.yaml <<EOF
source: "/default/config/path"
concurrency: 4
mode: copy
EOF

echo "1. Config file only (lowest priority)"
echo "   Config says: source=/default/config/path, concurrency=4"
echo ""
./build/mlc scan --config demo-config/test.yaml -s demo-config/music --db demo-config/test.db 2>&1 | grep -E "Source:|Concurrency:" || true
echo ""

# Clean up for next test
rm -f demo-config/test.db*

echo "2. Environment variable overrides config file"
echo "   Config says: concurrency=4"
echo "   Env says: MLC_CONCURRENCY=12"
echo ""
MLC_CONCURRENCY=12 ./build/mlc scan --config demo-config/test.yaml -s demo-config/music --db demo-config/test.db 2>&1 | grep -E "Concurrency:" || true
echo ""

# Clean up for next test
rm -f demo-config/test.db*

echo "3. Command-line flag overrides both (highest priority)"
echo "   Config says: concurrency=4"
echo "   Env says: MLC_CONCURRENCY=12"
echo "   Flag says: -c 16"
echo ""
MLC_CONCURRENCY=12 ./build/mlc scan --config demo-config/test.yaml -s demo-config/music --db demo-config/test.db -c 16 2>&1 | grep -E "Concurrency:" || true
echo ""

# Clean up
rm -rf demo-config

echo "=== Demo Complete ==="
echo ""
echo "Precedence order (lowest to highest):"
echo "  1. Config file (YAML)"
echo "  2. Environment variables (MLC_*)"
echo "  3. Command-line flags (--flag)"
