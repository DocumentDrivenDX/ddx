Create the Frame-phase artifacts for "tsk", a Go CLI task tracker.

1. Write docs/prd.md with ddx: frontmatter (id: PRD-001):
   - Problem: developers need a zero-friction CLI task tracker
   - Features: tsk add "title", tsk list, tsk done <id>, tsk rm <id>
   - Tasks stored as JSON in .tsk.json
   - P0: basic CRUD. P1: priorities. P2: due dates.
   - Keep it to ~40 lines

2. Write docs/features/FEAT-001-task-crud.md with ddx: frontmatter:
   - User stories with acceptance criteria for each command
   - tsk add "Buy milk" → prints "Added task #1: Buy milk"
   - tsk list → shows id, status checkbox, title
   - tsk done 1 → marks task as done
   - tsk rm 1 → removes task
   - ~40 lines

3. Create tracker beads using ddx bead create:
   ddx bead create "Implement JSON storage layer" --type task --labels "helix,phase:build" --set "spec-id=FEAT-001"
   ddx bead create "Implement tsk add command" --type task --labels "helix,phase:build" --set "spec-id=FEAT-001"
   ddx bead create "Implement tsk list command" --type task --labels "helix,phase:build" --set "spec-id=FEAT-001"
   ddx bead create "Implement tsk done command" --type task --labels "helix,phase:build" --set "spec-id=FEAT-001"
   ddx bead create "Implement tsk rm command" --type task --labels "helix,phase:build" --set "spec-id=FEAT-001"

4. Wire dependencies: all command beads depend on the storage bead.
   Use ddx bead dep add <command-id> <storage-id> for each.

Create directories as needed. Use markdown with ddx: frontmatter.
