import { spawnSync } from 'node:child_process';
import * as crypto from 'node:crypto';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';

const FRONTEND_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const CLI_DIR = path.resolve(FRONTEND_DIR, '../../..');

let ddxBinary: string | null = null;

function sleep(ms: number) {
	Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

export function ensureDdxE2EBinary(): string {
	if (ddxBinary) return ddxBinary;

	const cacheKey = crypto.createHash('sha256').update(CLI_DIR).digest('hex').slice(0, 16);
	const root = path.join(os.tmpdir(), `ddx-e2e-bin-${cacheKey}`);
	const binary = path.join(root, process.platform === 'win32' ? 'ddx-e2e.exe' : 'ddx-e2e');
	const lock = path.join(root, 'build.lock');
	const goCache = path.join(root, 'gocache');
	fs.mkdirSync(root, { recursive: true });

	if (fs.existsSync(binary)) {
		ddxBinary = binary;
		return binary;
	}

	let locked = false;
	for (let i = 0; i < 600; i++) {
		try {
			fs.mkdirSync(lock);
			locked = true;
			break;
		} catch (err) {
			if ((err as NodeJS.ErrnoException).code !== 'EEXIST') throw err;
			if (fs.existsSync(binary)) {
				ddxBinary = binary;
				return binary;
			}
			sleep(250);
		}
	}
	if (!locked) throw new Error(`timed out waiting for ddx e2e binary build lock: ${lock}`);

	try {
		if (fs.existsSync(binary)) {
			ddxBinary = binary;
			return binary;
		}

		fs.mkdirSync(goCache, { recursive: true });
		const tmpBinary = path.join(root, `ddx-e2e.${process.pid}.tmp`);
		const result = spawnSync('go', ['build', '-buildvcs=false', '-o', tmpBinary, '.'], {
			cwd: CLI_DIR,
			env: {
				...process.env,
				GOCACHE: goCache
			},
			encoding: 'utf8'
		});
		if (result.status !== 0) {
			throw new Error(`failed to build ddx test binary\n${result.stdout}\n${result.stderr}`);
		}
		fs.renameSync(tmpBinary, binary);
		ddxBinary = binary;
		return binary;
	} finally {
		fs.rmSync(lock, { recursive: true, force: true });
	}
}
