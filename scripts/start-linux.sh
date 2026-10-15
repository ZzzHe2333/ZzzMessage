#!/bin/sh
set -eu
cd "$(dirname "$0")"
if [ ! -f .env ]; then
  cp .env.example .env
  echo "Set INVITE_CODE and SESSION_SECRET in .env, then run again." >&2
  exit 1
fi
WEB_DIR="./web"
LISTEN_ADDR="127.0.0.1:58463"
export WEB_DIR LISTEN_ADDR
exec ./zzzmessage
