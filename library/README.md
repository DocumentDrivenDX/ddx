# DDx Library

Source tree for the default DDx plugin (`package.yaml` name: `ddx`).
`ddx init` installs this content into the project plugin layer; other
tools may also resolve it from a global cache or the binary-embedded
copy.

## Contents

| Path | Purpose |
|------|---------|
| `personas/` | Bindable AI personality templates |
| `prompts/` | Shared prompts and system-prompt fragments |
| `patterns/` | Reusable code/architecture patterns |
| `templates/` | Project templates |
| `skills/` | Agent-facing skills (agentskills.io layout) |
| `mcp-servers/` | MCP server registry entries |
| `environments/` | Reproducible dev environment configs |
| `checks/` | Dun-compatible quality checks |
| `artifacts/` | Shared artifact templates (MET, operator prompt) |
| `tools/` | Auxiliary scripts |

## Install topology

Plugin and persona lookup follows **project > global > baked-in**
precedence (`ddx-937de9fc`):

1. **Project-local** — `ddxroot.Path()/plugins/<name>/`
   - In-tree mode: `<project>/.ddx/plugins/<name>/`
   - Convention mode: `${XDG_DATA_HOME}/ddx/projects/<identity>/plugins/<name>/`
2. **Global install** — `${XDG_DATA_HOME}/ddx/global/plugins/<name>/`
3. **Baked-in** — package embedded in the DDx binary (default `ddx`
   plugin only)

The project layer always wins when present. `ddx doctor` reports the
global and project install layers, including when the project copy is
absent and resolution falls through (`lazy-resolves-to-global`).

### Agent-facing skills

Universal agent-facing skill installation is handled by the marketplace
package in `DocumentDrivenDX/ddx-library`, not by forcing a DDx package
reinstall:

```bash
npx claude-plugins install @DocumentDrivenDX/ddx-library/ddx
```

That path is separate from the DDx package cache above. DDx still
resolves project plugin resources (personas, prompts, templates) from
the project/global/baked-in layers.

## Related docs

- Persona details: `personas/README.md`
- DDx skill (intent router + topology): `skills/ddx/SKILL.md`
- Agent guide (persona system + precedence): repo-root `CLAUDE.md`
