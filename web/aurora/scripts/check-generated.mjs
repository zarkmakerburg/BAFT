import { mkdtempSync, readFileSync, readdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';

const root = fileURLToPath(new URL('../', import.meta.url));
const temporary = mkdtempSync(join(tmpdir(), 'baft-aurora-build-'));
const files = path => readdirSync(path, { recursive: true, withFileTypes: true })
  .filter(entry => entry.isFile()).map(entry => join(entry.parentPath, entry.name).slice(path.length + 1)).sort();
try {
  const compiler = resolve(root, 'node_modules/typescript/bin/tsc');
  const version = JSON.parse(readFileSync(resolve(root, 'node_modules/typescript/package.json'), 'utf8')).version;
  assert.equal(version, '6.0.3', 'Use the locked compiler');
  execFileSync(process.execPath, [compiler, '-p', resolve(root, 'tsconfig.json'), '--outDir', temporary], { stdio: 'inherit' });
  const committed = resolve(root, 'generated');
  const expected = files(temporary);
  assert.deepEqual(files(committed), expected, 'Missing or orphan generated assets');
  for (const path of expected) assert.deepEqual(readFileSync(join(committed, path)), readFileSync(join(temporary, path)), `Generated output drift: ${path}`);
  console.log(`Generated assets verified: ${expected.length} files, compiler ${version}`);
} finally {
  rmSync(temporary, { recursive: true, force: true });
}
