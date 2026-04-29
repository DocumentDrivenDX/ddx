# Health Check Endpoint PRD

## Overview
Implement a simple HTTP health check endpoint for the hello-bun server.

## Requirements

### API Endpoint: GET /health
- **Response format**: JSON
- **Response schema**:
  ```json
  {
    "status": "ok",
    "uptime": <number of seconds server has been running>
  }
  ```
- **Status code**: 200 OK

### Technical Constraints
Per typescript-bun concern:
- Implement using raw `Bun.serve()` (no Express, Fastify, or other framework)
- Write tests using `bun:test` (no Vitest, Jest)
- Format code with Biome (no ESLint, Prettier)

### Success Criteria
- GET /health endpoint responds with JSON containing `status` and `uptime` fields
- Uptime accurately reflects server startup time
- Server runs on configurable port (default: 3000)
- Full test coverage for the endpoint
