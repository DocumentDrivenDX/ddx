<script lang="ts">
	interface Props {
		active?: boolean;
		onclick: (e: MouseEvent) => void;
		label: string;
		testid?: string;
		ariaPressed?: boolean;
		clear?: boolean;
	}
	let { active = false, onclick, label, testid, ariaPressed, clear = false }: Props = $props();

	const activeCls =
		'bg-accent-lever text-white dark:bg-dark-accent-lever';
	const inactiveCls =
		'bg-bg-surface text-fg-muted hover:bg-bg-elevated hover:text-fg-ink dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink';
	const clearCls =
		'text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink';
</script>

<button
	type="button"
	data-testid={testid}
	aria-pressed={ariaPressed}
	{onclick}
	class="rounded-full px-3 py-1 font-label-caps text-label-caps uppercase transition-colors {clear
		? clearCls
		: active
			? activeCls
			: inactiveCls}"
>
	{label}
</button>
