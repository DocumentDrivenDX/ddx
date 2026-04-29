# Refinement Log: {{STORY_ID}}-refinement-{{REFINEMENT_NUMBER}}

**Story ID**: {{STORY_ID}}
**Date**: {{CURRENT_DATE}}
**Type**: {{REFINEMENT_TYPE}} <!-- bugs|requirements|enhancement|mixed -->
**Triggered By**: {{TRIGGER_REASON}}
**Status**: {{STATUS}} <!-- in-progress|completed|deferred -->

## Summary

{{BRIEF_DESCRIPTION}}

**Affected Phases**: {{AFFECTED_PHASES}} <!-- frame|design|test|build|deploy|iterate -->

## Original State

- **Story**: [{{STORY_ID}} - {{STORY_TITLE}}](../01-frame/user-stories/{{STORY_ID}}.md)
- **Status**: {{IMPLEMENTATION_STATUS}}
- **Key AC**: {{#each ORIGINAL_ACCEPTANCE_CRITERIA}}**{{ac_id}}**: {{ac_description}}; {{/each}}

## Issues Identified

{{#each IDENTIFIED_ISSUES}}
### {{issue_number}}: {{issue_title}}

**Category**: {{issue_category}} | **Priority**: {{issue_priority}} | **Affected AC**: {{#each affected_ac}}{{ac_id}} {{/each}}

{{issue_description}}

**Root Cause**: {{root_cause}}
{{/each}}

## Resolutions

{{#each REFINEMENT_RESOLUTIONS}}
### {{resolution_number}}: {{resolution_title}}

**Addresses**: {{addressed_issues}} | **Strategy**: {{strategy}}

{{resolution_description}}

**Requirements Changes**:
{{#each requirement_changes}}
- {{change_type}} {{target_requirement}}: {{refined_text}} (was: {{original_text}})
{{/each}}

**Impact**: code: {{code_changes}}; tests: {{test_updates}}; effort: {{effort_estimate}}
{{/each}}

## Phase Updates

| Phase | Changes |
|-------|---------|
| Frame | {{#each story_modifications}}{{section_name}}: {{change_type}} — {{updated_content}}; {{/each}} |
| Design | {{#each architecture_changes}}{{component_name}}: {{change_description}}; {{/each}} |
| Test | {{#each new_test_cases}}NEW: {{test_name}}; {{/each}}{{#each modified_test_cases}}MOD: {{test_name}}; {{/each}} |
| Build | {{#each implementation_changes}}{{component_name}}: {{implementation_change}}; {{/each}} |

## Next Actions

{{#each immediate_actions}}
- [ ] {{action_description}} (owner: {{action_owner}})
{{/each}}
{{#each followup_items}}
- [ ] {{item_description}} (timeline: {{item_timeline}})
{{/each}}
