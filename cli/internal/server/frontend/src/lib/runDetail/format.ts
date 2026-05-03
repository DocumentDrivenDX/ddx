export function fmtDate(iso: string | null): string {
	if (!iso) return '—'
	return new Date(iso).toLocaleString()
}

export function fmtDuration(ms: number | null): string {
	if (ms == null) return '—'
	if (ms < 1000) return `${ms}ms`
	if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
	const m = Math.floor(ms / 60_000)
	const s = Math.floor((ms % 60_000) / 1000)
	return `${m}m ${s}s`
}

export function fmtCost(c: number | null): string {
	if (c == null) return '—'
	return `$${c.toFixed(4)}`
}

export function tryPretty(s: string | null | undefined): string {
	if (!s) return ''
	try {
		return JSON.stringify(JSON.parse(s), null, 2)
	} catch {
		return s
	}
}
