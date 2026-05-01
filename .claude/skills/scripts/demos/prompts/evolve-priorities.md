Use /helix-evolve to add task priorities to the tsk tracker.

Tasks should support priority levels: high, medium, low (default: medium).
- `tsk add --priority high "urgent task"` sets priority at creation
- `tsk list` shows priority as a column
- `tsk list --priority high` filters by priority

Update the feature spec, create implementation beads, and build the feature.
