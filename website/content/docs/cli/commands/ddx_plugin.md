---
title: "plugin"
generated: true
---

## ddx plugin

Manage project plugins

### Synopsis

Manage project-scoped plugin dependencies. Registry installs write
`.ddx/plugins.lock.yaml`, resolve payloads into the shared XDG plugin cache,
and generate local `.agents/skills/` plus `.claude/skills/` adapters.

Use `ddx plugin sync` to recreate generated adapters from the lock/cache.

```
ddx plugin [command]
```

### Available Commands

```
install     Install or update a project plugin
list        List project plugins
show        Show one project plugin
sync        Recreate generated plugin adapters
uninstall   Remove a project plugin
upgrade     Upgrade project plugins
```

### Examples

```
ddx plugin install helix
ddx plugin list
ddx plugin sync
ddx plugin install helix --local ../helix --force
ddx doctor --plugins
```

### SEE ALSO

* [ddx](/docs/cli/commands/ddx/) - Document-Driven Development eXperience - AI development toolkit
