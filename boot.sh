#!/bin/sh
# boot.sh -- ONE LINE DOES EVERYTHING.
#
#   bash -c "curl -fsSL https://raw.githubusercontent.com/Plazmatron4/dnstt-deploy/main/boot.sh | bash"
#
# Fetches whatever is missing (start.sh, the web UI, the dnstt source), builds it,
# and starts the tunnel plus the web UI. Safe to re-run: it skips what is present.
#
# Env, all optional:
#   DNSTT_DOMAIN    domain compiled into the server    (default t.example.com)
#   DNSTT_UPSTREAM  TCP target to forward to           (default 127.0.0.1:22)
#   DNSTT_PORT      UDP port to listen on              (default 53)
#   DNSTT_MTU       tunnel MTU                         (default 1232)
#   UI_PORT         web UI port (panel's SERVER_PORT wins)
#   UI_PASS         web UI password (generated + printed if unset)
#   DNSTT_DIR       where to install (default: /home/container, else $HOME)
set -e

RAW="${DNSTT_RAW:-https://raw.githubusercontent.com/Plazmatron4/dnstt-deploy/main}"
TARBALL="${DNSTT_TARBALL:-https://codeload.github.com/Plazmatron4/dnstt-deploy/tar.gz/refs/heads/main}"

# ------------------------------------------------------------ pick install dir
DIR="${DNSTT_DIR:-}"
if [ -z "$DIR" ]; then
	for c in /home/container "$HOME" "$(pwd)"; do
		if [ -n "$c" ] && [ -d "$c" ] && [ -w "$c" ]; then DIR="$c"; break; fi
	done
fi
[ -n "$DIR" ] || { echo "[boot] no writable install directory"; exit 1; }
cd "$DIR"
echo "[boot] installing into $DIR"

# ------------------------------------------------------------- start.sh + web UI
for f in webui/main.go webui/go.mod; do
	mkdir -p "$(dirname "$f")"
	[ -s "$f" ] && continue
	echo "[boot] fetching $f"
	curl -fsSL "$RAW/$f" -o "$f" || { echo "[boot] could not fetch $f"; exit 1; }
done

# start.sh keeps changing; always take the fresh copy (a few KB)
echo "[boot] fetching start.sh"
curl -fsSL "$RAW/start.sh" -o start.sh || { echo "[boot] could not fetch start.sh"; exit 1; }

# --------------------------------------------------- prebuilt web UI (fast path)
# Compiling on a panel can hang, and pushing a multi-MB file over the panel's
# SFTP dies mid-transfer. Pulling a gzipped binary from GitHub does neither.
case "$(uname -m 2>/dev/null)" in
	aarch64|arm64) UI_GZ="webui-linux-arm64.gz" ;;
	*)             UI_GZ="webui-linux-amd64.gz" ;;
esac
if [ ! -x bin/webui ]; then
	mkdir -p bin
	echo "[boot] fetching prebuilt web UI ($UI_GZ)..."
	if curl -fsSL "$RAW/$UI_GZ" -o bin/webui.gz; then
		if gzip -dc bin/webui.gz >bin/webui 2>/dev/null; then
			chmod +x bin/webui 2>/dev/null || true
			echo "[boot] web UI ready"
		fi
	fi
fi

# --------------------------------------------------------------- dnstt source
if [ ! -f dnstt-server/main.go ] && [ ! -f dnstt/dnstt-server/main.go ]; then
	echo "[boot] fetching dnstt source..."
	rm -rf .boottmp
	if command -v git >/dev/null 2>&1; then
		git clone --depth 1 https://github.com/Plazmatron4/dnstt-deploy .boottmp
	elif command -v curl >/dev/null 2>&1 && command -v tar >/dev/null 2>&1; then
		mkdir -p .boottmp
		curl -fsSL "$TARBALL" | tar xz -C .boottmp --strip-components=1
	else
		echo "[boot] need git, or curl+tar, to fetch the dnstt source"
		exit 1
	fi
	mkdir -p dnstt
	cp -a .boottmp/dnstt/. dnstt/
	rm -rf .boottmp
	echo "[boot] source ready"
fi

chmod +x start.sh 2>/dev/null || true
exec sh start.sh
