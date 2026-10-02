import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import test from 'node:test';

const apiPath = new URL('../src/api.ts', import.meta.url);
const sourceRoot = new URL('../src/', import.meta.url);

async function sourceFiles(directory: URL): Promise<string[]> {
  const files: string[] = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = new URL(entry.name, directory);
    if (entry.isDirectory()) files.push(...await sourceFiles(new URL(`${entry.name}/`, directory)));
    else if (/\.[cm]?[jt]sx?$/.test(entry.name)) files.push(await readFile(path, 'utf8'));
  }
  return files;
}

test('全量前端源码不使用浏览器持久化存储', async () => {
  const sources = await sourceFiles(sourceRoot);
  const forbidden = /\b(?:localStorage|sessionStorage|indexedDB)\b|document\.cookie|navigator\.storage|caches\.(?:open|match|delete)|serviceWorker\.register\s*\(/;
  for (const source of sources) assert.doesNotMatch(source, forbidden);
});

test('every API fetch explicitly sets cache no-store after caller init', async () => {
  const source = await readFile(apiPath, 'utf8');
  const fetches = [...source.matchAll(/fetch\([^,]+,\s*\{([\s\S]*?)\}\)/g)].map(match => match[1]);
  assert.equal(fetches.length, 3);
  for (const init of fetches) {
    assert.match(init, /cache:\s*'no-store'/);
    if (init.includes('...init')) assert.ok(init.indexOf("cache: 'no-store'") > init.indexOf('...init'));
  }
});
