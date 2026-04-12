Build the tsk CLI task tracker per the specs in docs/.

Work through the ready beads using ddx bead ready to find what's unblocked.
For each bead:
1. ddx bead update <id> --claim
2. Write tests first (TDD Red phase)
3. Implement to pass tests (Green phase)
4. Run go test ./... to verify
5. git add and git commit with the bead ID in the message
6. ddx bead close <id>

The storage layer (JSON file at .tsk.json) must be implemented first since
all commands depend on it.

Use Go. Keep it simple — no frameworks, just standard library.
Each command is a function called from main() based on os.Args.
