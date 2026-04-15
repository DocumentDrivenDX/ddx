import cacheRef from "../cache";
import { getCurrentConfig, localApiEndpoint } from "../lib";
import { flatten } from "../lib/flatten";
import { DocumentStore } from "./documentStore";
import {
  fetch as fetchPlugin,
  fetchParams as fetchParamsPlugin,
  fragment as fragmentPlugin,
  mutation as mutationPlugin,
  query as queryPlugin,
  throwOnError as throwOnErrorPlugin,
  optimisticKeys
} from "./plugins";
import pluginsFromPlugins from "./plugins/injectedPlugins";
import { DocumentStore as DocumentStore2 } from "./documentStore";
import { fetch, mutation, query, subscription } from "./plugins";
class HoudiniClient {
  url;
  throwOnError_operations;
  cache = null;
  throwOnError;
  fetchParams;
  pipeline;
  extraPlugins;
  proxies = {};
  componentCache = {};
  setCache(cache) {
    this.cache = cache;
  }
  constructor({
    url,
    fetchParams,
    plugins,
    pipeline,
    throwOnError
  } = {}) {
    if (plugins && pipeline) {
      throw new Error(
        "A client cannot be given a pipeline and a list of plugins at the same time."
      );
    }
    this.throwOnError_operations = throwOnError?.operations ?? [];
    let serverPort = globalThis.process?.env?.HOUDINI_PORT ?? "5173";
    this.url = url ?? (globalThis.window ? "" : `https://localhost:${serverPort}`) + localApiEndpoint(getCurrentConfig());
    this.throwOnError = throwOnError;
    this.fetchParams = fetchParams;
    this.pipeline = pipeline;
    this.extraPlugins = plugins;
  }
  get plugins() {
    return flatten(
      [].concat(
        this.throwOnError ? [throwOnErrorPlugin(this.throwOnError)] : [],
        fetchParamsPlugin(this.fetchParams),
        this.pipeline ?? [
          optimisticKeys(this.cache ?? cacheRef),
          queryPlugin(this.cache ?? cacheRef),
          mutationPlugin(this.cache ?? cacheRef),
          fragmentPlugin(this.cache ?? cacheRef)
        ].concat(
          this.extraPlugins ?? [],
          pluginsFromPlugins,
          fetchPlugin()
        )
      )
    );
  }
  observe({
    enableCache = true,
    fetching = false,
    ...rest
  }) {
    return new DocumentStore({
      client: this,
      plugins: createPluginHooks(this.plugins),
      fetching,
      enableCache,
      cache: this.cache ?? void 0,
      ...rest
    });
  }
  registerProxy(url, handler) {
    this.proxies[url] = handler;
  }
}
function createPluginHooks(plugins) {
  return plugins.reduce((hooks, plugin) => {
    if (typeof plugin !== "function") {
      throw new Error("Encountered client plugin that's not a function");
    }
    const result = plugin();
    if (!result) {
      return hooks;
    }
    if (!Array.isArray(result)) {
      return hooks.concat(result);
    }
    for (const value of result) {
      if (!value) {
        continue;
      }
      if (typeof value === "function") {
        return hooks.concat(createPluginHooks([value]));
      }
      hooks.push(value);
    }
    return hooks;
  }, []);
}
export {
  DocumentStore2 as DocumentStore,
  HoudiniClient,
  createPluginHooks,
  fetch,
  mutation,
  query,
  subscription
};
