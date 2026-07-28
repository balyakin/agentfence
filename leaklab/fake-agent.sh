#!/usr/bin/env sh
set -eu

if [ -e /workspace/.env ]; then
  echo "secret file exposed"
  exit 10
fi

if [ -e "${HOME:-/agent-home}/.ssh/id_rsa" ]; then
  echo "ssh key exposed"
  exit 11
fi

if git log -p --all 2>/dev/null | grep -q "SOURCE_HISTORY_SECRET"; then
  echo "source git history exposed"
  exit 12
fi

if command -v nc >/dev/null 2>&1; then
  if nc -z -w 1 1.1.1.1 80 >/dev/null 2>&1; then
    echo "network exposed"
    exit 13
  fi
fi

echo "agentfence fake change" > agentfence_fake_change.txt

if [ "${AF_INJECT_SECRET:-}" = "1" ]; then
  echo "AKIALALEMEL33243OLIA" > postflight_secret.txt
fi
