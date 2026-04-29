# Practices: Observability (OpenTelemetry)

## Requirements (Frame phase)

- All services must define SLOs for availability and latency
- Incident response requires structured logs and traces

## Design

- Use OpenTelemetry SDK for instrumentation (not vendor-specific SDKs)
- All cross-service calls propagate W3C Trace Context headers
- Define span naming conventions per service type
- Metric names follow OpenTelemetry semantic conventions

## Implementation

- Structured JSON logging — no unstructured text logs in production
- Every HTTP handler: request ID, trace ID, duration, status code
- Every database query: duration, table, operation type
- Every external API call: duration, endpoint, status code
- Error logs include stack trace and request context
- Use log levels consistently: ERROR (actionable), WARN (degraded),
  INFO (business events), DEBUG (development only)

## Testing

- Verify trace propagation in integration tests
- Verify structured log format in unit tests
- Load test with tracing enabled to validate overhead < 5%
- Alert on missing trace context in production logs

## Deployment

- Configure OTEL collector as a sidecar or daemonset
- Export to the project's observability backend (Grafana, Datadog, etc.)
- Set sampling rate appropriate to traffic volume
