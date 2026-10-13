# signal-receiver

A small bridge that turns **Prometheus Alertmanager** notifications into **Signal**
messages, using a running [signal-cli-rest-api](https://github.com/bbernhard/signal-cli-rest-api)
instance (JSON-RPC mode) as the sender. No Signal account is registered by this project —
it reuses the account configured in your signal-cli-rest-api.

```
Alertmanager ──POST /alert──> signal-receiver ──JSON-RPC send──> signal-cli-rest-api ──> Signal
```

## Configuration

| Env | Default | Meaning |
| --- | --- | --- |
| `SIGNAL_RPC_URL` | `http://localhost:8081` | base URL of signal-cli-rest-api's JSON-RPC API (port 8081 in `MODE=json-rpc`) |
| `SIGNAL_NUMBERS` | *(required)* | comma-separated recipient numbers, e.g. `+48123456789,+48987654321` |
| `LISTEN_ADDR` | `:8080` | listen address |

## Usage

Alertmanager point: webhook receivers may send the standard Alertmanager payload to
`http://<host>:8080/alert`:

```yaml
receivers:
  - name: signal
    webhook_configs:
      - url: http://signal-receiver:8080/alert
        send_resolved: true
```

A non-2xx response makes Alertmanager retry the notification (sent via HTTP 502),
so delivery failure ≠ silent loss. `GET /` answers 200 for health probes.

Message format rendered per alert group:

```
🔥 alertmanager firing [warning]
FIRING SomeAlertName
  job=... instance=...
  summary: ...
  description: ...
```

(`✅` + `RESOLVED ...` for resolved groups; groups longer than ~3500 chars are truncated.)

## Build & run

```bash
# local binary
go build -o signal-receiver .

# container (multi-stage, distroless, nonroot)
podman build -t signal-receiver:dev .
podman run -p 8080:8080 \
  -e SIGNAL_RPC_URL=http://signal-cli-rest-api:8081 \
  -e SIGNAL_NUMBERS=+48123456789 \
  signal-receiver:dev
```

Published image: `ghcr.io/sq7lrx/signal-receiver` — tag per release, pin the digest.
The package is **private**: your cluster needs ghcr pull credentials to pull it
(on k3s, `/etc/rancher/k3s/registries.yaml` with `configs: ghcr.io: auth:` — see the
[k3s private-registry docs](https://docs.k3s.io/installation/private-registry);
on vanilla kubelet, an imagePullSecret).

## Kubernetes

`deploy/k8s.yaml` is a minimal Deployment + Service for the example above.
It is not maintained per-cluster; edit it for your environment.

## Requirements

- signal-cli-rest-api in JSON-RPC mode, account registered and logged in.
- Nothing else — single static binary, no database, no config files.
