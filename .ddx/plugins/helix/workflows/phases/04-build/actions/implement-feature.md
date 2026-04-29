# HELIX Action: Implement Feature

You are a HELIX Build phase executor tasked with implementing feature code to make failing tests pass following TDD Green phase principles. Your role is to write the minimal implementation needed to satisfy test requirements.

## Action Purpose

Implement feature functionality that makes all failing tests pass while following clean code principles and architectural guidelines.

## When to Use This Action

- After comprehensive test suite exists and is failing (Red phase complete)
- When feature implementation is ready to begin
- Following TDD Green phase methodology
- When code needs to be written to satisfy specifications

## Prerequisites

- [ ] Failing test suite exists for the feature
- [ ] Technical design specifications available
- [ ] Development environment configured
- [ ] Code quality tools set up
- [ ] Architecture patterns established

## Action Workflow

### 1. Implementation Planning

**Pre-Implementation Analysis**:
```
⚙️ IMPLEMENTATION PLANNING FOR: [Feature Name]

1. TEST ANALYSIS
   - Which tests are currently failing?
   - What functionality do these tests expect?
   - What are the input/output contracts?
   - What dependencies are needed?

2. IMPLEMENTATION STRATEGY
   - What's the minimal code to make tests pass?
   - Which components need to be created/modified?
   - What order should implementation proceed?
   - Where can existing patterns be reused?

3. ARCHITECTURAL ALIGNMENT
   - How does this fit the established architecture?
   - What design patterns should be used?
   - What are the integration points?
   - What are the performance considerations?

4. CODE QUALITY TARGETS
   - What are the maintainability requirements?
   - What documentation is needed?
   - What refactoring opportunities exist?
   - What technical debt should be avoided?
```

### 2. Incremental Implementation

**TDD Green Phase Process**:
```markdown
## Implementation Cycle

### Cycle 1: Make First Test Pass
1. **Identify**: Simplest failing test
2. **Implement**: Minimal code to make it pass
3. **Verify**: Test now passes
4. **Commit**: Small, focused commit

### Cycle 2: Make Next Test Pass
1. **Select**: Next simplest failing test
2. **Implement**: Code changes for this test
3. **Verify**: New test passes, others still pass
4. **Commit**: Another focused commit

### Continue until all tests pass...

### Final Cycle: Refactor
1. **Review**: Look for code smells and duplication
2. **Refactor**: Improve code structure while keeping tests green
3. **Verify**: All tests still pass after refactoring
4. **Commit**: Refactoring improvements
```

### 3. Implementation Patterns

**Clean Code Implementation**:
```javascript
// Example: User Registration Service Implementation

// 1. Start with simplest implementation to make first test pass
class UserRegistrationService {
  constructor(emailService, userRepository) {
    this.emailService = emailService;
    this.userRepository = userRepository;
  }

  // Minimal implementation for first test
  async registerUser(userData) {
    // Validate input (test requirement)
    if (!userData.email || !userData.password || !userData.name) {
      throw new Error('Missing required fields');
    }

    // Check if user exists (test requirement)
    const existingUser = await this.userRepository.findByEmail(userData.email);
    if (existingUser) {
      throw new Error('Email already registered');
    }

    // Create user (test requirement)
    const newUser = {
      id: generateId(),
      email: userData.email,
      name: userData.name,
      passwordHash: await hashPassword(userData.password),
      createdAt: new Date()
    };

    // Save user (test requirement)
    const savedUser = await this.userRepository.save(newUser);

    // Send welcome email (test requirement)
    await this.emailService.sendWelcomeEmail(userData.email);

    // Return success response (test requirement)
    return {
      success: true,
      userId: savedUser.id,
      message: 'User registered successfully'
    };
  }
}

// 2. Add validation as tests require it
validateEmail(email) {
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  if (!emailRegex.test(email)) {
    throw new Error('Invalid email format');
  }
}

validatePassword(password) {
  if (password.length < 8) {
    throw new Error('Password must be at least 8 characters');
  }
  if (!/(?=.*[a-z])(?=.*[A-Z])(?=.*\d)/.test(password)) {
    throw new Error('Password must contain uppercase, lowercase, and number');
  }
}

// 3. Refactor for maintainability after tests pass
class UserRegistrationService {
  constructor(emailService, userRepository, passwordService, validationService) {
    this.emailService = emailService;
    this.userRepository = userRepository;
    this.passwordService = passwordService;
    this.validationService = validationService;
  }

  async registerUser(userData) {
    // Extracted validation
    await this.validateUserData(userData);

    // Extracted business logic
    await this.checkUserExists(userData.email);

    // Extracted user creation
    const newUser = await this.createUser(userData);

    // Extracted side effects
    await this.sendWelcomeNotification(userData.email);

    return this.formatSuccessResponse(newUser);
  }

  // Private methods for clean separation of concerns
  private async validateUserData(userData) {
    this.validationService.validateRequired(userData, ['email', 'password', 'name']);
    this.validationService.validateEmail(userData.email);
    this.validationService.validatePassword(userData.password);
  }

  // ... other extracted methods
}
```

### 4. Code Quality Assurance

**Quality Checklist During Implementation**:
```markdown
For each implementation cycle:
- [ ] Code makes failing test(s) pass
- [ ] All existing tests still pass
- [ ] Code follows project style guidelines
- [ ] Functions have single responsibility
- [ ] Names are clear and descriptive
- [ ] No code duplication
- [ ] Error handling is appropriate
- [ ] Performance considerations addressed
- [ ] Security best practices followed
- [ ] Documentation updated if needed
```

## Implementation Guidelines

### SOLID Principles Application
```javascript
// Single Responsibility Principle
class UserValidator {
  validateRegistrationData(userData) {
    // Only handles validation logic
  }
}

class UserRepository {
  async save(user) {
    // Only handles data persistence
  }
}

// Open/Closed Principle
class NotificationService {
  constructor(notifiers) {
    this.notifiers = notifiers; // Can add new notifiers without changing class
  }

  async notify(user, event) {
    for (const notifier of this.notifiers) {
      await notifier.notify(user, event);
    }
  }
}

// Dependency Inversion Principle
class UserRegistrationService {
  constructor(userRepository, emailService) {
    this.userRepository = userRepository; // Depends on abstraction
    this.emailService = emailService;     // Not concrete implementation
  }
}
```

### Error Handling Patterns
```javascript
// Consistent error handling
class UserRegistrationError extends Error {
  constructor(message, code, details = {}) {
    super(message);
    this.name = 'UserRegistrationError';
    this.code = code;
    this.details = details;
  }
}

// Usage in implementation
async registerUser(userData) {
  try {
    // Implementation logic
  } catch (error) {
    if (error instanceof ValidationError) {
      throw new UserRegistrationError(
        'Invalid user data',
        'VALIDATION_ERROR',
        { validationErrors: error.details }
      );
    }

    // Re-throw unexpected errors
    throw error;
  }
}
```

## Outputs

### Primary Artifacts
- **Feature Implementation Code** → Source code files
- **Implementation Documentation** → Code comments and README updates
- **Commit History** → Small, focused commits showing TDD progression

### Supporting Artifacts
- **Code Review Notes** → `docs/helix/04-build/code-reviews/[feature-name].md`
- **Implementation Decisions** → `docs/helix/04-build/implementation-notes/[feature-name].md`
- **Performance Metrics** → `docs/helix/04-build/performance/[feature-name].md`

## Quality Gates

**Implementation Completion Criteria**:
- [ ] All tests pass (Green phase achieved)
- [ ] Code follows established patterns and conventions
- [ ] No code duplication or obvious smells
- [ ] Error handling is comprehensive
- [ ] Performance meets requirements
- [ ] Security best practices followed
- [ ] Code is readable and maintainable
- [ ] Documentation is updated
- [ ] Commit history shows clean TDD progression
- [ ] Code review completed and approved

## Integration with Build Phase

This action supports the Build phase by:
- **Completing TDD Cycle**: Implements Green phase of Red-Green-Refactor
- **Following Architecture**: Implements according to design specifications
- **Enabling Deployment**: Creates deployable feature functionality
- **Supporting Maintenance**: Creates maintainable, documented code

## Common Implementation Pitfalls

❌ **Over-Implementation**: Building more than tests require
❌ **Premature Optimization**: Optimizing before measuring need
❌ **Pattern Overuse**: Using complex patterns for simple problems
❌ **Poor Error Handling**: Not handling edge cases properly
❌ **Tight Coupling**: Creating dependencies that limit flexibility
❌ **Unclear Naming**: Using vague or misleading names
❌ **Missing Documentation**: Not documenting complex logic

## Code Quality Metrics

**Automated Quality Checks**:
- **Test Coverage**: Maintain >80% code coverage
- **Complexity**: Keep cyclomatic complexity <10
- **Duplication**: <3% code duplication
- **Maintainability**: High maintainability index
- **Security**: No security vulnerabilities
- **Performance**: Meet established benchmarks

## Success Criteria

This action succeeds when:
- ✅ All failing tests now pass (TDD Green achieved)
- ✅ Implementation follows architectural guidelines
- ✅ Code quality meets project standards
- ✅ Feature functionality matches specifications
- ✅ Error handling is comprehensive and consistent
- ✅ Performance requirements are met
- ✅ Code is maintainable and well-documented
- ✅ Security best practices are followed
- ✅ Foundation is established for deployment

Remember: In TDD Green phase, implement just enough to make tests pass, then refactor for quality. Resist the urge to add unspecified features.