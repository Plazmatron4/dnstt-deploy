#!/bin/sh
# setup.sh -- one-line installer.
#
#   curl -fsSL https://raw.githubusercontent.com/Plazmatron4/dnstt-deploy/main/setup.sh | sh
#
# Clones the repo, then builds and starts everything via start.sh.
# Override with env vars:  DNSTT_DIR, DNSTT_DOMAIN, DNSTT_UPSTREAM, DNSTT_PORT, UI_PASS
set -e

REPO="${DNSTT_REPO:-https://github.com/Plazmatron4/dnstt-deploy}"
DIR="${DNSTT_DIR:-$HOME/dnstt-deploy}"

if [ -d "$DIR/.git" ]; then
	echo "[setup] updating $DIR"
	git -C "$DIR" pull --ff-only 2>/dev/null || echo "[setup] pull failed, using what is there"
else
	echo "[setup] cloning into $DIR"
	git clone --depth 1 "$REPO" "$DIR"
fi

cd "$DIR"
chmod +x start.sh setup.sh 2>/dev/null || true
exec sh start.sh
