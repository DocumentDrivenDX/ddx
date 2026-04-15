<script lang="ts">
	import { Button } from 'bits-ui';
	import { User } from 'lucide-svelte';
	import { toggleMode, mode } from 'mode-watcher';
</script>

<div class="flex min-h-screen flex-col items-center justify-center gap-8 p-8">
	<h1 class="text-2xl font-bold">UI Primitives Test</h1>

	<div class="flex items-center gap-4">
		<User class="h-6 w-6" />
		<span>lucide-svelte User icon</span>
	</div>

	<Button.Root
		class="rounded bg-blue-600 px-4 py-2 text-white hover:bg-blue-700"
		onclick={() => alert('bits-ui Button clicked')}
	>
		bits-ui Button
	</Button.Root>

	<button
		class="rounded border px-4 py-2 hover:bg-gray-100 dark:border-gray-600 dark:hover:bg-gray-800"
		onclick={toggleMode}
	>
		Toggle dark mode (current: {mode})
	</button>
</div>
