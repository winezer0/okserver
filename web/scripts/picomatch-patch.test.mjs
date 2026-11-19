import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { applyPatches, patch, patchedSource } from "./picomatch-patch.mjs";

const require = createRequire(import.meta.url);
const filename = require.resolve("picomatch/lib/parse.js");
const source = await readFile(filename, "utf8");
const patched = patchedSource(source, patch.version);
let original = patched;
for (const { before, after } of [...patch.edits].reverse()) original = original.replace(after, () => before);
assert.equal(createHash("sha256").update(original).digest("hex"), patch.originalSha256);

test("installed dependency is patched and patching is idempotent", async () => {
  assert.equal(source, patched, "Run npm rebuild before using the frontend toolchain");
  assert.equal(patchedSource(patched, patch.version), patched);
  assert.equal(await applyPatches(undefined, true), 1);
});

test("unknown versions and modified source fail closed", () => {
  assert.throws(() => patchedSource(original, "4.0.8"), /Unsupported/);
  assert.throws(() => patchedSource(original + "\n", patch.version), /Unexpected/);
  assert.throws(() => patchedSource(patched + "\n", patch.version), /Unexpected/);
});

test("clean and nested installations are patched and skipped scripts are detected", async t => {
  const root = await mkdtemp(path.join(os.tmpdir(), "okserver-picomatch-test-"));
  t.after(async () => {
    assert.equal(path.dirname(path.resolve(root)), path.resolve(os.tmpdir()));
    assert.ok(path.basename(root).startsWith("okserver-picomatch-test-"));
    await rm(root, { recursive: true, force: true });
  });
  const locations = ["node_modules/picomatch", "node_modules/example/node_modules/picomatch"];
  await writeFile(path.join(root, "package-lock.json"), JSON.stringify({ packages: Object.fromEntries(locations.map(location => [location, { version: patch.version }])) }));
  for (const location of locations) {
    const directory = path.join(root, location);
    await mkdir(path.join(directory, "lib"), { recursive: true });
    await writeFile(path.join(directory, "package.json"), JSON.stringify({ version: patch.version }));
    await writeFile(path.join(directory, patch.file), original);
  }
  await assert.rejects(applyPatches(root, true), /patch is missing/);
  assert.equal(await applyPatches(root), 2);
  assert.equal(await applyPatches(root, true), 2);
  assert.equal(await applyPatches(root), 2);
  for (const location of locations) assert.equal(await readFile(path.join(root, location, patch.file), "utf8"), patched);
});

function child(script, timeout = 5000) {
  const result = spawnSync(process.execPath, ["--input-type=commonjs", "-"], {
    input: script, timeout, encoding: "utf8", windowsHide: true,
  });
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  return result.stdout;
}

test("single-branch repeated wildcard attacks finish in a bounded child process", () => {
  child(`
    const assert = require('node:assert/strict');
    const pm = require(${JSON.stringify(require.resolve("picomatch"))});
    for (const options of [{}, { fastpaths: false }, { maxExtglobRecursion: false }, { capture: true }]) {
      for (const pattern of ['+(*)b', '+(*a)b', '*(*a)b', 'prefix/+(*a).txt', '+(a*)b', '+(+(a))b']) {
        const input = (pattern.startsWith('prefix/') ? 'prefix/' : '') + 'a'.repeat(256) + 'c';
        assert.equal(pm.isMatch(input, pattern, options), false, pattern);
      }
    }
    assert.equal(pm.isMatch('+(*)b', '+(*)b'), true, 'risky globs retain the upstream literal fallback');
  `);
});

test("normal glob, escaped wildcard and safe nested matching retain their behavior", () => {
  const cases = [
    ["src/main.ts", "**/*.{ts,tsx}"], ["src/main.tsx", "src/**/*.tsx"],
    ["src/foobar.ts", "src/+(foo|bar).ts"], ["src/other.ts", "src/+(foo|bar).ts"],
    ["aaa", "+(a)"], ["abab", "+(ab)"], ["***b", "+(\\*)b"],
    ["***b", "+([*])b"], ["abba", "+(*(a)|*(b))"],
    [".hidden", "**/*"], ["folder/file.txt", "*.txt"],
  ];
  const baseline = child(`
    const Module = require('node:module');
    const path = require('node:path');
    const filename = ${JSON.stringify(filename)};
    const module = new Module(filename);
    module.filename = filename;
    module.paths = Module._nodeModulePaths(path.dirname(filename));
    module._compile(${JSON.stringify(original)}, filename);
    require.cache[filename] = module;
    const pm = require(${JSON.stringify(require.resolve("picomatch"))});
    console.log(JSON.stringify(${JSON.stringify(cases)}.map(([input, glob]) => pm.isMatch(input, glob))));
  `);
  const pm = require("picomatch");
  assert.deepEqual(cases.map(([input, glob]) => pm.isMatch(input, glob)), JSON.parse(baseline));
});
