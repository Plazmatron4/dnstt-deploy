#!/bin/sh
# start.sh -- build + run dnstt-server and the web UI.
#
#   sh start.sh
#
# Finds the dnstt source automatically, so this works in two layouts:
#   ./dnstt/dnstt-server/main.go   (full repo clone)
#   ./dnstt-server/main.go         (this script dropped into a dnstt checkout)
#
# Environment (all optional -- edit the defaults below, or export them):
#   DNSTT_DOMAIN   domain compiled into the server       (default t.example.com)
#   DNSTT_UPSTREAM TCP target the tunnel forwards to     (default 127.0.0.1:22)
#   DNSTT_PORT     UDP port the server listens on        (default 53)
#   DNSTT_MTU      tunnel MTU; 1232 where EDNS is stripped (default 1232)
#   UI_PORT        web UI port (a panel's SERVER_PORT wins over this)
#   UI_PASS        web UI password; generated and printed if unset
set -u

cd "$(dirname "$0")" 2>/dev/null || true
ROOT="$(pwd)"

DOMAIN="${DNSTT_DOMAIN:-t.example.com}"
UPSTREAM="${DNSTT_UPSTREAM:-127.0.0.1:22}"
DNSTT_PORT="${DNSTT_PORT:-53}"
UI_PORT="${SERVER_PORT:-${UI_PORT:-8080}}"
MTU="${DNSTT_MTU:-1232}"

# panels often hand us an unwritable HOME; keep every cache inside the server dir
export HOME="${HOME:-$ROOT}"
export GOCACHE="${GOCACHE:-$ROOT/.gocache}"
export GOMODCACHE="${GOMODCACHE:-$ROOT/.gomodcache}"
mkdir -p bin keys logs "$GOCACHE" "$GOMODCACHE" 2>/dev/null

# ------------------------------------------------- find the dnstt source tree
SRC=""
for cand in "$ROOT/dnstt" "$ROOT" "$ROOT/dnstt-deploy/dnstt"; do
	if [ -f "$cand/dnstt-server/main.go" ] && [ -f "$cand/go.mod" ]; then
		SRC="$cand"
		break
	fi
done
if [ -z "$SRC" ]; then
	echo "[dnstt] cannot find the dnstt source (no dnstt-server/main.go + go.mod)."
	echo "[dnstt] clone it:  git clone --depth 1 https://github.com/Plazmatron4/dnstt-deploy"
	exit 1
fi
echo "[dnstt] source: $SRC"

# vendored deps build with no network; a plain checkout needs the module cache
if [ -d "$SRC/vendor" ]; then GOFLAGS="-mod=vendor"; else GOFLAGS=""; fi
export GOFLAGS

# ---------------------------------------------------------------- tunnel binary
if [ ! -x bin/dnstt-server ]; then
	echo "[dnstt] building dnstt-server..."
	( cd "$SRC" && go build -o "$ROOT/bin/dnstt-server" ./dnstt-server ) || {
		echo "[dnstt] BUILD FAILED"
		exit 1
	}
fi

# ------------------------------------------------------------------------- keys
if [ ! -f keys/server.key ]; then
	echo "[dnstt] generating keypair..."
	./bin/dnstt-server -gen-key -privkey-file keys/server.key -pubkey-file keys/server.pub || exit 1
fi
PUB="$(cat keys/server.pub 2>/dev/null || echo '?')"

echo
echo "  ----------------------------------------------------------------"
echo "  dnstt-server"
echo "    domain      : $DOMAIN"
echo "    public key  : $PUB"
echo "    listen      : udp/$DNSTT_PORT"
echo "    forwards to : $UPSTREAM"
echo
echo "  DNS records to create at your registrar:"
echo "    $DOMAIN.    IN NS   ns1.<THIS-BOX-PUBLIC-IP>.sslip.io."
echo "    (sslip.io resolves any embedded IPv4, so no separate A record is needed)"
echo
echo "  client:  dnstt-client -udp <resolver>:53 -pubkey $PUB $DOMAIN 127.0.0.1:7000"
echo "  ----------------------------------------------------------------"
echo

# ----------------------------------------------------------------- start tunnel
echo "[dnstt] starting tunnel..."
./bin/dnstt-server -udp ":$DNSTT_PORT" -privkey-file keys/server.key -mtu "$MTU" \
	"$DOMAIN" "$UPSTREAM" >>logs/dnstt.log 2>&1 &
echo $! >logs/dnstt.pid

# --------------------------------------------------------------------- start UI
if [ ! -x bin/webui ]; then
	echo "[dnstt] building web UI..."
	( cd webui && go build -o "$ROOT/bin/webui" . ) || echo "[dnstt] UI build failed, running headless"
fi

if [ -x bin/webui ]; then
	echo "[dnstt] starting web UI on :$UI_PORT"
	exec env UI_PASS="${UI_PASS:-}" ./bin/webui \
		-addr "0.0.0.0:$UI_PORT" -root "$ROOT" -domain "$DOMAIN" \
		-pub keys/server.pub -log logs/dnstt.log -pid logs/dnstt.pid
fi

wait
