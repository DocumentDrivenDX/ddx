# Configure Monitoring Prompt

Set up monitoring and observability infrastructure to track application health, performance, and business metrics in real-time.

## Operational Action

This action configures monitoring systems -- dashboards, alerts, and metrics collection. It does not generate documentation files.

## Setup Checklist

### 1. Metrics Collection
- Configure Prometheus (or equivalent) to scrape application and node metrics
- Instrument application code with RED metrics (request rate, errors, duration)
- Add business metrics (orders, revenue, conversions) as custom counters/histograms
- Expose `/metrics` endpoint with proper labels

### 2. Logging Infrastructure
- Configure centralized log aggregation (Fluentd/ELK or equivalent)
- Ensure structured JSON logging with: timestamp, level, service, trace_id, message
- Set log retention policies per environment

### 3. Distributed Tracing
- Deploy trace collector (Jaeger/Zipkin or equivalent)
- Instrument application with OpenTelemetry spans for critical operations
- Configure sampling: 100% errors, 1-10% normal traffic

### 4. Dashboards
- **Operations**: Request rate, error rate, p95 response time, active users
- **Business**: Key conversion and revenue metrics
- Set up Grafana (or equivalent) with auto-refresh

### 5. Alert Rules
- Critical: High error rate (>1% for 5m), pod crash loops, data loss risk
- Warning: High latency (p95 >1s for 10m), low business metrics
- Route alerts to appropriate on-call channels with severity labels

### 6. Health Check Endpoints
- `/health` endpoint checking database, cache, and critical dependencies
- Return degraded status (503) when dependencies fail

### 7. Verification
- Confirm metrics flowing to dashboards
- Test alert firing and notification delivery
- Run load test to validate monitoring under stress

## Maintenance
- Tune alert thresholds weekly based on observed patterns
- Archive old logs monthly
- Prune unused metrics to control cardinality and cost
