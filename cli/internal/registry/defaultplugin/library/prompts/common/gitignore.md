# .gitignore Evaluation and Creation Guide

Evaluate and enhance the .gitignore file in this project following this comprehensive checklist.

## Goal
Create a .gitignore that:
- Prevents sensitive, temporary, and generated files from entering version control
- Maintains a clean repository without unnecessary bloat
- Enables efficient collaboration while preserving necessary files
- Follows platform and language-specific best practices

## Evaluation Checklist

### 1. Operating System Files
Ensure these OS-specific files are ignored:
- **macOS**: `.DS_Store`, `.AppleDouble`, `.LSOverride`, `Icon\r`, `._*`, `.Spotlight-V100`, `.Trashes`
- **Windows**: `Thumbs.db`, `ehthumbs.db`, `Desktop.ini`, `$RECYCLE.BIN/`, `*.lnk`
- **Linux**: `*~`, `.directory`, `.Trash-*`, `.nfs*`

### 2. IDE and Editor Files
Check for common development environment artifacts:
- **JetBrains IDEs**: `.idea/`, `*.iml`, `*.iws`, `*.ipr`, `out/`
- **Visual Studio Code**: `.vscode/`, `*.code-workspace`, `.history/`
- **Visual Studio**: `.vs/`, `*.suo`, `*.user`, `*.userosscache`, `*.sln.docstates`
- **Vim/Emacs**: `*.swp`, `*.swo`, `*~`, `.*.sw[a-z]`, `[._]*.s[a-v][a-z]`, `\#*\#`, `.\#*`
- **Sublime Text**: `*.sublime-project`, `*.sublime-workspace`
- **Others**: `.project`, `.classpath`, `.settings/`, `*.tmproj`

### 3. Language/Framework Specific

#### Go Projects
- Build artifacts: Binary names, `*.exe`, `*.dll`, `*.so`, `*.dylib`
- Test files: `*.test`, `*.out`, `coverage.*`
- Dependencies: `vendor/` (if not committing vendor)
- Workspace: `go.work`, `go.work.sum`

#### Node.js/JavaScript
- Dependencies: `node_modules/`, `jspm_packages/`
- Build outputs: `dist/`, `build/`, `.next/`, `out/`
- Logs: `npm-debug.log*`, `yarn-debug.log*`, `yarn-error.log*`, `lerna-debug.log*`
- Runtime: `.npm`, `.yarn`, `.pnp.*`
- Testing: `coverage/`, `.nyc_output/`

#### Python
- Byte-compiled: `__pycache__/`, `*.py[cod]`, `*$py.class`
- Distribution: `dist/`, `build/`, `*.egg-info/`, `*.egg`
- Virtual environments: `venv/`, `env/`, `ENV/`, `.venv`
- Testing: `.pytest_cache/`, `.coverage`, `htmlcov/`, `.tox/`
- Type checking: `.mypy_cache/`, `.pyre/`, `.pytype/`

#### Java
- Compiled: `*.class`, `target/`, `out/`
- Package files: `*.jar`, `*.war`, `*.ear`
- Build tools: `.gradle/`, `.mvn/`, `gradle-app.setting`

#### Ruby
- Dependencies: `/.bundle/`, `/vendor/bundle`
- Documentation: `/.yardoc`, `/_yardoc/`, `/doc/`, `/rdoc/`
- Testing: `/coverage/`, `/spec/reports/`, `/test/tmp/`

#### Rust
- Build: `/target/`, `Cargo.lock` (for libraries)
- Debug: `*.pdb`

### 4. Security and Credentials
Critical files that must never be committed:
- Environment files: `.env`, `.env.*`, `!.env.example`, `!.env.template`
- Credentials: `*.pem`, `*.key`, `*.cert`, `*.p12`
- Secrets: `secrets.yml`, `secrets.json`, `credentials.json`
- Cloud configs: `*.tfvars`, `kubeconfig`, `.aws/`, `.gcp/`
- SSH keys: `id_rsa`, `id_dsa`, `*.ppk`
- API tokens: `.npmrc` (with tokens), `.pypirc`, `.netrc`

### 5. Logs and Temporary Files
- Logs: `*.log`, `logs/`, `*.log.*`
- Temporary: `*.tmp`, `*.temp`, `tmp/`, `temp/`, `cache/`
- Backups: `*.bak`, `*.backup`, `*.old`, `~*`
- Core dumps: `core`, `core.*`, `*.stackdump`

### 6. Build and Package Files
- Archives: `*.zip`, `*.tar.gz`, `*.rar`, `*.7z` (unless intentional)
- Installers: `*.dmg`, `*.iso`, `*.msi`, `*.pkg`, `*.deb`, `*.rpm`
- Compiled docs: `*.pdf` (if generated), `*.chm`

### 7. Project-Specific Patterns
Consider these based on project needs:
- Database files: `*.sqlite`, `*.db`, `*.sqlite3`
- Media files: Large binaries that should use LFS instead
- Generated documentation: `docs/_build/`, `site/`
- CI/CD artifacts: `.cache/`, `artifacts/`
- Container files: `*.pid`, `.dockerignore` conflicts

### 8. Best Practices

#### Pattern Organization
1. Group related patterns with comments
2. Order from most specific to least specific
3. Use directory markers (`/`) for clarity
4. Separate concerns (OS, IDE, language, project)

#### Negation Patterns
- Use `!` to include specific files within ignored directories
- Place negations after the patterns they override
- Example: `build/`, `!build/.gitkeep`

#### Testing Patterns
- Use `git check-ignore <path>` to test if a file would be ignored
- Run `git status --ignored` to see currently ignored files
- Check `git ls-files --others --ignored --exclude-standard`

#### Common Pitfalls to Avoid
- **Over-ignoring**: Don't ignore configuration examples or templates
- **Under-ignoring**: Missing platform-specific files
- **Wrong syntax**: Forgetting trailing `/` for directories
- **No comments**: Future maintainers won't understand reasoning
- **Committed files**: Already tracked files won't be ignored
- **Case sensitivity**: Git is case-sensitive, be consistent

### 9. Validation Steps

1. **Audit existing repository**:
   ```bash
   # Find large files that might need ignoring
   git rev-list --objects --all | git cat-file --batch-check='%(objecttype) %(objectname) %(objectsize) %(rest)' | awk '/^blob/ {print substr($0,6)}' | sort -n -k 2 | tail -20
   
   # Check for sensitive patterns
   git grep -E "(password|secret|token|key|api)" --cached
   ```

2. **Test ignore patterns**:
   ```bash
   # Test if a file would be ignored
   git check-ignore -v <filename>
   
   # Show all ignored files
   git status --ignored
   ```

3. **Clean up already committed files**:
   ```bash
   # Remove files that should have been ignored
   git rm --cached <file>
   git commit -m "Remove accidentally committed files"
   ```

### 10. Template Resources
Consider starting from or comparing with:
- GitHub's gitignore templates: https://github.com/github/gitignore
- gitignore.io for custom combinations
- Language/framework specific documentation

## Final Review Questions
- [ ] Are all developer environment files ignored?
- [ ] Are credentials and secrets protected?
- [ ] Are build artifacts and dependencies handled correctly?
- [ ] Is the file well-commented and organized?
- [ ] Have you tested the patterns work as expected?
- [ ] Are there any project-specific files that need ignoring?
- [ ] Are necessary config examples preserved with negation patterns?
