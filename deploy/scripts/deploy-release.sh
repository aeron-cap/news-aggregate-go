#!/usr/bin/env bash
# Activates an uploaded release on the server. .github/workflows/deploy.yml runs it as:
#   ssh deploy@host bash -s -- <sha> < deploy/scripts/deploy-release.sh
# after copying pocket-news-<sha>.tar.gz to the deploy user's home directory.
#
# Server layout (APP_DIR, default /opt/pocket-news):
#   releases/<sha>/   unpacked releases, newest KEEP_RELEASES are kept
#   shared/.env       config linked into every release
#   shared/.venv      Python venv for resnik.py, updated from each release's requirements.txt
#   current           symlink to the active release
#
# The deploy user needs passwordless sudo for exactly:
#   /usr/bin/systemctl restart pocket-news-api, /usr/bin/systemctl reload nginx
set -euo pipefail

sha="${1:-}"
if [[ ! "$sha" =~ ^[0-9a-f]{40}$ ]]; then
  echo "usage: deploy-release.sh <40-character commit sha>" >&2
  exit 2
fi

app_dir="${APP_DIR:-/opt/pocket-news}"
health_url="${HEALTHCHECK_URL:-http://127.0.0.1:6767/healthz}"
keep_releases="${KEEP_RELEASES:-5}"
tarball="$HOME/pocket-news-$sha.tar.gz"
release="$app_dir/releases/$sha"
shared="$app_dir/shared"

switch_to() {
  ln -sfn "$1" "$app_dir/current.tmp"
  mv -Tf "$app_dir/current.tmp" "$app_dir/current"
}

restart() {
  sudo -n systemctl restart pocket-news-api
  sudo -n systemctl reload nginx
}

healthy() {
  for _ in $(seq 1 30); do
    if curl -fsS --max-time 2 "$health_url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

# Unpack next to the target first so a re-deploy of the same sha never leaves a half-written release.
mkdir -p "$app_dir/releases"
staging="$(mktemp -d "$app_dir/releases/.staging-XXXXXX")"
trap 'rm -rf "$staging"' EXIT
tar -xzf "$tarball" -C "$staging"
chmod 755 "$staging"
ln -sfn "$shared/.env" "$staging/.env"
rm -rf "$release"
mv "$staging" "$release"
rm -f "$tarball"

"$shared/.venv/bin/pip" install --quiet --disable-pip-version-check -r "$release/requirements.txt"

previous=""
if [[ -L "$app_dir/current" ]]; then
  previous="$(readlink -f "$app_dir/current")"
fi

switch_to "$release"
restart

if ! healthy; then
  echo "health check failed for $sha" >&2
  if [[ -n "$previous" && -d "$previous" && "$previous" != "$release" ]]; then
    echo "rolling back to $(basename "$previous")" >&2
    switch_to "$previous"
    restart
    if ! healthy; then
      echo "previous release is unhealthy too" >&2
    fi
  else
    echo "no previous release to roll back to" >&2
  fi
  exit 1
fi

echo "deployed $sha"

# Keep the newest releases and never remove the active one.
active="$(readlink -f "$app_dir/current")"
ls -1dt "$app_dir"/releases/*/ | tail -n +"$((keep_releases + 1))" | while read -r dir; do
  dir="${dir%/}"
  if [[ "$dir" != "$active" ]]; then
    rm -rf "$dir"
  fi
done
