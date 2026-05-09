/**
 * Canonical ordered list of bead lifecycle status values shown in filter chips.
 * Runs routes use separate status options and are excluded from this list.
 */
export const BEAD_STATUS_OPTIONS = ['open', 'in-progress', 'blocked', 'closed', 'proposed', 'cancelled'] as const;

/**
 * Map display label (may use hyphens) to the wire value (underscores) sent to GraphQL.
 */
export function beadStatusWireValue(status: string): string {
	return status.replace(/-/g, '_');
}
