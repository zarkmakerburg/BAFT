import { readFileSync, readdirSync } from 'node:fs';
import { gzipSync } from 'node:zlib';
import assert from 'node:assert/strict';

const directory = new URL('../generated/', import.meta.url);
let raw = 0, gzip = 0, shellRaw = 0, shellGzip = 0;
const shellModules = new Set(['catalog.js', 'preferences.js', 'shell.js', 'preview-entry.js']);
for (const name of readdirSync(directory).filter(name => name.endsWith('.js')).sort()) {
  const bytes = readFileSync(new URL(name, directory));
  const compressed = gzipSync(bytes, { level: 9 }).byteLength;
  if (shellModules.has(name)) { shellRaw += bytes.byteLength; shellGzip += compressed; }
  else { raw += bytes.byteLength; gzip += compressed; }
}
// Foundation-module size guard only, not rendering/network performance acceptance.
assert.ok(raw <= 32_768 && gzip <= 12_288, 'Review the Aurora foundation byte budget before expanding it');
assert.ok(shellRaw <= 32_768 && shellGzip <= 10_240, 'Review the isolated shell byte budget before expanding it');
const css = readFileSync(new URL('../preview/shell.css', import.meta.url));
const cssGzip = gzipSync(css, { level: 9 }).byteLength;
assert.ok(css.byteLength <= 12_288 && cssGzip <= 4096, 'Review preview CSS size');
console.log(JSON.stringify({ foundationRawBytes: raw, foundationGzipBytes: gzip, shellRawBytes: shellRaw,
  shellGzipBytes: shellGzip, cssRawBytes: css.byteLength, cssGzipBytes: cssGzip,
  runtimeDependencies: 0, embeddedInBCC: false }));
