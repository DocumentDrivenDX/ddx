const PORT = process.env.PORT ? parseInt(process.env.PORT) : 3000;
const startTime = Date.now();

const server = Bun.serve({
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

console.log(`Server running at http://localhost:${PORT}`);
