---
title: DDx Server
weight: 4
---

`ddx-server` exposes your document library over HTTP and MCP endpoints, letting AI agents browse, search, and read documents programmatically.

{{< callout type="info" >}}
DDx Server is under active development. This page describes the planned architecture and API.
{{< /callout >}}

## Why a Server?

The DDx CLI manages documents locally. But agents often need to **discover** documents — to find out what's available and fetch what's relevant. The server bridges that gap:

- **MCP endpoints** let agents call tools like `ddx_list_documents` and `ddx_read_document`
- **HTTP API** supports web interfaces, scripts, and non-MCP consumers
- **Search** enables finding documents by content, not just path

## Quick Start

```bash
# Start serving your local library
ddx-server --library-path .ddx/library

# Specify a port
ddx-server --port 8080

# With API key for non-local access
ddx-server --api-key your-secret-key
```

## MCP Endpoints

When an agent connects to `ddx-server` via MCP, these tools become available:

### `ddx_list_documents`

List documents in the library, optionally filtered by type.

```json
{
  "type": "personas"
}
```

Returns document names, types, and brief descriptions.

### `ddx_read_document`

Fetch the full content of a document by path.

```json
{
  "path": "personas/strict-code-reviewer"
}
```

Returns the complete document content.

### `ddx_search`

Search across document contents.

```json
{
  "query": "error handling",
  "type": "patterns"
}
```

Returns matching documents ranked by relevance.

### `ddx_resolve_persona`

Given a role name, resolve it to the bound persona document using the project's configuration.

```json
{
  "role": "code-reviewer"
}
```

Returns the full persona document for the bound persona.

## HTTP API

The same operations are available over REST:

| Method | Path | Description |
|--------|------|------------|
| `GET` | `/api/documents` | List all documents |
| `GET` | `/api/documents?type=personas` | List by type |
| `GET` | `/api/documents/{path}` | Read a document |
| `GET` | `/api/search?q=error+handling` | Search documents |
| `GET` | `/api/persona/{role}` | Resolve persona for role |

All responses are JSON.

## Architecture

DDx Server is stateless and lightweight:

- **Reads from filesystem** — no database, no persistent state
- **Single binary** — same Go toolchain as the CLI
- **Concurrent-safe** — handles parallel requests safely
- **Minimal footprint** — suitable for running alongside development tools
