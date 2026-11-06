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
| `DATABASE_URL`   | Postgres DSN, or `sqlite://path/to/file.db` (`sqlite://:memory:` for in-memory) |
| `SLACK_BOT_TOKEN`| Slack bot token. Requires the `chat:write` scope; add `channels:read`/`groups:read` and/or `usergroups:read` if `SLACK_CHANNELS`/`SLACK_TEAMS` use names/handles instead of IDs (see below) |
| `SLACK_CHANNELS` | JSON mapping cluster label -> `{"alerting": "...", "notifications": "..."}`. A `"default"` entry is used when an alert has no `cluster` label. Alerts with severity `critical`, `high` or `warning` go to `alerting`; anything else goes to `notifications`. Values may be a channel name or ID — names are resolved to IDs at startup (requires the `channels:read`/`groups:read` bot scopes); an unresolvable channel fails startup. |
| `SLACK_TEAMS`    | JSON mapping `team` label -> Slack user-group handle or ID, e.g. `{"platform": "platform-team"}`. Resolved to the group's ID at startup (requires `usergroups:read`) to render a real, notifying `<!subteam^ID>` mention; an unresolvable team logs a warning and falls back to plain `@team` text. |
| `ALERTMANAGER_URL` | Optional Alertmanager base URL. When set, inhibited alerts in a group's scope (cluster/namespace/workload, or alertname without a workload) are fetched from `/api/v2/alerts` and posted as one thread comment each |
| `ALERTMANAGER_USERNAME` / `ALERTMANAGER_PASSWORD` | Optional basic-auth credentials for `ALERTMANAGER_URL` |
| `WEBHOOK_TOKEN`  | Bearer token required on incoming `/webhook` requests           |
| `LOG_FORMAT`     | `text` (default) or `json`                                      |
| `LOG_LEVEL`      | `debug`, `info` (default), `warn`, or `error`                    |

Listens on `:8080`, exposing `/webhook` (Alertmanager `webhook_configs` target), `/argocd` (Argo CD notifications target) and `/healthz`.

## Argo CD notifications

`POST /argocd` (same `Authorization: Bearer $WEBHOOK_TOKEN`) keeps one Slack message per `cluster@revision` (just `revision` when `cluster` is empty; the revision links to its GitHub commit when `repo` is a GitHub URL): the root lists every app on that revision with its latest status (worst status sets the colour), each event also replies in its thread, and the message closes once every app is `healthy` (a later event on the same revision starts a new thread). Channel is picked from `SLACK_CHANNELS` by `cluster`: if any app is `sync-failed`/`degraded` the message is in `alerting` (a failure after the revision already posted to `notifications` starts a new thread there), otherwise in `notifications`. Events are accumulated for 5 seconds per revision, so a burst costs one root update and one combined thread reply instead of one per event (events queued in the last few seconds are lost on restart). Body fields: `app`, `event` (`syncing`, `synced`, `sync-failed`, `degraded`, `healthy`), `revision`, `cluster`, `namespace`, `repo`, `message`, `url`. Example `argocd-notifications-cm`:

```yaml
service.webhook.relay: |
  url: http://alerting-relay/argocd
  headers: [{name: Authorization, value: Bearer $relay-token}, {name: Content-Type, value: application/json}]
template.relay-base: |
  webhook:
    relay:
      method: POST
      body: |
        {"app":"{{.app.metadata.name}}","repo":"{{.app.spec.source.repoURL}}","cluster":"{{.app.spec.destination.name}}",
         "namespace":"{{.app.spec.destination.namespace}}","revision":"{{.app.status.sync.revision}}",
         "event":"EVENT","message":"{{.app.status.operationState.message}}",
         "url":"{{.context.argocdUrl}}/applications/{{.app.metadata.name}}"}
```

Define one template per event (`EVENT` = `syncing` etc.) and subscribe via triggers `on-sync-running`, `on-sync-succeeded`, `on-sync-failed`, `on-health-degraded`, `on-deployed`/health-healthy.

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
