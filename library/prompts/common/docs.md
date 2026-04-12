# Documentation Creation and Maintenance Guide

Evaluate, create, and maintain project documentation following these comprehensive guidelines.

## Core Principles

### 1. Search Before Creating
**CRITICAL**: Never create new documentation without first thoroughly searching for existing content.
- Search across all documentation folders for related topics
- Check for similar concepts using different terminology
- Look for partial documentation that can be enhanced
- Review related files that might contain relevant information
- If found, enhance existing docs rather than creating duplicates

### 2. Never Archive
**IMPORTANT**: Never move content to archive folders or mark as deprecated.
- All documentation evolves - update in place
- Historical context is valuable - preserve it
- Use version control for history, not archive folders
- Mark sections as "Historical Context" if needed, but keep them
- Add "Last Updated" dates rather than archiving

### 3. Prefer Small, Focused Files
- One concept per file
- Target 100-300 lines per document
- Split large documents into logical components
- Use clear file names that describe the single purpose
- Link between related documents rather than combining

## Naming Conventions

### File Naming
- Use lowercase with hyphens: `api-authentication.md`
- Be descriptive and specific: `user-registration-flow.md` not `users.md`
- Include the type when relevant: `rfc-001-api-versioning.md`
- Date-prefix for time-sensitive docs: `2024-01-15-meeting-notes.md`
- Avoid generic names: Not `overview.md`, use `project-overview.md`

### Folder Naming
- Use lowercase with hyphens for consistency
- Singular form for categories: `architecture` not `architectures`
- Clear, intuitive names that reflect content
- Avoid deep nesting (max 3 levels recommended)

### Document Titles
- Use Title Case for main headings: `# User Authentication System`
- Use sentence case for subheadings: `## How authentication works`
- Be specific and searchable in titles
- Include context in the title when helpful

## Recommended Tooling

### Primary Tools

#### Markdown
- **Usage**: All text-based documentation
- **Flavor**: CommonMark with GitHub Flavored Markdown extensions
- **Features to use**:
  - Tables for structured data
  - Task lists for tracking
  - Code blocks with syntax highlighting
  - Collapsible sections with `<details>`
  - Footnotes for additional context

#### Obsidian
- **Usage**: Knowledge management and linking
- **Key features**:
  - Wiki-style `[[links]]` between documents
  - Graph view for visualizing connections
  - Tags for categorization: `#architecture #api`
  - Canvas for visual documentation organization
  - Daily notes for development logs

#### Excalidraw
- **Usage**: Architecture diagrams and visual explanations
- **File naming**: `diagram-<purpose>.excalidraw`
- **Export**: Always export as SVG alongside the `.excalidraw` file
- **Embedding**: Use `![[diagram-name.svg]]` in Markdown
- **Version control**: Commit both `.excalidraw` and `.svg` files

#### Mermaid
- **Usage**: Flow charts, sequence diagrams, state machines
- **Embedding**: Use code blocks with `mermaid` language
- **Types to use**:
  ```mermaid
  graph TD
  flowchart LR
  sequenceDiagram
  stateDiagram-v2
  erDiagram
  gantt
  ```
- **Keep diagrams focused**: One concept per diagram

#### CSV Files
- **Usage**: Structured data, configuration matrices, comparison tables
- **Naming**: `data-<purpose>.csv`
- **Rules**:
  - Always include headers
  - Use consistent date formats (ISO 8601)
  - Quote fields containing commas
  - Document the schema in accompanying `.md` file

### Supporting Tools

#### PlantUML
- For complex UML diagrams
- Store source as `.puml` files
- Generate and commit SVG outputs

#### Draw.io
- For collaborative diagrams
- Export as `.drawio` and `.svg`
- Embed using standard Markdown image syntax

## Organizational Structure

### Top-Level Folders

```
docs/
├── business/           # Business strategy and planning
├── product/            # Product specifications and roadmaps
├── architecture/       # Technical architecture and design
├── implementation/     # Implementation guides and patterns
├── usage/             # User guides and tutorials
├── development/       # Developer documentation
└── references/        # External references and research
```

### Detailed Structure

#### business/
```
business/
├── strategy/          # Business strategy documents
├── requirements/      # Business requirements
├── stakeholders/      # Stakeholder maps and communications
├── metrics/          # KPIs and success metrics
├── compliance/       # Legal and compliance documentation
└── processes/        # Business processes and workflows
```

#### product/
```
product/
├── roadmap/          # Product roadmap and planning
├── features/         # Feature specifications
├── user-stories/     # User stories and scenarios
├── personas/         # User personas and research
├── design/           # Design documents and mockups
└── feedback/         # User feedback and analysis
```

#### architecture/
```
architecture/
├── decisions/        # Architecture Decision Records (ADRs)
├── diagrams/         # System architecture diagrams
├── components/       # Component specifications
├── integrations/     # Integration patterns and APIs
├── data/            # Data models and schemas
└── security/        # Security architecture
```

#### implementation/
```
implementation/
├── setup/           # Setup and installation guides
├── configuration/   # Configuration documentation
├── patterns/        # Code patterns and examples
├── apis/           # API documentation
├── database/       # Database schemas and migrations
└── deployment/     # Deployment guides
```

#### usage/
```
usage/
├── getting-started/  # Quick start guides
├── tutorials/        # Step-by-step tutorials
├── how-to/          # How-to guides for specific tasks
├── faq/             # Frequently asked questions
├── troubleshooting/ # Common issues and solutions
└── examples/        # Usage examples
```

#### development/
```
development/
├── contributing/    # Contribution guidelines
├── standards/       # Coding standards and conventions
├── testing/         # Testing strategies and guides
├── ci-cd/          # CI/CD documentation
├── tools/          # Development tools and setup
└── release/        # Release processes
```

## Documentation Standards

### Document Structure

Every document should include:

```markdown
# Document Title

> **Last Updated**: 2024-01-15
> **Status**: Draft | Review | Approved
> **Owner**: Team/Person Name

## Overview
Brief description of what this document covers.

## Context
Why this document exists and when to use it.

## Content
[Main content sections]

## Related Documents
- [[related-doc-1]]
- [[related-doc-2]]

## References
- External links and sources
```

### Writing Guidelines

#### Clarity
- Write for your audience (technical vs non-technical)
- Define acronyms on first use
- Use clear, concise language
- Provide examples for complex concepts

#### Structure
- Use hierarchical headings (max depth of 4)
- Include a table of contents for long documents
- Use bullet points for lists
- Number steps in procedures

#### Visual Elements
- Include diagrams where they add value
- Use tables for comparing options
- Add screenshots for UI documentation
- Create flowcharts for processes

#### Code Examples
- Provide working examples
- Include comments explaining key parts
- Show both correct and incorrect usage
- Specify language/version requirements

### Cross-Referencing

#### Internal Links
- Use Obsidian-style links: `[[document-name]]`
- Create bidirectional links where relevant
- Link to specific sections: `[[document#section]]`
- Maintain a glossary with term definitions

#### External Links
- Use descriptive link text, not "click here"
- Include link context in parentheses
- Archive important external content locally
- Note last-verified date for external resources

## Maintenance Practices

### Regular Reviews
- Schedule quarterly documentation reviews
- Update "Last Updated" dates
- Verify external links still work
- Ensure code examples still function
- Check for outdated information

### Version Control
- Commit documentation with code changes
- Write clear commit messages for doc updates
- Use semantic versioning for API docs
- Tag significant documentation milestones

### Collaboration
- Use pull requests for significant changes
- Request reviews from subject matter experts
- Include documentation in definition of done
- Maintain a documentation changelog

### Search Optimization
- Use descriptive headings
- Include relevant keywords naturally
- Create index pages for major sections
- Maintain a master glossary
- Tag documents consistently

## Quality Checklist

Before committing documentation:

- [ ] Searched for existing related documentation
- [ ] Followed naming conventions
- [ ] Included required metadata (date, status, owner)
- [ ] Added to appropriate folder structure
- [ ] Created necessary cross-references
- [ ] Included relevant diagrams or examples
- [ ] Verified all links work
- [ ] Spell-checked and grammar-checked
- [ ] Reviewed for clarity and completeness
- [ ] Updated any related documentation
- [ ] Ensured file is focused and appropriately sized
- [ ] Added to index or navigation if needed

## Anti-Patterns to Avoid

1. **Creating mega-documents**: Split into focused files
2. **Duplicating content**: Link to single source of truth
3. **Archiving content**: Update in place instead
4. **Generic file names**: Be specific and descriptive
5. **Deep nesting**: Keep folder structure shallow
6. **Orphaned documents**: Ensure everything is linked
7. **Outdated examples**: Test and update regularly
8. **Missing context**: Always explain why, not just what
9. **Ignoring existing docs**: Always search first
10. **Binary files in git**: Use appropriate storage for large files

## Tools Configuration

### Obsidian Settings
```yaml
plugins:
  - Excalidraw
  - Mermaid
  - CSV Editor
  - Git integration
  - Templates
  
settings:
  new_link_format: relative
  use_markdown_links: false
  default_view: source
```

### VS Code Extensions
- Markdown All in One
- Mermaid Preview
- Excalidraw
- CSV to Table
- Markdown lint

### Documentation Generation
- Use tools like MkDocs or Docusaurus for public docs
- Generate API docs from code comments
- Create automatic dependency graphs
- Build searchable documentation sites