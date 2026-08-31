#!/usr/bin/env bash
# Serve the documentation locally with live reload.
set -euo pipefail
cd "$(dirname "$0")"
exec mkdocs serve -a 127.0.0.1:8765
