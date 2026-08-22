#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/.." && pwd)"
deploy_host="${FAMILY_DAILY_DEPLOY_HOST:-mmini}"
build_dir="$(mktemp -d)"
trap 'rm -rf "$build_dir"' EXIT

git -C "$root_dir" diff --quiet
git -C "$root_dir" diff --cached --quiet

commit="$(git -C "$root_dir" rev-parse HEAD)"
version="${VERSION:-$(date -u +%Y%m%d-%H%M%S)}"
build_time="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

(cd "$root_dir/backend" && go test ./...)
(cd "$root_dir/backend" && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath \
  -ldflags "-s -w -X main.version=$version -X main.commit=$commit -X main.buildTime=$build_time" \
  -o "$build_dir/family-daily" .)

cp "$root_dir/deploy/run-macos.sh" "$build_dir/run.sh"
cp "$root_dir/deploy/life.integ.family-daily.plist" "$build_dir/life.integ.family-daily.plist"

if [[ -f "$root_dir/.env" ]] && grep -q '^GEMINI_API_KEY=.' "$root_dir/.env"; then
  sed -n '/^GEMINI_API_KEY=./p' "$root_dir/.env" > "$build_dir/gemini.env"
  chmod 600 "$build_dir/gemini.env"
fi

scp "$build_dir/family-daily" "$build_dir/run.sh" "$build_dir/life.integ.family-daily.plist" "$deploy_host:/tmp/"
if [[ -f "$build_dir/gemini.env" ]]; then
  scp "$build_dir/gemini.env" "$deploy_host:/tmp/family-daily-gemini.env"
fi

ssh "$deploy_host" 'set -euo pipefail
umask 077
config_root="$HOME/.config/family-daily"
storage_root="$HOME/.local/share/family-daily/storage"
agent="$HOME/Library/LaunchAgents/life.integ.family-daily.plist"

mkdir -p "$HOME/.local/bin" "$HOME/.local/libexec/family-daily" "$config_root" "$storage_root" "$HOME/Library/LaunchAgents"
touch "$storage_root/.family-daily-storage"

if [[ ! -f "$config_root/backend.env" ]]; then
  admin_token="$(openssl rand -hex 32)"
  {
    printf "%s\n" "APP_ENV=production"
    printf "%s\n" "ADDR=127.0.0.1:8096"
    printf "%s\n" "FAMILY_DAILY_STORAGE_DIR=$storage_root"
    printf "%s\n" "PUBLIC_BASE_URL=https://family-api.integ.life"
    printf "%s\n" "CORS_ALLOWED_ORIGINS=https://family.integ.life,https://family-api.integ.life"
    printf "%s\n" "ADMIN_API_TOKEN=$admin_token"
  } > "$config_root/backend.env"
fi

clean_env="$(mktemp "$config_root/backend.env.XXXXXX")"
awk '!/^FAMILY_API_TOKEN=/' "$config_root/backend.env" > "$clean_env"
chmod 600 "$clean_env"
mv "$clean_env" "$config_root/backend.env"

if ! grep -q '^PUBLIC_BASE_URL=' "$config_root/backend.env"; then
  printf "%s\n" "PUBLIC_BASE_URL=https://family-api.integ.life" >> "$config_root/backend.env"
fi

if [[ -f /tmp/family-daily-gemini.env ]]; then
  install -m 0600 /tmp/family-daily-gemini.env "$config_root/gemini.env"
  rm -f /tmp/family-daily-gemini.env
fi

if [[ -x "$HOME/.local/bin/family-daily" ]]; then
  cp "$HOME/.local/bin/family-daily" "$HOME/.local/bin/family-daily.previous"
fi
install -m 0755 /tmp/family-daily "$HOME/.local/bin/family-daily"
install -m 0755 /tmp/run.sh "$HOME/.local/libexec/family-daily/run.sh"
install -m 0600 /tmp/life.integ.family-daily.plist "$agent"

launchctl bootout "gui/$(id -u)" "$agent" 2>/dev/null || true
sleep 2
launchctl bootstrap "gui/$(id -u)" "$agent"
launchctl kickstart -k "gui/$(id -u)/life.integ.family-daily"

for attempt in {1..30}; do
  if curl --fail --silent http://127.0.0.1:8096/healthz >/dev/null; then
    break
  fi
  sleep 1
done
curl --fail --silent --show-error http://127.0.0.1:8096/healthz
printf "\n"
curl --fail --silent --show-error http://127.0.0.1:8096/version
printf "\n"
'
