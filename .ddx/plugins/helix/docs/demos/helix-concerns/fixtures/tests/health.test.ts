import { describe, it, expect, beforeAll, afterAll } from "bun:test";

let server: any;
const PORT = 3001;
const baseUrl = `http://localhost:${PORT}`;

beforeAll(async () => {
  const startTime = Date.now();

  server = Bun.serve({
    port: PORT,
    fetch(request: Request): Response {
      const url = new URL(request.url);

      if (url.pathname === "/health" && request.method === "GET") {
        const uptimeSeconds = Math.floor((Date.now() - startTime) / 1000);
        return new Response(
          JSON.stringify({
            status: "ok",
            uptime: uptimeSeconds,
          }),
          {
            status: 200,
            headers: {
              "Content-Type": "application/json",
            },
          },
        );
      }

      return new Response("Not Found", {
        status: 404,
      });
    },
  });
});

afterAll(() => {
  server?.stop();
});

describe("GET /health", () => {
  it("should return 200 OK", async () => {
    const response = await fetch(`${baseUrl}/health`);
    expect(response.status).toBe(200);
  });

  it("should return JSON with status and uptime fields", async () => {
    const response = await fetch(`${baseUrl}/health`);
    const data = await response.json();

    expect(data).toHaveProperty("status");
    expect(data).toHaveProperty("uptime");
  });

  it("should have status equal to 'ok'", async () => {
    const response = await fetch(`${baseUrl}/health`);
    const data = await response.json();

    expect(data.status).toBe("ok");
  });

  it("should return uptime as a number", async () => {
    const response = await fetch(`${baseUrl}/health`);
    const data = await response.json();

    expect(typeof data.uptime).toBe("number");
    expect(data.uptime).toBeGreaterThanOrEqual(0);
  });

  it("should return increasing uptime on subsequent calls", async () => {
    const response1 = await fetch(`${baseUrl}/health`);
    const data1 = await response1.json();

    await new Promise((resolve) => setTimeout(resolve, 100));

    const response2 = await fetch(`${baseUrl}/health`);
    const data2 = await response2.json();

    expect(data2.uptime).toBeGreaterThanOrEqual(data1.uptime);
  });

  it("should have Content-Type header set to application/json", async () => {
    const response = await fetch(`${baseUrl}/health`);
    expect(response.headers.get("Content-Type")).toBe("application/json");
  });
});
