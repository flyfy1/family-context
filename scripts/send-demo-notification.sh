#!/usr/bin/env bash
set -euo pipefail

title="家里想你了"
message="好几天没有看到新的家庭动态，点这里看看家人的近况。"
title_en="We miss you"
message_en="There haven't been any new family updates for a few days. Tap to see how everyone is doing."
action_url="https://family.integ.life/#/feed"
deploy_host="${FAMILY_DAILY_DEPLOY_HOST:-mmini}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --title) title="${2:-}"; shift 2 ;;
    --message) message="${2:-}"; shift 2 ;;
    --title-en) title_en="${2:-}"; shift 2 ;;
    --message-en) message_en="${2:-}"; shift 2 ;;
    --url) action_url="${2:-}"; shift 2 ;;
    --host) deploy_host="${2:-}"; shift 2 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; exit 2 ;;
  esac
done

if [[ -z "$title" || -z "$message" || -z "$title_en" || -z "$message_en" || "$action_url" != https://* ]]; then
	printf 'Chinese and English titles/messages are required; URL must use HTTPS.\n' >&2
  exit 2
fi
if ! command -v jq >/dev/null; then
  printf 'jq is required to encode the notification safely.\n' >&2
  exit 1
fi

payload="$(jq -n --arg title "$title" --arg titleEn "$title_en" --arg message "$message" --arg messageEn "$message_en" --arg actionUrl "$action_url" \
	'{title:$title,titleEn:$titleEn,message:$message,messageEn:$messageEn,actionUrl:$actionUrl,markNeedsAttention:true}')"
response="$(printf '%s' "$payload" | ssh "$deploy_host" 'set -euo pipefail
  config="$HOME/.config/family-daily/backend.env"
  admin_token=$(sed -n "s/^ADMIN_API_TOKEN=//p" "$config")
  test -n "$admin_token"
  curl --fail --silent --show-error -X POST http://127.0.0.1:8096/api/v1/admin/notifications/broadcast \
    -H "X-Admin-Token: $admin_token" -H "Content-Type: application/json" --data-binary @-')"

created="$(jq -r '.notificationsCreated // 0' <<<"$response")"
marked="$(jq -r '.membersMarkedForAttention // 0' <<<"$response")"
printf 'Marked %s production elders as needing attention.\n' "$marked"
printf 'Created %s member notifications.\n' "$created"

android_adb="${ANDROID_ADB:-$HOME/Library/Android/sdk/platform-tools/adb}"
if [[ -x "$android_adb" ]] && "$android_adb" get-state >/dev/null 2>&1; then
  "$android_adb" shell am broadcast --receiver-foreground \
    -a life.integ.familydaily.DEMO_SYNC_NOTIFICATIONS \
    -p life.integ.familydaily >/dev/null
  printf 'Asked the connected Android demo App to fetch now.\n'
else
  printf 'No connected Android device; signed-in Apps will fetch on launch or background schedule.\n'
fi
