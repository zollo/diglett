# Deploying Diglett on TrueNAS SCALE

There are two supported paths depending on your TrueNAS SCALE version. Both
run the same stateless container — Diglett stores no data.

## Option A — Custom App via Docker Compose (Electric Eel 24.10 and newer)

TrueNAS SCALE Electric Eel runs apps with Docker. This is the simplest path.

1. In the web UI, go to **Apps → Discover Apps**.
2. Click **Custom App** (top right), then choose **Install via YAML**.
3. Paste the contents of [`docker-compose.yaml`](./docker-compose.yaml).
4. Adjust the published port on the left side of `"8080:8080"` if 8080 is
   already taken, then install.
5. Open `http://<truenas-ip>:8080`.

## Option B — Helm chart / custom catalog app (older SCALE, Bluefin/Cobia)

The [`diglett/`](./diglett) directory is a self-contained Helm chart with a
TrueNAS `questions.yaml` GUI form.

### Install directly with Helm

```bash
helm install diglett ./deploy/truenas/diglett \
  --set service.nodePort=30080
```

Then browse to `http://<node-ip>:30080`.

### Add as a TrueNAS catalog app

1. Host this repository (or a fork) somewhere reachable by TrueNAS.
2. In **Apps → Discover Apps → (gear) → Manage Catalogs → Add Catalog**, add
   the repository with a train that contains the `deploy/truenas` directory.
3. Diglett appears in the catalog with a configuration form generated from
   `questions.yaml` (port, query limits, default resolvers, extra env vars).

## Configuration

All configuration is via `DIGLETT_*` environment variables — see the
[main README](../../README.md#configuration) for the full list. To ship a
completely custom resolver set, set `DIGLETT_RESOLVERS`, for example:

```
DIGLETT_RESOLVERS=google|Google|8.8.8.8|udp|Public, cf-doh|Cloudflare DoH|https://cloudflare-dns.com/dns-query|https|Encrypted
```
