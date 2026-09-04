# ⛏️ Diglett

**A free, open-source, self-hostable web DNS lookup tool — a
[digwebinterface.com](https://www.digwebinterface.com) you can run yourself.**

Diglett lets you query one or more DNS hostnames for one or more record types
against a selection of well-known public DNS resolvers, from a clean web UI or
a fully-featured JSON API. It ships as a single tiny container with no external
dependencies and no database.

- 🔎 **Multiple hostnames × multiple record types × multiple resolvers** in one
  request, run concurrently.
- 🧠 **Automatic record discovery** — leave the type unset and Diglett probes a
  curated set of common record types (A, AAAA, CNAME, MX, NS, TXT, SOA, CAA,
  SRV) and shows whichever actually exist.
- 🌐 **Well-known resolvers built in** — Google, Cloudflare, Quad9, OpenDNS,
  AdGuard, plus DNS-over-HTTPS variants. Configure your own, or query any plain
  DNS server address ad-hoc.
- 🔁 **Reverse (PTR) lookups**, **`+trace`-style delegation tracing** from the
  root servers, and **DNSSEC** record requests.
- 🧰 **API-first** — the web UI is just a client of the same unauthenticated
  JSON API. Everything is scriptable with `curl`.
- 🔒 **UDP (with TCP fallback), TCP, DoT and DoH** transports.
- 📦 **Simple delivery** — one static binary, one distroless container,
  configured entirely by a config file or environment variables. Production
  Docker Compose and TrueNAS SCALE app deployments included.

---

## Quick start

### Run with Docker

```bash
docker run --rm -p 8080:8080 ghcr.io/zollo/diglett:latest
```

Open <http://localhost:8080>.

### Run with Docker Compose (production)

```bash
docker compose up -d
```

See [`docker-compose.yml`](./docker-compose.yml) — the container runs read-only,
with no capabilities and no new privileges.

### Run from source

```bash
go run .            # listens on :8080
# or
make build && ./diglett -config config.yaml
```

### Deploy on TrueNAS SCALE

See [`deploy/truenas/`](./deploy/truenas) for both the Docker-Compose custom-app
path (Electric Eel 24.10+) and a full Helm chart with a TrueNAS GUI form.

---

## Container images

Public multi-arch images (`linux/amd64` + `linux/arm64`) are published to the
GitHub Container Registry by CI:

```
ghcr.io/zollo/diglett
```

| Tag | Points at |
| --- | --- |
| `latest` | The most recent build of `main` — a **rolling** tag that can be ahead of the newest release. |
| `main` | The tip of the `main` branch (same image as the current `latest`). |
| `sha-<short>` | A specific commit, for reproducible pins. |
| `X.Y.Z`, `X.Y` | A tagged release (pushed from a `vX.Y.Z` git tag). |

For a stable, reproducible deployment, pin a version tag (`X.Y.Z`) or a
`sha-<short>` digest rather than `latest`.

The [`Publish container`](./.github/workflows/docker-publish.yml) workflow builds
and pushes on every push to `main` and on `v*` tags; pull requests only build the
image (in [`CI`](./.github/workflows/ci.yml)) without publishing. Cutting a
release is just a tag:

```bash
git tag v1.0.0 && git push origin v1.0.0
```

---

## The API

Diglett is API-first: the web UI calls exactly these endpoints, and so can you.
No authentication is required.

### `GET|POST /api/lookup`

Perform a lookup. `POST` takes a JSON body; `GET` takes query parameters (handy
for `curl` and shareable links).

**JSON body (POST):**

```jsonc
{
  "hostnames": ["example.com", "github.com"],
  "type": "MX",                 // omit or use "AUTO" for auto-discovery
  "resolvers": ["google", "cloudflare"], // IDs or literal addresses
  "reverse": false,             // treat hostnames as IPs, do PTR lookups
  "trace": false,               // iterative trace from the root servers
  "dnssec": false               // set the DO bit, request DNSSEC records
}
```

**Query parameters (GET):** `name`/`host`/`q` (repeatable or comma/space/newline
separated), `type`, `resolvers`, `reverse`, `trace`, `dnssec`, and `text=1` for
a plain-text, dig-style response.

**Examples:**

```bash
BASE=http://localhost:8080

# Auto-discover all common records for a hostname
curl "$BASE/api/lookup?name=example.com"

# A specific type across two resolvers (JSON)
curl -X POST "$BASE/api/lookup" -H 'Content-Type: application/json' -d '{
  "hostnames": ["example.com", "github.com"],
  "type": "MX",
  "resolvers": ["google", "cloudflare"]
}'

# Reverse lookup, terminal-friendly text output
curl "$BASE/api/lookup?name=8.8.8.8&reverse=1&text=1"

# Query an ad-hoc resolver by address
curl "$BASE/api/lookup?name=example.com&type=A&resolvers=9.9.9.9"
```

**Response shape (JSON):**

```jsonc
{
  "queries": [
    {
      "hostname": "example.com",
      "type": "A",
      "resolver": { "id": "google", "name": "Google", "address": "8.8.8.8", "protocol": "udp" },
      "status": "NOERROR",
      "answers": [
        { "name": "example.com.", "type": "A", "class": "IN", "ttl": 300, "data": "93.184.216.34" }
      ],
      "authority": [],
      "additional": [],
      "query_time_ms": 20,
      "server": "8.8.8.8:53",
      "protocol": "udp",
      "raw": ";; Query: example.com  Type: A ..."
    }
  ],
  "duration_ms": 21
}
```

### `GET /api/resolvers`

Lists the configured resolvers, the default selection, and the supported record
types — this is what populates the UI.

### `GET /api/version` · `GET /healthz`

Version info and a health probe.

---

## Configuration

Configuration is layered: **built-in defaults → optional YAML file → environment
variables** (env always wins). Point at a file with `-config path.yaml` or
`DIGLETT_CONFIG=path.yaml`. See [`config.example.yaml`](./config.example.yaml)
for a fully commented reference.

### Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `DIGLETT_CONFIG` | — | Path to a YAML config file. |
| `DIGLETT_HOST` | `0.0.0.0` | Listen address. |
| `DIGLETT_PORT` | `8080` | Listen port. |
| `DIGLETT_TRUSTED_PROXY` | `false` | Honour `X-Forwarded-For` in logs. |
| `DIGLETT_TIMEOUT` | `5s` | Per-exchange DNS timeout. |
| `DIGLETT_CONCURRENCY` | `12` | Max simultaneous exchanges per request. |
| `DIGLETT_MAX_HOSTNAMES` | `100` | Max hostnames per request (0 = unlimited). |
| `DIGLETT_MAX_RESOLVERS` | `20` | Max resolvers per request (0 = unlimited). |
| `DIGLETT_ALLOW_CUSTOM_RESOLVERS` | `true` | Allow ad-hoc plain-DNS resolver addresses (see note). |
| `DIGLETT_DEFAULT_RESOLVERS` | `google,cloudflare,quad9` | Pre-selected resolver IDs. |
| `DIGLETT_RESOLVERS` | built-in list | Inline resolver definitions (see below). |

`DIGLETT_RESOLVERS` is a comma-separated list of resolvers, each in the compact
form `id|Name|address|protocol|group`:

```bash
DIGLETT_RESOLVERS="google|Google|8.8.8.8|udp|Public, cf-doh|Cloudflare DoH|https://cloudflare-dns.com/dns-query|https|Encrypted"
```

### Resolver protocols

| `protocol` | Meaning | `address` form |
| --- | --- | --- |
| `udp` (default) | Plain UDP, falls back to TCP on truncation | `host` or `host:port` |
| `tcp` | Plain TCP | `host` or `host:port` |
| `tls` | DNS-over-TLS (DoT) | `host` or `host:port` (default `:853`) |
| `https` | DNS-over-HTTPS (DoH) | full URL, e.g. `https://dns.google/dns-query` |

> **Security note.** `DIGLETT_ALLOW_CUSTOM_RESOLVERS` only permits ad-hoc
> resolvers that are **plain DNS servers** (an IP or host, queried over
> UDP/TCP). DoH/DoT endpoints — which involve the server making an HTTP(S)
> request — can only be defined in the configured resolver set and referenced
> by `id`; arbitrary URLs are rejected so the unauthenticated API cannot be
> abused as a server-side request forgery (SSRF) vector. Set it to `false` to
> disallow ad-hoc resolvers entirely.

---

## Development

```bash
make test    # go test -race ./...
make vet     # go vet ./...
make fmt     # gofmt -w .
make run     # run locally on :8080
make docker  # build the container image
```

The web UI (`web/static/`) is plain HTML/CSS/JS with no build step, embedded
into the binary via `go:embed`, so the compiled binary is fully self-contained.

### Project layout

```
main.go                     entrypoint / flag parsing
internal/config             layered configuration (defaults, YAML, env)
internal/lookup             DNS query engine (transports, discovery, trace)
internal/server             HTTP API + embedded UI serving
web/static                  the web UI (embedded)
deploy/truenas              TrueNAS SCALE Helm chart + custom-app compose
Dockerfile, docker-compose.yml
```

---

## License

[MIT](./LICENSE).
