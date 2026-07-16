# alerting-relay

Receives Alertmanager webhooks, aggregates alerts by group, and keeps a single Slack message per group up to date: firing posts the root message, follow-ups reply in its thread, and resolution edits the root message and drops a final thread reply.

## Build / test / run

```sh
go build ./cmd/alerting-relay
go test ./...
go run ./cmd/alerting-relay
```

## Development setup

`make setup` installs the pinned toolchain (`mise.toml`: go, helm, pre-commit) and registers the pre-commit git hooks (`.pre-commit-config.yaml`).

## Configuration

Set via environment variables:

| Variable         | Description                                                  |
|------------------|----------------------------------------------------------------|
| `DATABASE_URL`   | Postgres DSN                                                    |
| `SLACK_BOT_TOKEN`| Slack bot token                                                 |
| `SLACK_CHANNELS` | JSON mapping cluster label -> `{"alerting": "...", "notifications": "..."}`. A `"default"` entry is used when an alert has no `cluster` label. Values may be a channel name or ID — names are resolved to IDs at startup (requires the `channels:read`/`groups:read` bot scopes); an unresolvable channel fails startup. |
| `SLACK_TEAMS`    | JSON mapping `team` label -> Slack user-group handle or ID, e.g. `{"platform": "platform-team"}`. Resolved to the group's ID at startup (requires `usergroups:read`) to render a real, notifying `<!subteam^ID>` mention; an unresolvable team logs a warning and falls back to plain `@team` text. |
| `WEBHOOK_TOKEN`  | Bearer token required on incoming `/webhook` requests           |
| `LOG_FORMAT`     | `text` (default) or `json`                                      |
| `LOG_LEVEL`      | `debug`, `info` (default), `warn`, or `error`                    |

Listens on `:8080`, exposing `/webhook` (Alertmanager `webhook_configs` target) and `/healthz`.

## Layout

- `cmd/alerting-relay` — entrypoint: config loading and the relay logic wiring Slack + Postgres together
- `internal/webhook` — Alertmanager webhook payload types
- `internal/slack` — Slack Block Kit rendering and posting
- `internal/store` — Postgres persistence of alert-group -> Slack message state
- `charts/alerting-relay` — Helm chart for deploying the relay to Kubernetes

## Deploying with Helm

The chart is published as an OCI artifact to GHCR on every push to `main`/`v*` tags (`.github/workflows/helm-publish.yml`):

```sh
helm install alerting-relay oci://ghcr.io/next-gen-infrastructure/charts/alerting-relay --version 0.1.0 \
  --set ingress.host=alerting-relay.example.com
```

`image.repository` defaults to `ghcr.io/next-gen-infrastructure/alerting-relay` (the image published by `docker-publish.yml`); override `image.tag` to pin a specific version instead of `latest`.

By default the chart references a pre-existing Secret and ConfigMap (names configurable via `secret.name`/`configMap.name`, default `alerting-relay`) providing the app's env vars (`DATABASE_URL`, `SLACK_BOT_TOKEN`, `WEBHOOK_TOKEN`, `SLACK_CHANNELS`, `SLACK_TEAMS`) — bring your own however you manage config. Or set `secret.create`/`configMap.create` to `true` with `secret.content`/`configMap.content` to have the chart create them for you. See `charts/alerting-relay/values.yaml` for the full set of configurable values (replica count, image, service port, ingress class/annotations, resources).

## License

Apache-2.0, see [LICENSE](LICENSE).
