const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const net = require("node:net");
const { spawn, spawnSync } = require("node:child_process");
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

test("standalone embedded binary, version, authentication and invalid configuration", { timeout: 180000 }, async () => {
  const scratch = fs.mkdtempSync(path.join(os.tmpdir(), "okserver-release-test-"));
  let server;
  try {
    const binary = path.join(scratch, process.platform === "win32" ? "okserver.exe" : "okserver");
    const build = spawnSync("go", ["build", "-buildvcs=true", "-trimpath", "-ldflags", "-X main.version=v0.0.0-acceptance", "-o", binary, "./cmd/okserver"], {
      cwd: path.resolve(__dirname, ".."), encoding: "utf8", timeout: 120000, windowsHide: true,
    });
    assert.equal(build.status, 0, build.stderr);
    const version = spawnSync(binary, ["--version"], { cwd: scratch, encoding: "utf8", windowsHide: true });
    assert.equal(version.status, 0, version.stderr);
    assert.match(version.stdout, /^v0\.0\.0-acceptance \(commit .+\)\n$/);
    const generatedDirectory = path.join(scratch, "generated");
    fs.mkdirSync(generatedDirectory);
    const generate = (...args) => spawnSync(binary, ["-g", ...args], { cwd: generatedDirectory, encoding: "utf8", timeout: 5000, windowsHide: true });
    const generated = generate();
    assert.equal(generated.status, 0, generated.stderr);
    const defaultPath = path.join(generatedDirectory, "server.yaml");
    const generatedData = fs.readFileSync(defaultPath, "utf8");
    assert.match(generatedData, /^server:/m);
    assert.match(generatedData, /^upload:/m);
    assert.match(generatedData, /^log:/m);
    assert.equal(generate().status, 1, "generation must reject an existing file");
    assert.equal(fs.readFileSync(defaultPath, "utf8"), generatedData);
    const customPath = path.join(generatedDirectory, "custom config.yaml");
    const custom = generate("-c", customPath);
    assert.equal(custom.status, 0, custom.stderr);
    assert.equal(fs.readFileSync(customPath, "utf8"), generatedData);
    assert.deepEqual(fs.readdirSync(generatedDirectory).sort(), ["custom config.yaml", "server.yaml"]);
    const config = path.join(scratch, "config.yaml");
    fs.writeFileSync(config, "upload:\n  idle_timeout: invalid\n");
    const invalid = spawnSync(binary, ["-c", config], { cwd: scratch, encoding: "utf8", windowsHide: true });
    assert.equal(invalid.status, 1);
    assert.match(invalid.stderr, /upload.idle_timeout must be a positive duration/);
    const missing = spawnSync(binary, ["-c", path.join(scratch, "missing.yaml")], { cwd: scratch, encoding: "utf8", windowsHide: true });
    assert.notEqual(missing.status, 0);
    assert.match(missing.stderr, /read configuration/);
    fs.writeFileSync(config, "listen: ':28080'\nlog:\n  logger:\n    filename: old.log\n");
    const legacy = spawnSync(binary, ["-c", config], { cwd: scratch, encoding: "utf8", timeout: 5000, windowsHide: true });
    assert.equal(legacy.status, 1, "legacy configuration must fail instead of falling back to defaults");
    assert.match(legacy.stderr, /expected server, upload, download and log sections/);
    fs.writeFileSync(config, "server:\n  auth: acceptance:private-password\nlog:\n  console:\n    format: json\n");
    const listener = net.createServer();
    await new Promise(resolve => listener.listen(0, "127.0.0.1", resolve));
    const port = listener.address().port;
    await new Promise(resolve => listener.close(resolve));
    const base = `http://127.0.0.1:${port}`;
    const root = path.join(scratch, "storage");
    let output = "";
    let consoleLogs = "";
    server = spawn(binary, ["-c", config, "-s", `127.0.0.1:${port}`, "-p", root], { cwd: scratch, windowsHide: true });
    server.stdout.on("data", data => { output += data; consoleLogs += data; });
    server.stderr.on("data", data => output += data);
    const headers = { Authorization: "Basic " + Buffer.from("acceptance:private-password").toString("base64") };
    let ready = false;
    for (let attempt = 0; attempt < 100; attempt++) {
      try { ready = (await fetch(base + "/readyz", { headers })).ok; } catch {}
      if (ready) break;
      if (server.exitCode !== null) throw new Error(output);
      await delay(100);
    }
    assert.ok(ready, output);
    const page = await fetch(base + "/files/nested", { headers });
    assert.equal(page.status, 200);
    const html = await page.text();
    const assets = [...html.matchAll(/(?:src|href)="([^"]+\.(?:js|css))"/g)].map(match => match[1]);
    assert.ok(assets.length >= 2, "missing embedded JS/CSS references");
    for (const asset of assets) {
      const response = await fetch(new URL(asset, base));
      assert.equal(response.status, 200, asset);
      assert.ok((await response.arrayBuffer()).byteLength > 0, asset);
    }
    assert.match(html, /type="module"/);
    assert.equal((await fetch(base + "/api/v1/files")).status, 401);
    assert.equal((await fetch(base + "/api/v1/files", { headers })).status, 200);
    assert.ok(!output.includes("private-password"), "startup output leaked authentication");
    assert.match(output, /server.started/);
    const records = consoleLogs.trim().split(/\r?\n/).filter(Boolean).map(line => JSON.parse(line));
    assert.ok(records.some(record => record.msg === "server.started"), "console JSON was mixed with startup text");

    const stopped = new Promise(resolve => server.once("exit", resolve));
    server.kill();
    await stopped;
    // No server.yaml, no -c and no -p: built-in defaults create ./uploads.
    let defaultOutput = "";
    server = spawn(binary, ["-s", `127.0.0.1:${port}`], { cwd: scratch, windowsHide: true });
    server.stdout.on("data", data => defaultOutput += data);
    server.stderr.on("data", data => defaultOutput += data);
    ready = false;
    for (let attempt = 0; attempt < 100; attempt++) {
      try { ready = (await fetch(base + "/readyz")).ok; } catch {}
      if (ready) break;
      if (server.exitCode !== null) throw new Error(`default-config server stopped: ${defaultOutput}`);
      await delay(100);
    }
    assert.ok(ready, "default-config server did not become ready");
    assert.ok(fs.statSync(path.join(scratch, "uploads")).isDirectory());
    assert.equal((await fetch(base + "/api/v1/files")).status, 200);
  } finally {
    if (server && server.exitCode === null) {
      const stopped = new Promise(resolve => server.once("exit", resolve));
      server.kill();
      await stopped;
    }
    const target = path.resolve(scratch);
    assert.equal(path.dirname(target), path.resolve(os.tmpdir()));
    assert.ok(path.basename(target).startsWith("okserver-release-test-"));
    fs.rmSync(target, { recursive: true, force: true, maxRetries: 10, retryDelay: 200 });
  }
});
