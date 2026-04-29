# Smoke Test Prompt

Execute rapid validation tests immediately after deployment to verify critical system functionality in the target environment.

## Operational Action

This action runs post-deployment validation tests. It does not generate documentation files.

## Principles

- Run in under 5 minutes total
- Test only critical paths; skip edge cases
- Fail fast on first critical error
- Adapt to target environment (staging vs. production)
- Avoid destructive operations; clean up test artifacts

## Test Suites

### 1. Infrastructure Health (Critical)
- All pods running and ready
- All services have endpoints
- Database connectivity verified

### 2. API Health (Critical)
- `/health` returns healthy status
- `/version` returns expected version
- Database and cache connectivity via health sub-endpoints

### 3. Authentication Flow (Critical)
- Login returns valid token
- Authenticated request succeeds
- Logout invalidates token

### 4. Critical Business Flows (Critical)
- Core listing/search endpoints return data
- Key transaction endpoints respond (use dry-run/validation mode)

### 5. UI (Non-critical)
- Homepage loads within timeout
- Critical page elements present (header, nav, main, footer)
- No JavaScript console errors

### 6. External Service Integration (Non-critical)
- Payment gateway reachable (test mode)
- Email service ready
- CDN serving static assets

## Orchestration

Run test suites sequentially. If a critical suite fails, stop immediately and report failure. Non-critical failures are logged but do not block deployment.

## Success Criteria

- All critical tests pass (100% required)
- Non-critical pass rate > 80%
- Total execution < 5 minutes
- No timeouts or network errors

## Environment Configuration

Each environment needs: API URL, web URL, test user credentials, timeout, retry count. Store in environment-specific config, not hardcoded.
