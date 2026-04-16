/**
 * Reactive WebSocket connection state for the graphql-ws subscription client.
 *
 * Tracks whether the WS has been connected and its current state so the UI
 * can render a "disconnected" / "reconnecting…" banner when live updates are
 * stale.
 */

export type WsState = 'idle' | 'connecting' | 'connected' | 'disconnected';

let _state = $state<WsState>('idle');
let _wasConnected = $state(false);

export const wsConnection = {
	get state(): WsState {
		return _state;
	},

	/** True when the client was previously connected but is no longer. */
	get showBanner(): boolean {
		return _wasConnected && _state !== 'connected';
	},

	// ---- called by subscriptions.ts when client events fire ----

	_onConnecting() {
		_state = 'connecting';
	},
	_onConnected() {
		_state = 'connected';
		_wasConnected = true;
	},
	_onClosed() {
		_state = _wasConnected ? 'disconnected' : 'idle';
	},

	/** Reset for tests / hot-reload. */
	_reset() {
		_state = 'idle';
		_wasConnected = false;
	}
};
