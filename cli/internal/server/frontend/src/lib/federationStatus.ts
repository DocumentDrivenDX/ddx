// Helpers for rendering FederationNode status across the federation UI.
// Maps the spoke status string (registered, active, stale, offline, degraded)
// to the existing badge-status-* CSS classes so each state has a distinct
// visual treatment.

export type FederationStatus = 'registered' | 'active' | 'stale' | 'offline' | 'degraded' | string

export function federationBadgeClass(status: string): string {
	switch (status) {
		case 'active':
			return 'badge-status-closed'
		case 'stale':
			return 'badge-status-in-progress'
		case 'degraded':
			return 'badge-status-blocked'
		case 'offline':
			return 'badge-status-failed'
		case 'registered':
			return 'badge-status-open'
		default:
			return 'badge-status-neutral'
	}
}

export function isVersionSkew(lastError: string | null | undefined): boolean {
	if (!lastError) return false
	const e = lastError.toLowerCase()
	return e.includes('version') || e.includes('skew') || e.includes('schema')
}
