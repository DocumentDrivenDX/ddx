# Obsidian Integration Tools

Tools for converting DDx markdown files to Obsidian-compatible format with frontmatter and wikilinks.

## Installation

```bash
# Apply the Obsidian tools to your project
ddx apply tools/obsidian
```

## Usage

The Obsidian tools provide scripts for:

### 1. Migration to Obsidian Format

Convert standard markdown files to Obsidian format with appropriate frontmatter:

```bash
./tools/obsidian/migrate.sh [path]
```

Features:
- Adds frontmatter based on file type (feature, phase, workflow, etc.)
- Converts markdown links to wikilinks
- Preserves existing content
- Supports dry-run mode

### 2. Validation

Check if files are Obsidian-compliant:

```bash
./tools/obsidian/validate.sh [path]
```

### 3. Navigation Hub Generation

Generate an Obsidian navigation hub for HELIX workflow:

```bash
./tools/obsidian/generate-nav.sh
```

### 4. Revert to Standard Markdown

Convert Obsidian format back to standard markdown:

```bash
./tools/obsidian/revert.sh [path]
```

## File Type Detection

The tools automatically detect file types based on:

- **Features**: Files matching `FEAT-XXX` pattern
- **Phases**: README.md files in phase directories
- **Workflows**: coordinator.md, enforcer.md files
- **Templates**: template.md files
- **Prompts**: prompt.md files

## Frontmatter Generation

Different file types receive appropriate frontmatter:

### Feature Files
```yaml
---
id: FEAT-001-user-authentication
title: User Authentication
type: feature-specification
status: draft
priority: p1
owner: security-team
tags:
  - feature
  - authentication
  - security
created: 2024-01-15
updated: 2024-01-15
---
```

### Phase Files
```yaml
---
id: frame-phase
title: Frame Phase - Problem Definition
type: workflow-phase
phase: frame
status: active
tags:
  - helix
  - workflow
  - frame
created: 2024-01-15
updated: 2024-01-15
---
```

## Implementation Note

These tools were originally part of the DDx CLI but have been moved to the library to maintain CLI minimalism and follow the "Extensibility Through Composition" principle. The functionality is now available as optional tools that can be applied when needed rather than being built into the core CLI.

## Related

- [HELIX Workflow](../../workflows/helix/README.md)
- [DDx Templates](../../templates/README.md)
- [Contributing Guide](../../../CONTRIBUTING.md)