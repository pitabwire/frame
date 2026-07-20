# queue-push example

Demonstrates Frame queue **HTTP push** multiplexing alongside classic `mem://` pull.

## Run

```bash
cd examples/queue-push
go run .
```

## Push a message

```bash
curl -sS -X POST http://localhost:8080/_frame/queue/events \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello push"}'
```

CloudEvents binary:

```bash
curl -sS -X POST http://localhost:8080/_frame/queue/events \
  -H 'Ce-Specversion: 1.0' \
  -H 'Ce-Id: demo-1' \
  -H 'Ce-Type: com.example.demo' \
  -H 'Ce-Source: //local' \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello ce"}'
```

## Knative

Point a Trigger at `http://<service>/_frame/queue/events` and register the subscriber as `push://events`. See [docs/queue.md](../../docs/queue.md).
