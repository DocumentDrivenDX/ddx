# Write Security Tests

Create security tests validating authentication, authorization, data protection, and vulnerability prevention. Ensure the application resists common attacks and follows security best practices.

## Output Location

`tests/security/` organized by: `auth/`, `authz/`, `input/`, `crypto/`

## Test Categories

### Authentication
- Password complexity enforcement
- Brute force prevention (account lockout after N failures)
- Session invalidation on logout
- Session expiration after inactivity
- Session fixation prevention (new session ID after login)

### Authorization
- Unauthorized access to admin/protected endpoints returns 403
- Cross-tenant data isolation (404, not 403, for hidden resources)
- Field-level permissions (sensitive fields excluded from response)
- Privilege escalation prevention (role manipulation rejected)

### Input Validation
- **SQL injection**: Malicious payloads in all inputs -> rejected, no 500 errors, no SQL in response
- **XSS**: Script payloads sanitized in responses; CSP headers set
- **File upload**: Reject dangerous file types (.php, .exe, double extensions); prevent path traversal
- **Command injection**: Shell metacharacters rejected

### Data Protection
- Sensitive data encrypted at rest (query DB directly to verify)
- HTTPS enforced (HTTP redirects to HTTPS)
- Secure cookie flags: `Secure`, `HttpOnly`, `SameSite=Strict`
- No sensitive info in error messages

### Security Headers
```javascript
expect(headers['x-content-type-options']).toBe('nosniff');
expect(headers['x-frame-options']).toBe('DENY');
expect(headers['strict-transport-security']).toContain('max-age=');
expect(headers['x-powered-by']).toBeUndefined();
```

### CSRF Protection
State-changing operations require valid CSRF token; requests without token return 403.

### Rate Limiting
Rapid requests trigger 429 response with `x-ratelimit-*` headers.

## Key Rules

**DO**: Test with realistic attack payloads. Verify both positive and negative cases. Test authorization at multiple levels. Automate security scanning.

**DON'T**: Test against production data. Hardcode secrets in tests. Skip edge cases. Assume frameworks are secure by default.

## Quality Checklist

- [ ] Authentication mechanisms tested
- [ ] Authorization boundaries validated
- [ ] Injection prevention verified (SQL, XSS, command)
- [ ] CSRF protection tested
- [ ] Encryption at rest/transit verified
- [ ] Security headers checked
- [ ] Rate limiting tested
- [ ] Error messages don't leak information
- [ ] No known dependency vulnerabilities
