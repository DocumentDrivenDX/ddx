# Deploy to Staging Prompt

Execute a complete deployment to staging, including building, deploying, and validating the application in a production-like environment.

## Operational Action

This action performs staging deployment and validation. It does not generate documentation files.

## Deployment Steps

### 1. Pre-Deployment
- Verify staging environment health (nodes, pods, database, external services)
- Build application and Docker image, push to registry
- Verify required secrets exist

### 2. Database Migration
- Backup current staging database
- Dry-run migration, then execute
- Verify migration success

### 3. Deploy
- Apply Kubernetes deployment (rolling update, maxSurge=1, maxUnavailable=0)
- Watch rollout status until complete
- Verify all pods running with correct version

### 4. Post-Deployment Validation
- Health check all endpoints
- Verify deployed version matches expected
- Run smoke tests (auth, core functionality, external integrations)

### 5. Performance Validation
- Run load test (ramp to target RPS, sustain, ramp down)
- Verify: p95 < 500ms, error rate < 0.1%

### 6. Monitoring Verification
- Confirm metrics collection is working
- Test alert firing

### 7. UAT
- Enable test users and feature flags
- Validate: login/logout, core workflows, admin functions, payments, email, mobile

### 8. Rollback Testing
- Deploy previous version, verify rollback works, restore current version

## Success Criteria
- [ ] All containers healthy, health checks passing
- [ ] Smoke tests and performance tests passing
- [ ] Monitoring data flowing, no critical alerts
- [ ] UAT sign-off received

## Troubleshooting
- **Pods not starting**: `kubectl describe pod`, check logs with `--previous`, check resources
- **Database failures**: Test connectivity, check secrets
- **Performance issues**: Check resource usage, slow queries, cache hit rates
