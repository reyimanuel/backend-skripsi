#!/bin/sh
set -eu

# Writes secret files from environment variables, so you don't need to bake or git-commit them.
# Supported env vars:
# - PRIVATE_KEY_PEM_B64 or PRIVATE_KEY_PEM  -> writes to $PRIVATE_KEY (default: /app/keys/private_key.pem)
# - PUBLIC_KEY_PEM_B64  or PUBLIC_KEY_PEM   -> writes to $PUBLIC_KEY  (default: /app/keys/public_key.pem)
# - FIREBASE_CREDENTIALS_JSON_B64 or FIREBASE_CREDENTIALS_JSON -> writes to $FIREBASE_CREDENTIALS (default: /app/keys/firebase.json)

mkdir -p /app/keys

write_text() {
  target="$1"
  content="$2"

  mkdir -p "$(dirname "$target")"
  # Convert literal \n sequences into newlines (common when pasting into dashboards)
  printf '%s' "$content" | sed 's/\\n/\
/g' > "$target"
}

write_b64() {
  target="$1"
  b64="$2"

  mkdir -p "$(dirname "$target")"
  printf '%s' "$b64" | base64 -d > "$target"
}

ensure_private_key() {
  if [ -n "${PRIVATE_KEY_PEM_B64:-}" ] || [ -n "${PRIVATE_KEY_PEM:-}" ]; then
    PRIVATE_KEY_PATH="${PRIVATE_KEY:-/app/keys/private_key.pem}"
    if [ -n "${PRIVATE_KEY_PEM_B64:-}" ]; then
      write_b64 "$PRIVATE_KEY_PATH" "$PRIVATE_KEY_PEM_B64"
    else
      write_text "$PRIVATE_KEY_PATH" "$PRIVATE_KEY_PEM"
    fi
    chmod 600 "$PRIVATE_KEY_PATH" || true
    export PRIVATE_KEY="$PRIVATE_KEY_PATH"
  fi
}

ensure_public_key() {
  if [ -n "${PUBLIC_KEY_PEM_B64:-}" ] || [ -n "${PUBLIC_KEY_PEM:-}" ]; then
    PUBLIC_KEY_PATH="${PUBLIC_KEY:-/app/keys/public_key.pem}"
    if [ -n "${PUBLIC_KEY_PEM_B64:-}" ]; then
      write_b64 "$PUBLIC_KEY_PATH" "$PUBLIC_KEY_PEM_B64"
    else
      write_text "$PUBLIC_KEY_PATH" "$PUBLIC_KEY_PEM"
    fi
    chmod 644 "$PUBLIC_KEY_PATH" || true
    export PUBLIC_KEY="$PUBLIC_KEY_PATH"
  fi
}

ensure_firebase_creds() {
  if [ -n "${FIREBASE_CREDENTIALS_JSON_B64:-}" ] || [ -n "${FIREBASE_CREDENTIALS_JSON:-}" ]; then
    FIREBASE_PATH="${FIREBASE_CREDENTIALS:-/app/keys/firebase.json}"
    if [ -n "${FIREBASE_CREDENTIALS_JSON_B64:-}" ]; then
      write_b64 "$FIREBASE_PATH" "$FIREBASE_CREDENTIALS_JSON_B64"
    else
      write_text "$FIREBASE_PATH" "$FIREBASE_CREDENTIALS_JSON"
    fi
    chmod 600 "$FIREBASE_PATH" || true
    export FIREBASE_CREDENTIALS="$FIREBASE_PATH"
  fi
}

ensure_private_key
ensure_public_key
ensure_firebase_creds

exec "$@"
