# signal-receiver — agent notes

Single Go binary (stdlib only, `main.go` ~150 lines) bridging Alertmanager webhooks to
signal-cli-rest-api's JSON-RPC API. Keep it this way: no dependencies, no frameworks,
no config files — everything is three env vars (see README table).

## Commands

```bash
go vet ./... && go build -o /tmp/signal-receiver .   # build/vet
podman build -t ghcr.io/sq7lrx/signal-receiver:vX.Y.Z .   # container build (distroless)
podman login ghcr.io -u sq7lrx --password-stdin   # PAT needs write:packages
podman push ghcr.io/sq7lrx/signal-receiver:vX.Y.Z
```

## Conventions

- Version flow: `go build` must pass; then podman build + push to ghcr, git-tag `vX.Y.Z`.
  The image digest is what consumers pin — record it in the release.
- The wire protocol with signal-cli-rest-api is a JSON-RPC 2.0 `send` with
  `params.recipients` (array of numbers) + `params.message` at `POST /api/v1/rpc`.
  The array-of-numbers `number` field and `send` via `/api/v1/send` REST endpoint do NOT
  work in `MODE=json-rpc` — verified against 0.101.
- Non-2xx from this server triggers Alertmanager retries: after a send failure reply 502,
  never swallow errors.

## Testing without Signal

The whole path can be exercised against a stub:

```bash
# listener standing in for signal-cli-rest-api
python3 -m http.server 18099 --bind 127.0.0.1 &
SIGNAL_RPC_URL=http://127.0.0.1:18099/api/v1/rpc SIGNAL_NUMBERS=+48000000000 \
  LISTEN_ADDR=:18098 ./signal-receiver &
curl -XPOST -d '{"status":"firing","alerts":[{"status":"firing","labels":{"alertname":"T"}}]}' \
  http://127.0.0.1:18098/alert
```

For real delivery, port-forward signal-cli-rest-api's 8081 (`MODE=json-rpc`) and point
`SIGNAL_RPC_URL` at it.

## Gotchas

- Message length: Signal caps plaintext around a few thousand bytes — the renderer
  truncates at 3500 chars on purpose; keep it that way.
- One static binary, `USER 65532:65532`, no shell in the image (`gcr.io/distroless/static`).
- This repo is deployed by `~/Devel/pixie/monitoring/30-signal-receiver.yaml` — when you
  release a version there, bump the tag AND digest there in the same pass.
