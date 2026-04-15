// GraphQL client entry point.
// HTTP queries/mutations: use createClient() from $lib/gql/client.
// WebSocket subscriptions: use subscribeWorkerProgress() and friends from $lib/gql/subscriptions.
export { createClient } from '$lib/gql/client'
export { getSubscriptionClient, subscribeWorkerProgress } from '$lib/gql/subscriptions'
export type { WorkerEvent } from '$lib/gql/subscriptions'
