import { ArtifactKind, DataSource } from "../../lib/types";
import { documentPlugin } from "../utils";
const query = (cache) => documentPlugin(ArtifactKind.Query, function() {
  let subscriptionSpec = null;
  let lastVariables = null;
  return {
    start(ctx, { next }) {
      const runtimeScalarPayload = {
        session: ctx.session
      };
      ctx.variables = {
        ...lastVariables,
        ...Object.fromEntries(
          Object.entries(ctx.artifact.input?.runtimeScalars ?? {}).map(
            ([field, type]) => {
              const runtimeScalar = ctx.config.features?.runtimeScalars?.[type];
              if (!runtimeScalar) {
                return [field, type];
              }
              return [field, runtimeScalar.resolve(runtimeScalarPayload)];
            }
          )
        ),
        ...ctx.variables
      };
      next(ctx);
    },
    end(ctx, { resolve, marshalVariables, variablesChanged }) {
      if (variablesChanged(ctx) && !ctx.cacheParams?.disableSubscriptions) {
        if (subscriptionSpec) {
          cache.unsubscribe(subscriptionSpec, subscriptionSpec.variables?.() || {});
        }
        lastVariables = { ...marshalVariables(ctx) };
        const variables = lastVariables;
        subscriptionSpec = {
          rootType: ctx.artifact.rootType,
          selection: ctx.artifact.selection,
          variables: () => variables,
          set: (newValue) => {
            resolve(ctx, {
              data: newValue,
              errors: null,
              fetching: false,
              partial: false,
              stale: false,
              source: DataSource.Cache,
              variables: ctx.variables ?? {}
            });
          }
        };
        cache.subscribe(subscriptionSpec, lastVariables);
      }
      resolve(ctx);
    },
    cleanup() {
      if (subscriptionSpec) {
        cache.unsubscribe(subscriptionSpec, subscriptionSpec.variables?.());
        lastVariables = null;
      }
    }
  };
});
export {
  query
};
