// Package ratelimiter provides admission-control primitives for protecting
// services from overload.
//
// The package intentionally contains multiple limiter types because overload is
// not one single problem. Each limiter answers a different operational
// question:
//
//   - WindowLimiter:
//     "Has this shared key already consumed too many requests in this window?"
//
//   - LeasedWindowLimiter:
//     "Same as WindowLimiter, but can we answer it without turning the cache
//     backend into the bottleneck at very high request volume?"
//
//   - ConcurrencyLimiter:
//     "Do we already have too many expensive operations running at the same
//     time in this process?"
//
//   - QueueDepthLimiter:
//     "Is downstream backlog already high enough that accepting more work would
//     be unsafe?"
//
// Choosing the correct limiter starts with identifying the first resource that
// fails in production:
//
//   - If a tenant or API key can send too many requests in a minute, use
//     WindowLimiter or LeasedWindowLimiter.
//
//   - If connector calls, CPU-heavy work, or database-heavy handlers collapse
//     when too many run in parallel, use ConcurrencyLimiter.
//
//   - If the real danger is backlog growth in a queue, outbox, or scheduler
//     pipeline, use QueueDepthLimiter.
//
// These limiters are not interchangeable:
//
//   - Do not use ConcurrencyLimiter for fleet-wide fairness. It is process
//     local. Ten replicas with a limit of 100 each permit roughly 1,000
//     concurrent operations fleet-wide.
//
//   - Do not use QueueDepthLimiter as a generic API rate limiter. It does not
//     shape traffic over time. It only admits or rejects based on observed
//     backlog.
//
//   - Do not use WindowLimiter or LeasedWindowLimiter to protect a resource
//     whose failure mode is in-flight saturation. Request-count limits do not
//     prevent a single expensive class of work from exhausting a process.
//
// The usual production composition is layered rather than exclusive:
//
//  1. WindowLimiter or LeasedWindowLimiter at the ingress boundary for
//     fairness and abuse protection.
//  2. QueueDepthLimiter before enqueue or ingest when backlog itself is the
//     overload signal.
//  3. ConcurrencyLimiter around expensive local execution such as worker
//     dispatch, connector calls, or CPU-heavy transforms.
//
// In other words:
//
//   - request budgets protect fairness,
//   - queue-depth admission protects drainability,
//   - concurrency caps protect finite execution capacity.
//
// If the service needs all three protections, use all three protections. They
// solve different failure modes.
package ratelimiter
