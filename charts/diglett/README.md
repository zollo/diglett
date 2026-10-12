# Diglett Helm chart

Deploys [Diglett](https://github.com/zollo/diglett), the web DNS lookup tool and
API, to Kubernetes as a stateless Deployment behind a ClusterIP Service, with an
optional Ingress, ConfigMap-mounted config file, HPA and PodDisruptionBudget.

## Install

```bash
helm install diglett ./charts/diglett --namespace diglett --create-namespace
kubectl port-forward -n diglett svc/diglett 8080:80   # then open http://127.0.0.1:8080
```

### Private image

While the `ghcr.io/zollo/diglett` package is private, give the cluster a pull
secret (a GitHub PAT with `read:packages`):

```bash
kubectl create secret docker-registry ghcr -n diglett \
  --docker-server=ghcr.io --docker-username=<github-user> --docker-password=<PAT>
helm upgrade --install diglett ./charts/diglett -n diglett \
  --set 'imagePullSecrets[0].name=ghcr'
```

## Image tag

`image.tag` defaults to the chart's `appVersion` (a pinned Diglett release). Set
`image.tag=latest` to follow the rolling build of `main`, or any published
`X.Y.Z`, `X.Y` or `sha-<short>` tag.

## Common values

| Key | Default | Purpose |
| --- | --- | --- |
| `replicaCount` | `1` | Pod count when autoscaling is off. Diglett is stateless, so scaling out is safe. |
| `imagePullSecrets` | `[]` | Pull secrets for a private GHCR package. |
| `service.type` / `service.port` | `ClusterIP` / `80` | The container listens on `containerPort` (`8080`). |
| `ingress.enabled` | `false` | Expose through an Ingress; set `config.trustedProxy: true` alongside it for real client IPs in logs. |
| `config.*` | unset | Rendered as `DIGLETT_*` env vars (`timeout`, `concurrency`, `maxHostnames`, `maxResolvers`, `allowCustomResolvers`, `defaultResolvers`, `trustedProxy`). Unset keeps the app default. |
| `configFile` | `{}` | A full Diglett config (see `config.example.yaml`), mounted from a ConfigMap. Use it for a custom resolver list. `config.*` env vars still win over it. |
| `extraEnv` | `[]` | Extra container env vars, e.g. `DIGLETT_RESOLVERS`. |
| `autoscaling.enabled` | `false` | CPU-based HorizontalPodAutoscaler. |
| `podDisruptionBudget.enabled` | `false` | PodDisruptionBudget (`minAvailable: 1`). |

See [`values.yaml`](./values.yaml) for everything else (resources, probes,
security contexts, scheduling).

## Test

```bash
helm test diglett -n diglett   # hits /healthz through the Service
```
