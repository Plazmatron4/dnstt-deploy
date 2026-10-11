#!/bin/sh
# start.sh -- build + run dnstt-server and the web UI.
# Works as a Pterodactyl startup command, or on any VPS.
#
#   sh start.sh
#
# Environment (all optional):
#   DNSTT_DOMAIN   the domain compiled into the server      (default t.example.com)
#   DNSTT_UPSTREAM TCP target the tunnel forwards to        (default 127.0.0.1:22)
#   DNSTT_PORT     UDP port the server listens on           (default 53)
#   DNSTT_MTU      tunnel MTU, 1232 for carriers that strip EDNS (default 1232)
#   UI_PORT        web UI port (SERVER_PORT from the panel wins)
#   UI_PASS        web UI password; generated and printed if unset
set -u

cd "$(dirname "$0")" 2>/dev/null || true
ROOT="$(pwd)"

DOMAIN="${DNSTT_DOMAIN:-t.example.com}"
UPSTREAM="${DNSTT_UPSTREAM:-127.0.0.1:22}"
DNSTT_PORT="${DNSTT_PORT:-53}"
UI_PORT="${SERVER_PORT:-${UI_PORT:-8080}}"
MTU="${DNSTT_MTU:-1232}"

# the panel may hand us an unwritable HOME; keep every cache inside the server dir
export HOME="${HOME:-$ROOT}"
export GOCACHE="${GOCACHE:-$ROOT/.gocache}"
export GOMODCACHE="${GOMODCACHE:-$ROOT/.gomodcache}"
export GOFLAGS="-mod=vendor"
mkdir -p bin keys logs "$GOCACHE" "$GOMODCACHE" 2>/dev/null

# ---------------------------------------------------------------- tunnel binary
if [ ! -x bin/dnstt-server ]; then
	echo "[dnstt] building dnstt-server (vendored deps, no network needed)..."
	( cd dnstt && go build -o "$ROOT/bin/dnstt-server" ./dnstt-server ) || {
		echo "[dnstt] BUILD FAILED -- is go installed?"
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
echo "  DNS records you must create at your registrar:"
echo "    $DOMAIN.            IN NS   ns1.<THIS-BOX-PUBLIC-IP>.sslip.io."
echo "    ns1.<IP>.sslip.io.  IN A    <THIS-BOX-PUBLIC-IP>"
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
