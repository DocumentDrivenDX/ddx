# Concern: Observability (OpenTelemetry)

## Category
observability

## Areas
api, backend, infra

## Components

- **Standard**: OpenTelemetry (traces, metrics, logs)
- **Traces**: Distributed tracing with correlation IDs
- **Metrics**: RED metrics (Rate, Errors, Duration) for all services
- **Logs**: Structured JSON logging with trace context

## Constraints

- All HTTP/gRPC endpoints must emit latency and error metrics
- All cross-service calls must propagate trace context
- Logs must be structured JSON with correlation IDs
- No `console.log` / `print` for operational logging

## When to use

Any project with backend services, APIs, or distributed systems. Essential
for production debugging, performance monitoring, and incident response.

## ADR References
