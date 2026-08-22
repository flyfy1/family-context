#!/bin/zsh
set -euo pipefail

config_root="$HOME/.config/family-daily"

set -a
source "$config_root/backend.env"
if [[ -f "$config_root/gemini.env" ]]; then
  source "$config_root/gemini.env"
fi
set +a

exec "$HOME/.local/bin/family-daily"
