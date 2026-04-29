# Write End-to-End Tests

Create E2E tests validating complete user journeys and critical business flows through the entire application stack, from the user's perspective.

## Output Location

`tests/e2e/` organized by: `critical/`, `features/`, `regression/`

## Focus Areas

Test paths that: generate revenue, acquire/retain users, handle sensitive data, represent common use cases. These are expensive but high-value tests.

## Implementation

Use Playwright (or equivalent) for cross-browser testing:

```javascript
test.describe('User Registration Flow', () => {
  test('should complete registration and access dashboard', async ({ page }) => {
    await page.goto('/register');
    await page.fill('[name="email"]', 'newuser@example.com');
    await page.fill('[name="password"]', 'SecurePass123!');
    await page.click('[type="submit"]');
    expect(page.url()).toContain('/dashboard');
    await expect(page.locator('.welcome-message')).toContainText('Welcome');
  });
});
```

## Data Management

```javascript
test.beforeEach(async () => {
  await resetDatabase();
  await seedTestData({ users: [...], products: [...] });
});
test.afterEach(async () => { await cleanupTestData(); });
```

## Additional Validation

- **Performance**: Assert page load < 3s, FCP < 1.5s
- **Accessibility**: Run axe checks, verify keyboard navigation
- **Visual regression**: Screenshot comparisons for key pages
- **Cross-browser**: Chrome, Firefox, Safari, mobile viewports

## Organization: Page Object Model

Encapsulate page interactions in reusable page objects for maintainability:

```javascript
class LoginPage {
  constructor(page) { this.page = page; }
  async login(email, password) {
    await this.page.fill('[name="email"]', email);
    await this.page.fill('[name="password"]', password);
    await this.page.click('[type="submit"]');
  }
}
```

## Key Rules

**DO**: Test from user's perspective. Use test IDs for reliable selection. Wait for elements explicitly. Focus on critical business flows.

**DON'T**: Test implementation details. Use brittle CSS selectors. Hardcode wait times. Share state between tests. Test what unit/integration tests already cover.

## Quality Checklist

- [ ] All critical user paths tested
- [ ] Cross-browser validation
- [ ] Performance thresholds verified
- [ ] Tests are stable (no flakiness)
- [ ] Test data properly managed
- [ ] Error scenarios covered
