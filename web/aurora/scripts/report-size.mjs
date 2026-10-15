import { readFileSync, readdirSync } from 'node:fs';
import { gzipSync } from 'node:zlib';
import assert from 'node:assert/strict';

const directory = new URL('../generated/', import.meta.url);
let raw = 0, gzip = 0;
for (const name of readdirSync(directory).filter(name => name.endsWith('.js')).sort()) {
  const bytes = readFileSync(new URL(name, directory));
  raw += bytes.byteLength;
  gzip += gzipSync(bytes, { level: 9 }).byteLength;
}
// Foundation-module size guard only, not rendering/network performance acceptance.
assert.ok(raw <= 32_768 && gzip <= 12_288, 'Review the Aurora foundation byte budget before expanding it');
console.log(JSON.stringify({ generatedRawBytes: raw, gzipBytesByFile: gzip, runtimeDependencies: 0, embeddedInBCC: false }));
