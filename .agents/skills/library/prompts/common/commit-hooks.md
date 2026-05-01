# Git Commit Hooks Implementation Assistant

You are an expert DevOps engineer specializing in git hooks, CI/CD, and developer tooling. Help me implement a comprehensive git hooks system that ensures code quality while maintaining excellent developer experience.

## Task Overview
Analyze my repository and implement a production-ready git hooks system that:
- Prevents problematic code from entering the repository
- Maintains consistent code quality standards
- Works seamlessly across different development environments
- Integrates with existing development workflows

## Step-by-Step Approach

### Phase 1: Repository Analysis
First, examine my repository to understand:
```
1. Run: find . -type f -name "*.go" -o -name "*.js" -o -name "*.py" -o -name "*.rs" | head -20
2. Check: ls -la .git/hooks/ 
3. Look for: package.json, go.mod, Cargo.toml, pyproject.toml, Makefile
4. Identify: .gitignore patterns, CI configuration files
5. Examine: Any existing hook scripts or configurations
```

Based on this analysis, identify:
- Primary language(s) and their versions
- Package managers and build tools
- Testing frameworks in use
- Current code quality tools
- Team workflow patterns

### Phase 2: Framework Selection

Evaluate frameworks against these criteria (rate each 1-10):

| Criteria        | Weight | Description                                        |
| --------------- | ------ | -------------------------------------------------- |
| Cross-platform  | 25%    | Native Windows/Mac/Linux support without emulation |
| Performance     | 20%    | Execution speed, especially for large codebases    |
| Dependencies    | 20%    | Fewer runtime dependencies = better                |
| Ecosystem       | 15%    | Available pre-built hooks and community            |
| Maintainability | 10%    | Ease of configuration and updates                  |
| Developer UX    | 10%    | Clear errors, easy overrides, good documentation   |
|                 |        |                                                    |

Consider these frameworks:
1. **Lefthook** (Go) - Single binary, parallel execution
2. **Pre-commit** (Python) - Large ecosystem, requires Python
3. **Husky** (Node.js) - Popular for JS projects, requires Node
4. **Git-hooks** (Shell) - No dependencies, platform limitations
5. **Prek** (Rust) - Fast, pre-commit compatible, newer

### Phase 3: Implementation Requirements

Design hooks for these scenarios:

#### Pre-commit Hooks (must be fast, <3 seconds ideal)
- [ ] **Conflict markers**: Block commits with merge conflicts
- [ ] **Secrets detection**: Prevent API keys, tokens, passwords
- [ ] **Large files**: Warn/block files over size threshold
- [ ] **Formatting**: Auto-fix code style issues
- [ ] **Linting**: Catch code quality issues
- [ ] **Filename validation**: Enforce naming conventions

#### Pre-push Hooks (can be slower, <30 seconds)
- [ ] **Type checking**: Full type validation
- [ ] **Tests**: Run affected unit tests
- [ ] **Build verification**: Ensure code compiles
- [ ] **Documentation**: Validate docs are updated
- [ ] **Dependencies**: Check for security vulnerabilities

### Phase 4: Configuration Generation

Create configuration that:
1. **Supports staged files only** - Use `{staged_files}` or equivalent
2. **Runs in parallel** - Maximize performance
3. **Allows selective disable** - Via environment variables
4. **Provides clear output** - What failed and why
5. **Auto-fixes when possible** - With `stage_fixed: true` or equivalent

Example structure:
```yaml
# Tool-specific configuration
pre-commit:
  parallel: true
  commands:
    # Fast checks first
    syntax-check:
      priority: 1
      glob: "**/*.{ext}"
      run: command {staged_files}
    
    # Auto-fixable issues
    format:
      priority: 2
      glob: "**/*.{ext}"
      run: formatter --fix {staged_files}
      stage_fixed: true
    
    # Validation
    lint:
      priority: 3
      run: linter {staged_files}
```

### Phase 5: Custom Scripts

For checks without native tools, provide scripts that:
- Work with POSIX sh for maximum compatibility
- Have fallbacks for Windows (or PowerShell alternatives)
- Exit with clear error messages
- Support `--help` flags
- Can run independently for testing

Template:
```bash
#!/usr/bin/env sh
# Description: [What this check does]
# Usage: script.sh [files...]
# Exit codes: 0=success, 1=failure, 2=skip

set -e

# Cross-platform compatibility
if [ -z "$1" ]; then
    echo "Usage: $0 [files...]"
    exit 2
fi

# Main check logic
for file in "$@"; do
    # Check implementation
done
```

### Phase 6: Rollout Strategy

Provide a phased deployment plan:

**Week 1: Pilot**
- Install on 1-2 developer machines
- Run in warning-only mode
- Collect feedback on false positives

**Week 2: Refinement**
- Adjust rules based on feedback
- Add project-specific customizations
- Document override procedures

**Week 3: Team Adoption**
- Team-wide installation
- Training session / documentation
- Support channel for issues

**Week 4: Enforcement**
- Enable on CI/CD pipeline
- Make hooks required for merges
- Monitor metrics (bypass frequency, failure rates)

## Output Requirements

Structure your response as:

```markdown
# Git Hooks Implementation for [Project Name]

## Executive Summary
[2-3 sentences summarizing the recommendation]

## Repository Analysis
- **Language**: [Primary language and version]
- **Build System**: [Tools and commands]
- **Test Framework**: [Testing tools]
- **Current Hooks**: [Existing setup if any]
- **Team Environment**: [OS distribution, tool versions]

## Recommended Solution: [Framework Name]

### Why This Framework
[Bullet points with specific advantages for this project]

### Performance Benchmarks
- Formatting check: X ms
- Linting: X ms  
- Tests: X seconds
- Total pre-commit: X seconds

## Installation Guide

### Prerequisites
\`\`\`bash
# Commands to install framework
\`\`\`

### Configuration Files

#### [framework-config.yml]
\`\`\`yaml
# Complete configuration file
\`\`\`

#### Custom Scripts
\`\`\`bash
# Any custom check scripts needed
\`\`\`

### Setup Commands
\`\`\`bash
# Step-by-step setup commands
\`\`\`

## Usage Guide

### Daily Workflow
\`\`\`bash
# Common commands developers will use
\`\`\`

### Bypassing Hooks
\`\`\`bash
# How to skip hooks when necessary
\`\`\`

### Troubleshooting

| Issue | Solution |
|-------|----------|
| [Common problem 1] | [Fix] |
| [Common problem 2] | [Fix] |

## Integration

### CI/CD Pipeline
\`\`\`yaml
# CI configuration additions
\`\`\`

### IDE Setup
- [IDE 1]: [Configuration]
- [IDE 2]: [Configuration]

## Metrics & Monitoring
- Hook execution times
- Bypass frequency
- Most common failures

## Next Steps
1. [ ] Install framework locally
2. [ ] Test with sample commits
3. [ ] Customize for project needs
4. [ ] Roll out to team
5. [ ] Add to CI pipeline
```

## Additional Context for Claude

When analyzing the repository:
- Use multiple tools to gather information (grep, find, read, glob)
- Check for both explicit and implicit patterns
- Consider the team's expertise level
- Account for existing workflows that shouldn't be disrupted

When making recommendations:
- Provide specific version numbers for all tools
- Include command examples with real file paths from the repo
- Explain trade-offs explicitly
- Suggest incremental adoption paths
- Consider both senior and junior developer perspectives

Common pitfalls to address:
- Windows line ending issues (CRLF vs LF)
- File permission changes
- Symbolic links handling  
- Submodule considerations
- Binary file detection
- Network-dependent checks
- Docker/container considerations

Remember: The goal is to improve code quality without frustrating developers. Every added check should provide clear value and actionable feedback.