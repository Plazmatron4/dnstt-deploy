# dnstt-deploy

One repo that builds and runs [dnstt](https://www.bamsoftware.com/software/dnstt/) — a DNS
tunnel — plus a small web UI so you can see what is running and move files around without
fighting a panel's file manager.

dnstt source is vendored, so `go build` works with **no network access**.

---

## Install

**On a VPS or any box where you control the machine:**

```sh
curl -fsSL https://raw.githubusercontent.com/Plazmatron4/dnstt-deploy/main/setup.sh | sh
```

**As a panel startup command** (Pterodactyl, Pelican, etc. — put this in the Startup Command field):

```sh
bash -c 'cd /home/container && ([ -d dnstt-deploy ] || git clone --depth 1 https://github.com/Plazmatron4/dnstt-deploy) && cd dnstt-deploy && sh start.sh'
```

---

## READ THIS BEFORE YOU DEPLOY

**A panel container cannot be a working DNS tunnel endpoint.** Not this one, not any of them.

An NS record holds a *hostname only* — the DNS protocol has no syntax for a port, and a resolver
always asks an authoritative nameserver on **port 53**. A Pterodactyl/Replit/Koyeb/Render
container sits behind the host's NAT on a **shared IP** with a **panel-allocated high port**.
So there is nothing for an NS record to point at.

On a panel the tunnel binary will build, run, and print a public key — and **no resolver on
earth will ever reach it**. The web UI will work fine, because it uses the allocated port.

**You need a box with its own public IPv4.** On any real VPS:

| | |
|---|---|
| Root | yes |
| Bind UDP/53 | yes, you are root |
| Own public IPv4 | yes |

That is the whole requirement. Nothing free provides it; a €1-4/month VPS does.

---

## What it runs

| Process | Port | Purpose |
|---|---|---|
| `dnstt-server` | UDP 53 (`DNSTT_PORT`) | the tunnel |
| `webui` | `$SERVER_PORT` or 8080 | status, file browser, shell |

---

## Environment variables

| Var | Default | Meaning |
|---|---|---|
| `DNSTT_DOMAIN` | `t.example.com` | domain compiled into the server |
| `DNSTT_UPSTREAM` | `127.0.0.1:22` | TCP target the tunnel forwards to |
| `DNSTT_PORT` | `53` | UDP port to listen on |
| `DNSTT_MTU` | `1232` | use 1232 on carriers that strip EDNS |
| `UI_PORT` | `$SERVER_PORT` or `8080` | web UI port |
| `UI_PASS` | generated | web UI password, printed at startup if unset |

Set them in the panel's startup variables, or `export` them before `sh start.sh`.

---

## DNS setup

Your domain's NS record must point at the box. The easiest form uses `sslip.io`, which
resolves any embedded IPv4 — no extra A record needed:

```
yourdomain.tld.       IN NS   ns1.203.0.113.9.sslip.io.
```

`start.sh` prints exactly these two lines with your box's IP filled in.

Then point a client at it:

```sh
dnstt-client -udp 1.1.1.1:53 -pubkey <PUBKEY> yourdomain.tld 127.0.0.1:7000
```

`<PUBKEY>` is printed by `start.sh` and stored in `keys/server.pub`.

---

## Web UI

Served on the UI port with HTTP basic auth — **any username, the password**. The password is
generated on first run and printed to the console; set `UI_PASS` to pin it.

- `/` — tunnel status, public key, listening sockets, disk, uptime, last 40 log lines
- `/files/` — browse, download, upload, mkdir, delete
- `/sh` — run a shell command in the server directory

The shell endpoint runs commands as the server user. Keep the password long.

---

## Layout

```
start.sh          build + run everything
setup.sh          clone-and-start one-liner
dnstt/            dnstt source, vendored deps included
webui/            the web UI (Go, stdlib only)
bin/              build output        (gitignored)
keys/             server keypair      (gitignored)
logs/             tunnel log, pidfile (gitignored)
```

## License

dnstt is **CC0 1.0 Universal** (public domain) — see `dnstt/COPYING`.
