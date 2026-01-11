#!/bin/bash
# Script to create a clean Python venv for CI Failure Summarizer
#
# Usage:
#   ./setup_venv.sh
#
# After setup, activate with:
#   source venv/bin/activate

set -euo pipefail  # Exit on error, undefined vars, and pipe failures

cd "$(dirname "$0")"

# Clean up AppImage pollution from environment (for Cursor IDE users)
unset APPIMAGE 2>/dev/null || true
unset APPDIR 2>/dev/null || true
unset LD_LIBRARY_PATH 2>/dev/null || true
unset PERLLIB 2>/dev/null || true
unset QT_PLUGIN_PATH 2>/dev/null || true
unset GSETTINGS_SCHEMA_DIR 2>/dev/null || true

# Reset PATH to standard system directories only
export PATH=/usr/local/bin:/usr/local/sbin:/usr/bin:/usr/sbin

# Add user bin directories if they exist
[ -d "$HOME/.local/bin" ] && export PATH="$PATH:$HOME/.local/bin"
[ -d "$HOME/bin" ] && export PATH="$PATH:$HOME/bin"

# Optionally unset XDG_DATA_DIRS if it contains AppImage paths
if [[ "${XDG_DATA_DIRS:-}" == *"/tmp/.mount_"* ]]; then
    unset XDG_DATA_DIRS
fi

# Remove old venv if it exists
rm -rf venv

# Create fresh venv with real Python
echo "Creating virtual environment..."
/usr/bin/python3 -m venv venv

# Install dependencies
echo "Installing dependencies..."
venv/bin/pip install --upgrade pip
venv/bin/pip install -r requirements.txt

echo ""
echo "✅ Virtual environment created successfully!"
echo ""
echo "Python: $(venv/bin/python --version)"
echo "Location: $(pwd)/venv"
echo ""
echo "To activate:"
echo "  source $(pwd)/venv/bin/activate"
echo ""
echo "To test locally:"
echo "  export GROQ_API_KEY='your-key'"
echo "  export PROW_URL='https://prow.ci.openshift.org/view/gs/...'"
echo "  python -m workflows.ci_failure_summarizer.summarize"
echo ""
