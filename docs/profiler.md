# Profiler (pprof)

Frame includes a lightweight pprof server controlled by configuration.

## Enable

Set:

- `PROFILER_ENABLE=true`
- `PROFILER_PORT=:6060`

Frame starts the profiler during service startup and stops it during shutdown.

## Usage

```
http://localhost:6060/debug/pprof/
```

## Best Practices

- Enable only in trusted environments.
- Restrict access via network policy or authentication.
- Disable in public production networks unless secured.
