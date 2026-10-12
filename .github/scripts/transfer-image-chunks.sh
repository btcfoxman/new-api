#!/usr/bin/env bash
set -euo pipefail

: "${DEPLOY_IMAGE:?missing DEPLOY_IMAGE}"
: "${DEPLOY_HOST:?missing DEPLOY_HOST}"
: "${DEPLOY_USER:?missing DEPLOY_USER}"
: "${DEPLOY_PORT:?missing DEPLOY_PORT}"

image_tag="${DEPLOY_IMAGE##*:}"
if [[ "$DEPLOY_IMAGE" != ghcr.io/* || ! "$image_tag" =~ ^[a-f0-9]{12}$ ]]; then
  echo "A GHCR image with a 12-character SHA tag is required" >&2
  exit 1
fi

ssh_key="$HOME/.ssh/newapi_image_transfer"
ssh_options=(
  -i "$ssh_key" -p "$DEPLOY_PORT"
  -o BatchMode=yes -o IdentitiesOnly=yes
  -o StrictHostKeyChecking=accept-new
  -o ConnectTimeout=15 -o ServerAliveInterval=15 -o ServerAliveCountMax=3
)
remote="$DEPLOY_USER@$DEPLOY_HOST"
remote_dir="/tmp/newapi-image-transfer-$image_tag"
archive_dir="$(mktemp -d)"
trap 'rm -rf "$archive_dir"' EXIT

docker pull "$DEPLOY_IMAGE"
archive="$archive_dir/image.tar.gz"
docker save "$DEPLOY_IMAGE" | gzip -1 > "$archive"
split -b 4m -d -a 4 "$archive" "$archive_dir/chunk_"
mapfile -t chunks < <(find "$archive_dir" -maxdepth 1 -type f -name 'chunk_[0-9][0-9][0-9][0-9]' | sort)
echo "Prepared ${#chunks[@]} chunks from $(wc -c < "$archive") compressed bytes"

for index in "${!chunks[@]}"; do
  chunk="${chunks[$index]}"
  name="${chunk##*/}"
  expected="$(sha256sum "$chunk" | cut -d ' ' -f 1)"
  remote_file="$remote_dir/$name"
  echo "Transferring chunk $((index + 1))/${#chunks[@]}: $name"

  remote_hash="$(timeout 45s ssh "${ssh_options[@]}" "$remote" \
    "if [ -f '$remote_file' ]; then sha256sum '$remote_file' | cut -d ' ' -f 1; fi" || true)"
  if [[ "$remote_hash" == "$expected" ]]; then
    echo "Chunk $name already verified on server"
    continue
  fi

  uploaded=false
  for attempt in 1 2 3 4 5; do
    remote_command="set -e; mkdir -p '$remote_dir'; cat > '$remote_file.part'; actual=\$(sha256sum '$remote_file.part' | cut -d ' ' -f 1); test \"\$actual\" = '$expected'; mv '$remote_file.part' '$remote_file'"
    if python3 - "$chunk" <<'PY' | timeout 5m ssh "${ssh_options[@]}" "$remote" "$remote_command"
import sys
import time

rate = 1024 * 1024
sent = 0
started = time.monotonic()
with open(sys.argv[1], "rb") as source:
    while block := source.read(65536):
        sys.stdout.buffer.write(block)
        sys.stdout.buffer.flush()
        sent += len(block)
        delay = sent / rate - (time.monotonic() - started)
        if delay > 0:
            time.sleep(delay)
PY
    then
      uploaded=true
      break
    fi
    echo "Chunk $name attempt $attempt failed; retrying" >&2
    sleep 5
  done
  if [[ "$uploaded" != true ]]; then
    echo "Could not transfer $name after five attempts; verified chunks remain on server" >&2
    exit 1
  fi
done

echo "Loading verified image chunks on server"
timeout 10m ssh "${ssh_options[@]}" "$remote" \
  "bash -o pipefail -c \"cat '$remote_dir'/chunk_[0-9][0-9][0-9][0-9] | gzip -d | docker load\""
ssh "${ssh_options[@]}" "$remote" \
  "docker image inspect '$DEPLOY_IMAGE' >/dev/null && rm -rf '$remote_dir'"
