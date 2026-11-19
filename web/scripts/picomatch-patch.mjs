import { createHash } from "node:crypto";
import { readFile, realpath, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const frontend = fileURLToPath(new URL("../", import.meta.url));
export const patch = JSON.parse(await readFile(new URL("../patches/picomatch-4.0.7.json", import.meta.url), "utf8"));
const hash = source => createHash("sha256").update(source).digest("hex");

export function patchedSource(source, version) {
  if (version !== patch.version) throw new Error(`Unsupported picomatch ${version}; review the security patch before upgrading`);
  const actual = hash(source);
  if (actual === patch.patchedSha256) return source;
  if (actual !== patch.originalSha256) throw new Error("Unexpected picomatch source; refusing to apply the security patch");
  for (const { before, after } of patch.edits) {
    if (source.split(before).length !== 2) throw new Error("Picomatch patch context mismatch");
    source = source.replace(before, () => after);
  }
  if (hash(source) !== patch.patchedSha256) throw new Error("Picomatch patched source checksum mismatch");
  return source;
}

export async function applyPatches(root = frontend, check = false) {
  const lock = JSON.parse(await readFile(path.join(root, "package-lock.json"), "utf8"));
  const modules = await realpath(path.join(root, "node_modules"));
  let installed = 0;
  for (const location of Object.keys(lock.packages)) {
    if (!location.endsWith("node_modules/picomatch")) continue;
    const directory = path.resolve(root, location);
    let resolved;
    try { resolved = await realpath(directory); }
    catch (error) {
      if (error.code === "ENOENT" && !check) continue; // npm ci --omit=dev
      throw error;
    }
    const relative = path.relative(modules, resolved);
    if (relative.startsWith("..") || path.isAbsolute(relative)) throw new Error("Picomatch must be installed inside node_modules");
    const { version } = JSON.parse(await readFile(path.join(resolved, "package.json"), "utf8"));
    const filename = path.join(resolved, patch.file);
    const source = await readFile(filename, "utf8");
    const result = patchedSource(source, version);
    if (check && source !== result) throw new Error("Picomatch security patch is missing; run npm rebuild in web");
    if (source !== result) await writeFile(filename, result);
    installed++;
  }
  if (check && installed === 0) throw new Error("No patched picomatch installation found");
  return installed;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const count = await applyPatches(frontend, process.argv.includes("--check"));
    console.log(`Picomatch ${patch.version} security patch verified (${count} installation(s))`);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
