import { defineConfig } from '@playwright/test';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// The default Playwright harness boots the Go DDx server against a temp copy
// of e2e/fixtures/ so the API endpoints exercised by TC-008 (and other
// backend-dependent specs) return real data without reading the developer's
// $HOME or the repository's live .ddx/ state.

const CLI_PKG_DIR = path.resolve(__dirname, '../../..');
const FRONTEND_DIR = __dirname;
const FIXTURE_DIR = path.resolve(__dirname, 'e2e/fixtures');
const PORT = Number(process.env.DDX_E2E_PORT ?? 4174);
const BASE_URL = `https://127.0.0.1:${PORT}`;

// Build the SvelteKit frontend (so cli/internal/server/embed.go has assets to
// embed), copy the fixture workspace into a fresh temp dir, build the ddx
// binary from the cli module, and exec it from the temp dir. The temp dir has
// no .git so FindProjectRoot falls back to it, and tsnet is disabled so the
// harness has no Tailscale dependency.
const bootCommand = [
	`set -e`,
	`(cd "${FRONTEND_DIR}" && bun run build >&2)`,
	`TMP=$(mktemp -d -t ddx-e2e-XXXXXX)`,
	`cp -R "${FIXTURE_DIR}/." "$TMP/"`,
	`(cd "${CLI_PKG_DIR}" && go build -o "$TMP/ddx" .)`,
	`cd "$TMP"`,
	`DDX_OPERATOR_PROMPT_ALLOWLIST=localhost exec ./ddx server --tsnet=false --addr=127.0.0.1 --port=${PORT}`
].join(' && ');

export default defineConfig({
	webServer: {
		command: `bash -c '${bootCommand}'`,
		url: `${BASE_URL}/api/health`,
		ignoreHTTPSErrors: true,
		reuseExistingServer: !process.env.CI,
		timeout: 180_000,
		stdout: 'pipe',
		stderr: 'pipe'
	},
	use: {
		baseURL: BASE_URL,
		ignoreHTTPSErrors: true
	},
	testMatch: ['**/*.e2e.{ts,js}', 'e2e/**/*.spec.ts']
});
