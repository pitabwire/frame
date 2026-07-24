# Queue (Go Cloud Pub/Sub + HTTP push)

Frame wraps Go Cloud Pub/Sub for **pull** transports and adds a first-class **HTTP push** path for Knative Eventing and Cloud Tasks. Application registration stays the same; mode is selected by URL scheme.

## Concepts

- `Publisher`: publish messages to a named queue.
- `Subscriber`: receive messages and dispatch to handlers.
- `Manager`: register and manage publishers/subscribers.
- **Push mux**: `POST /_frame/queue/{reference}` demultiplexes to push subscribers.

## Quick Start (local)

```go
_, svc := frame.NewService(
    frame.WithRegisterPublisher("orders", "mem://orders"),
    frame.WithRegisterSubscriber("orders", "mem://orders", handler),
)

_ = svc.QueueManager().Publish(ctx, "orders", OrderCreated{ID: "123"})
```

## URL schemes

### Subscribe

| Scheme | Mode | Behavior |
|--------|------|----------|
| `mem://` | pull | In-memory gocloud |
| `nats://` | pull | NATS / JetStream via `natspubsub` |
| `gcppubsub://` | pull | GCP Pub/Sub StreamingPull via gocloud (`gcppubsub` driver; ADC or emulator). Frame blank-imports the driver by default; production apps should also blank-import `_ "gocloud.dev/pubsub/gcppubsub"` in `main` so GCP environments keep the driver linked. |
| `push://{ref}` | push | HTTP only; demux by **registration reference** |
| `http(s)://…` | push | Completes latent stub; demux still by registration ref |

Query on push URLs: `protocol=auto|raw|cloudevents|cloudtasks` (default `auto`).

**GCP Pub/Sub push vs pull:** `gcppubsub://…/subscriptions/…` is always **pull** inside Frame (the process opens a subscription and receives). If you configure a *GCP-side* push subscription that POSTs to this service, register the subscriber as `push://{ref}` and point the push endpoint at `https://…/_frame/queue/{ref}` — do not use `gcppubsub://` for that path (no gocloud receive loop; HTTP demux only).

### Publish

| Scheme | Behavior |
|--------|----------|
| `mem://` / `nats://` / `gcppubsub://` | gocloud OpenTopic |
| `ce+https://host/path?type=…&source=…` | CloudEvents 1.0 **binary** HTTP POST (`ce+` stripped on the wire) |
| `cloudtasks:///projects/{p}/locations/{l}/queues/{q}?url=…` | Cloud Tasks CreateTask REST |

GCP topic URL forms (gocloud):

```text
gcppubsub://projects/{project}/topics/{topic}
gcppubsub://{project}/{topic}
```

GCP subscription URL forms (pull):

```text
gcppubsub://projects/{project}/subscriptions/{sub}
gcppubsub://{project}/{sub}
```

Canonical Cloud Tasks form (empty host):

```text
cloudtasks:///projects/p/locations/l/queues/q?url=https%3A%2F%2Fsvc%2F_frame%2Fqueue%2Forders&oidc_sa=sa@proj.iam.gserviceaccount.com
```

## Knative (same handlers)

```go
frame.WithRegisterPublisher("orders",
    "ce+https://broker-ingress.knative-eventing.svc.cluster.local/default/default"+
        "?type=com.example.order.created&source=//orders-svc"),
frame.WithRegisterSubscriber("orders", "push://orders", handler{}),
// Cluster-internal: RUN_SERVICE_SECURELY=false or FRAME_QUEUE_PUSH_REQUIRE_AUTH=false
// or use bearer/OIDC as appropriate
```

Trigger:

```yaml
apiVersion: eventing.knative.dev/v1
kind: Trigger
metadata:
  name: orders-to-svc
spec:
  broker: default
  filter:
    attributes:
      type: com.example.order.created
  subscriber:
    uri: http://orders.default.svc.cluster.local/_frame/queue/orders
```

## GCP Pub/Sub (Cloud Run)

Reliable path: **publish to a topic via gocloud**, **receive via HTTP push** into the Frame demux.

| Side | Env | Example |
|------|-----|---------|
| Name / demux ref | `EVENTS_QUEUE_NAME` | `identity-profile-events` |
| Publish (topic) | `EVENTS_QUEUE_PUBLISH_URL` | `gcppubsub://my-project/identity-profile-events` |
| Subscribe (HTTP) | `EVENTS_QUEUE_SUBSCRIBE_URL` | `push://identity-profile-events` |
| Fallback both | `EVENTS_QUEUE_URL` | used when publish/subscribe overrides are empty |

1. Create a Pub/Sub topic (and optional regional `message_storage_policy`).
2. Create a **push** subscription with endpoint  
   `https://{service}/_frame/queue/{EVENTS_QUEUE_NAME}`  
   and OIDC (`FRAME_QUEUE_PUSH_AUTH=oidc`).
3. Set `EVENTS_QUEUE_PUBLISH_URL` to the topic URL and `EVENTS_QUEUE_SUBSCRIBE_URL` to `push://{name}`.
4. Do **not** use `gcppubsub://…/subscriptions/…` when the GCP subscription is push — that would open a StreamingPull loop instead of the HTTP handler.

```go
// setupEventsQueue (automatic with WithRegisterEvents) uses:
//   publisher  = GetEventsQueuePublishURL()  // gcppubsub://…
//   subscriber = GetEventsQueueSubscribeURL() // push://…
//   handlers   = events.Manager.Handler()
```

## Cloud Tasks

1. Publisher URL with ADC credentials on the service (CreateTask API OAuth2).
2. Subscriber `push://orders` with `FRAME_QUEUE_PUSH_AUTH=oidc` (optionally pin the invoker SA via `FRAME_QUEUE_PUSH_OIDC_ALLOWED_EMAILS`).
3. Task `url` = `https://…/_frame/queue/orders`.
4. Configure queue **maxAttempts** + **DLQ** (non-2xx always retry until maxAttempts).

## Push HTTP status map

| Condition | Status |
|-----------|--------|
| Success | 200 |
| Unknown / non-push ref | 404 |
| Bad method | 405 |
| Auth failure | 401 / 403 |
| Body too large | 413 |
| Decode error | 400 |
| `queue.ErrNotRetryable` | 422 |
| Handler / overload / panic | 503 |
| Handler timeout | 504 |

Permanent poison: return `fmt.Errorf("%w: …", queue.ErrNotRetryable)`. Optional `FRAME_QUEUE_PUSH_ACK_POISON=true` maps that to 200 (dangerous; off by default).

## Security

| Env | Default | Meaning |
|-----|---------|---------|
| `FRAME_QUEUE_PUSH_AUTH` | `none` | `none` \| `bearer` \| `oidc` |
| `FRAME_QUEUE_PUSH_REQUIRE_AUTH` | inherits `RUN_SERVICE_SECURELY` | fail start if push + auth=none |
| `FRAME_QUEUE_PUSH_BEARER_TOKEN` | | bearer secret |
| `FRAME_QUEUE_PUSH_OIDC_AUDIENCE` | request URL | JWT audience |
| `FRAME_QUEUE_PUSH_OIDC_ISSUERS` | Google accounts | comma-separated issuers |
| `FRAME_QUEUE_PUSH_OIDC_JWKS_URL` | Google certs | JWKS URL |
| `FRAME_QUEUE_PUSH_OIDC_ALLOWED_EMAILS` | empty | comma-separated SA emails / `sub` allowlist (empty = no principal filter) |
| `FRAME_QUEUE_PUSH_TRUST_METADATA` | `false` | allow claim-shaped metadata from push |
| `FRAME_QUEUE_PUSH_MAX_BODY_BYTES` | `1048576` | body limit |
| `FRAME_QUEUE_PUSH_HANDLER_TIMEOUT` | `0s` (no timeout) | optional hard push SLA |
| `FRAME_QUEUE_PUSH_BASE_PATH` | `/_frame/queue` | reserved mux path |

By default, push **strips** claim keys (`sub`, `tenant_id`, `roles`, …) so handlers cannot be fooled by forged headers.

OIDC for Cloud Tasks / GCP push uses a **dedicated** Google JWKS validator — not the app Hydra JWT authenticator. When `FRAME_QUEUE_PUSH_OIDC_ALLOWED_EMAILS` is set, **both** JWT `email` and `sub` are checked against the allowlist (case-insensitive); a match on **either** claim is enough. Mismatch on both → **403**.

Example (Cloud Tasks or Pub/Sub push from a known SA):

```bash
FRAME_QUEUE_PUSH_AUTH=oidc
FRAME_QUEUE_PUSH_OIDC_AUDIENCE=https://orders.example.com/_frame/queue/orders
FRAME_QUEUE_PUSH_OIDC_ALLOWED_EMAILS=tasks-invoker@my-proj.iam.gserviceaccount.com,pubsub-push@my-proj.iam.gserviceaccount.com
```

## Subscriber handlers

```go
type handler struct{}
func (h handler) Handle(ctx context.Context, metadata map[string]string, message []byte) error {
    return nil
}
```

Pull and push share the same `processDelivery` path (context enrichment policy, worker pool, metrics).

## CloudEvents ↔ Frame events

- Egress maps `frame._internal.event.header` → CE extension `frameevent`.
- Ingress maps `ce-frameevent` → `frame._internal.event.header`.
- There is **no** silent alias of `ce-type` → event name.

## Manager API

- `AddPublisher` / `AddSubscriber` / `Publish` / `Init` / `Close`
- `HasPushSubscribers` / `LookupPush`
- `Subscriber.Mode()` → pull or push

## Metrics

Subscribers expose `SubscriberMetrics` (idle / processing). Push increments active messages inside `processDeliverySync`.

## Best Practices

- Register publishers before subscribers (Frame enforces ordering).
- Keep handler work short; offload heavy work to the worker pool carefully (do not submit-and-wait pool jobs from inside a push handler job).
- Use structured metadata for trace correlation (`traceparent`).
- Prefer `push://{ref}` over raw `https://…` subscriber URLs for clarity.
- Local push testing: `FRAME_QUEUE_PUSH_REQUIRE_AUTH=false` or bearer with a dev token.
