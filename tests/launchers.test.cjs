const test = require("node:test");
const assert = require("node:assert/strict");
const { fork, spawn } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");
const os = require("node:os");
const net = require("node:net");
const project = path.resolve(__dirname, "..");
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

async function freePort() {
  const server = net.createServer();
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}
async function stopped(port) {
  for (let attempt = 0; attempt < 50; attempt++) {
    try { await fetch(`http://127.0.0.1:${port}/healthz`, { signal: AbortSignal.timeout(300) }); } catch { return; }
    await delay(100);
  }
  throw new Error(`Port ${port} remained active after shutdown`);
}
async function run(command, args, options = {}) {
  const child = spawn(command, args, { windowsHide: true, ...options });
  let output = "";
  child.stdout?.on("data", data => output += data);
  child.stderr?.on("data", data => output += data);
  const code = await new Promise((resolve, reject) => { child.once("error", reject); child.once("exit", resolve); });
  assert.equal(code, 0, output);
  return output;
}

test("Windows batch build from another directory", { skip: process.platform !== "win32", timeout: 180000 }, async () => {
  const output = await run(process.env.ComSpec || "cmd.exe", ["/d", "/s", "/c", `""${path.join(project, "start-webui-build.bat")}" --build-only"`], { cwd: os.tmpdir(), windowsVerbatimArguments: true });
  assert.match(output, /Building/);
  assert.ok(fs.existsSync(path.join(project, "bin/okserver-build.exe")));
});

test("development launcher proxies API, supports browser recovery and stops both children", { timeout: 240000 }, async () => {
  const scratch = fs.mkdtempSync(path.join(os.tmpdir(), "okserver-launcher-test-"));
  const frontendPort = await freePort();
  let apiPort = await freePort();
  while (apiPort === frontendPort) apiPort = await freePort();
  const root = path.join(scratch, "root");
  const config = path.join(scratch, "server.yaml");
  fs.writeFileSync(config, "upload:\n  max_concurrent: 4\n  min_free_disk_bytes: 0\n");
  const child = fork(path.join(project, "scripts/webui-start.mjs"), ["dev", String(frontendPort), String(apiPort), config, root, "--no-open"], { cwd: os.tmpdir(), stdio: ["ignore", "pipe", "pipe", "ipc"], windowsHide: true });
  let output = "";
  child.stdout.on("data", data => output += data);
  child.stderr.on("data", data => output += data);
  const exited = new Promise(resolve => child.once("exit", resolve));
  try {
    for (let attempt = 0; !output.includes("[READY]"); attempt++) {
      if (attempt > 600 || child.exitCode !== null) throw new Error(output || "Launcher failed to start");
      await delay(100);
    }
    const url = `http://127.0.0.1:${frontendPort}`;
    const html = await (await fetch(url)).text();
    assert.match(html, /@vite\/client/);
    assert.equal((await fetch(url + "/api/v1/files")).status, 200);
    const browserEnv = { ...process.env, OKSERVER_BROWSER_URL: url, OKSERVER_BROWSER_ROOT: root, OKSERVER_BROWSER_STATE_ROOT: path.join(project, "bin", ".okserver"), OKSERVER_BROWSER_HMR: "1" };
    delete browserEnv.NODE_TEST_CONTEXT;
    const browserOutput = await run(process.execPath, ["--test", path.join(project, "tests/browser.test.cjs")], { cwd: project, env: browserEnv });
    assert.match(browserOutput, /Browser verified/);
    console.log(browserOutput);
  } catch (error) {
    console.error("Launcher diagnostics:", output.slice(-12000));
    throw error;
  } finally {
    if (child.connected) child.send("stop");
    await Promise.race([exited, delay(10000)]);
    if (child.exitCode === null) { child.kill(); throw new Error("Launcher did not stop gracefully"); }
    await Promise.all([stopped(frontendPort), stopped(apiPort)]);
    const target = path.resolve(scratch);
    assert.equal(path.dirname(target), path.resolve(os.tmpdir()));
    assert.ok(path.basename(target).startsWith("okserver-launcher-test-"));
    fs.rmSync(target, { recursive: true, force: true, maxRetries: 10, retryDelay: 200 });
  }
});
