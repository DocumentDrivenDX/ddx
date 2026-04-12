#!/bin/bash

# Obsidian Migration Tool
# Converts DDx markdown files to Obsidian-compatible format

set -e

# Default values
DRY_RUN=false
VERBOSE=false
PATH_ARG="."

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --help|-h)
            cat << EOF
Usage: $0 [OPTIONS] [PATH]

Convert DDx markdown files to Obsidian-compatible format with frontmatter and wikilinks.

Options:
    --dry-run       Show what would be changed without modifying files
    --verbose, -v   Show detailed processing information
    --help, -h      Show this help message

Arguments:
    PATH           Path to file or directory to migrate (default: current directory)

Examples:
    $0                                    # Migrate all markdown files in current directory
    $0 docs/                             # Migrate all files in docs/
    $0 docs/helix/01-frame/features/     # Migrate specific directory
    $0 --dry-run workflow.md            # Preview changes for single file

EOF
            exit 0
            ;;
        *)
            PATH_ARG="$1"
            shift
            ;;
    esac
done

# Function to detect file type
detect_file_type() {
    local file="$1"
    local filename=$(basename "$file")
    local dir=$(dirname "$file")

    # Feature files
    if [[ "$filename" =~ FEAT-[0-9]+ ]]; then
        echo "feature"
        return
    fi

    # Phase files
    if [[ "$filename" == "README.md" ]] && [[ "$dir" =~ phases/[0-9]+-[a-z]+ ]]; then
        echo "phase"
        return
    fi

    # Workflow files
    case "$filename" in
        coordinator.md) echo "coordinator"; return ;;
        enforcer.md) echo "enforcer"; return ;;
        principles.md|principle.md) echo "principle"; return ;;
        template.md) echo "template"; return ;;
        prompt.md) echo "prompt"; return ;;
    esac

    # Default
    echo "document"
}

# Function to generate frontmatter
generate_frontmatter() {
    local file="$1"
    local type="$2"
    local date=$(date +%Y-%m-%d)
    local filename=$(basename "$file" .md)

    case "$type" in
        feature)
            local feature_id=$(echo "$filename" | grep -oE 'FEAT-[0-9]+' || echo "FEAT-XXX")
            local title=$(grep -m1 '^# ' "$file" 2>/dev/null | sed 's/^# //' || echo "$filename")
            cat << EOF
---
id: $filename
title: $title
type: feature-specification
status: draft
tags:
  - feature
  - helix
created: $date
updated: $date
---
EOF
            ;;
        phase)
            local phase_name=$(dirname "$file" | grep -oE '[0-9]+-[a-z]+' | sed 's/^[0-9]+-//')
            cat << EOF
---
id: ${phase_name}-phase
title: $(echo "$phase_name" | sed 's/\b\(.\)/\u\1/g') Phase
type: workflow-phase
phase: $phase_name
status: active
tags:
  - helix
  - workflow
  - $phase_name
created: $date
updated: $date
---
EOF
            ;;
        coordinator|enforcer)
            cat << EOF
---
id: helix-$type
title: HELIX $(echo "$type" | sed 's/\b\(.\)/\u\1/g')
type: workflow-$type
workflow: helix
tags:
  - helix
  - workflow
  - $type
created: $date
updated: $date
---
EOF
            ;;
        *)
            cat << EOF
---
id: $filename
title: $filename
type: document
tags:
  - ddx
created: $date
updated: $date
---
EOF
            ;;
    esac
}

# Function to convert markdown links to wikilinks
convert_to_wikilinks() {
    local content="$1"

    # Convert [text](../path/file.md) to [[file|text]]
    echo "$content" | sed -E 's/\[([^\]]+)\]\(([^)]+)\.md\)/[[\2|\1]]/g' | \
        sed -E 's/\[\[([^|]+\/)*([^|]+)\|/[[\2|/g'
}

# Function to process a single file
process_file() {
    local file="$1"

    # Skip if not a markdown file
    if [[ ! "$file" =~ \.md$ ]]; then
        return
    fi

    # Check if file already has frontmatter
    if head -n1 "$file" | grep -q '^---$'; then
        if [[ "$VERBOSE" == true ]]; then
            echo "Skipping $file (already has frontmatter)"
        fi
        return
    fi

    local file_type=$(detect_file_type "$file")
    local frontmatter=$(generate_frontmatter "$file" "$file_type")
    local content=$(cat "$file")
    local converted_content=$(convert_to_wikilinks "$content")

    if [[ "$DRY_RUN" == true ]]; then
        echo "Would process: $file (type: $file_type)"
        if [[ "$VERBOSE" == true ]]; then
            echo "Frontmatter to add:"
            echo "$frontmatter"
            echo ""
        fi
    else
        # Create backup
        cp "$file" "${file}.backup"

        # Write new content
        {
            echo "$frontmatter"
            echo ""
            echo "$converted_content"
        } > "$file"

        echo "✅ Migrated: $file (type: $file_type)"
    fi
}

# Main execution
if [[ -f "$PATH_ARG" ]]; then
    # Process single file
    process_file "$PATH_ARG"
elif [[ -d "$PATH_ARG" ]]; then
    # Process directory
    echo "Processing directory: $PATH_ARG"

    # Find all markdown files
    while IFS= read -r -d '' file; do
        process_file "$file"
    done < <(find "$PATH_ARG" -name "*.md" -type f -print0)

    if [[ "$DRY_RUN" == false ]]; then
        echo ""
        echo "✅ Migration complete!"
        echo "💡 Backup files created with .backup extension"
    fi
else
    echo "Error: Path not found: $PATH_ARG"
    exit 1
fi