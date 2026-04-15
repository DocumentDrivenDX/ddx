import { createServerAdapter as createAdapter } from "@whatwg-node/server";
import { parse, execute } from "graphql";
import { createYoga } from "graphql-yoga";
import { localApiSessionKeys, localApiEndpoint, getCurrentConfig } from "../lib/config";
import { find_match } from "./match";
import { get_session, handle_request } from "./session";
const config_file = getCurrentConfig();
const session_keys = localApiSessionKeys(config_file);
function _serverHandler({
  schema,
  yoga,
  client,
  production,
  manifest,
  graphqlEndpoint,
  on_render,
  componentCache
}) {
  if (schema && !yoga) {
    yoga = createYoga({
      schema,
      landingPage: !production,
      graphqlEndpoint
    });
  }
  client.componentCache = componentCache;
  if (schema) {
    client.registerProxy(graphqlEndpoint, async ({ query, variables, session }) => {
      const parsed = parse(query);
      return await execute(schema, parsed, null, session, variables);
    });
  }
  return async (request) => {
    if (!manifest) {
      return new Response(
        "Adapter did not provide the project's manifest. Please open an issue on github.",
        { status: 500 }
      );
    }
    const url = new URL(request.url).pathname;
    if (yoga && url === localApiEndpoint(config_file)) {
      return yoga(request);
    }
    const authResponse = await handle_request({
      request,
      config: config_file,
      session_keys
    });
    if (authResponse) {
      return authResponse;
    }
    const [match] = find_match(config_file, manifest, url);
    const rendered = await on_render({
      url,
      match,
      session: await get_session(request.headers, session_keys),
      manifest,
      componentCache
    });
    if (rendered) {
      return rendered;
    }
    return new Response("404", { status: 404 });
  };
}
const serverAdapterFactory = (args) => {
  return createAdapter(_serverHandler(args));
};
export {
  _serverHandler,
  serverAdapterFactory
};
