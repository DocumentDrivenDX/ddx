# Write Performance Tests

Create performance tests validating system responsiveness, scalability, and resource efficiency under various load conditions.

## Output Location

`tests/performance/` organized by: `load/`, `stress/`, `spike/`, `endurance/`

## Test Types

- **Load**: Normal conditions (expected users, transaction rates, data volumes)
- **Stress**: Find breaking points (max users, peak rates, resource exhaustion, recovery)
- **Spike**: Sudden load increases (flash sales, viral content, traffic surges)
- **Endurance**: Extended stability (memory leaks, resource degradation, connection pooling)

## Performance Budgets

```javascript
const budgets = {
  api: { p50: 100, p95: 500, p99: 1000, max: 3000 },  // ms
  web: { FCP: 1500, TTI: 3000, LCP: 2500, TBT: 300 }, // ms
  throughput: { reads: 10000, writes: 1000, searches: 500 } // req/sec
};
```

## Load Test Example (k6)

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '2m', target: 100 },
    { duration: '5m', target: 100 },
    { duration: '2m', target: 200 },
    { duration: '5m', target: 200 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
    errors: ['rate<0.01'],
  },
};

export default function () {
  const res = http.get('https://api.example.com/products');
  check(res, { 'status 200': (r) => r.status === 200 });
  sleep(1);
}
```

## What to Measure

- **Response time**: p50, p95, p99, max (not just averages)
- **Throughput**: Requests/sec for read and write operations
- **Resources**: CPU, memory, connections, disk I/O
- **Frontend**: Core Web Vitals (FCP, LCP, CLS, TTFB)
- **Database**: Query time p95, transactions/sec

## Key Rules

**DO**: Test in production-like environments. Use realistic data volumes. Include think time. Test read and write operations. Automate regression testing.

**DON'T**: Test against production. Use unrealistic data. Ignore warmup periods. Focus only on happy paths or max load. Skip endurance testing.

## Quality Checklist

- [ ] Load testing covers expected traffic patterns
- [ ] Stress testing identifies breaking points
- [ ] Response time targets validated
- [ ] Resource usage within limits
- [ ] Memory leaks tested
- [ ] Database and API performance validated
- [ ] Frontend meets Core Web Vitals
- [ ] Performance regression tests automated
