const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const net = require("node:net");
const crypto = require("node:crypto");
const { spawn, spawnSync } = require("node:child_process");
const { monitor } = require("./resource-monitor.cjs");
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

test("large CLI upload and parallel verified download use bounded buffers", { timeout: 600000 }, async () => {
  const scratch = fs.mkdtempSync(path.join(os.tmpdir(), "okserver-cli-large-"));
  const root = path.join(scratch, "storage");
  const executable = path.join(scratch, process.platform === "win32" ? "okserver.exe" : "okserver");
  const megabytes = Number(process.env.OKSERVER_CLI_MB || 64);
  assert.ok(Number.isInteger(megabytes) && megabytes > 0 && megabytes <= 4096);
  let server, client, stop;
  try {
    const build = spawnSync("go", ["build", "-o", executable, "./cmd/okserver"], { cwd: path.resolve(__dirname, ".."), encoding: "utf8", timeout: 120000 });
    assert.equal(build.status, 0, build.stderr);
    const input = path.join(scratch, "source.bin");
    const descriptor = fs.openSync(input, "w");
    const block = Buffer.alloc(1 << 20);
    for (let index = 0; index < block.length; index++) block[index] = index % 251;
    const hash = crypto.createHash("sha256");
    for (let index = 0; index < megabytes; index++) { fs.writeSync(descriptor, block); hash.update(block); }
    fs.closeSync(descriptor);
    const expected = hash.digest("hex");
    const listener = net.createServer();
    await new Promise(resolve => listener.listen(0, "127.0.0.1", resolve));
    const port = listener.address().port;
    await new Promise(resolve => listener.close(resolve));
    const config = path.join(scratch, "server.yaml");
    fs.writeFileSync(config, "upload:\n  min_free_disk_bytes: 0\n");
    server = spawn(executable, ["-c", config, "-s", `127.0.0.1:${port}`, "-p", root], { stdio: "ignore", windowsHide: true });
    const base = `http://127.0.0.1:${port}`;
    for (let attempt = 0; ; attempt++) {
      try { if ((await fetch(base + "/healthz")).ok) break; } catch {}
      if (attempt === 200) throw new Error("server startup failed");
      await delay(50);
    }
    stop = monitor(root, async () => ({ server: [server.pid], browser: [client?.pid] }));
    const run = async args => {
      const started = Date.now();
      client = spawn(executable, ["cli", "-t", "2m", ...args], {
        stdio: ["ignore", "pipe", "pipe"], windowsHide: true,
        env: { ...process.env, OKSERVER_CACHE_DIR: path.join(scratch, "cache") },
      });
      let output = "";
      client.stdout.on("data", data => output = (output + data).slice(-4096));
      client.stderr.on("data", data => output = (output + data).slice(-4096));
      const code = await new Promise((resolve, reject) => { client.once("error", reject); client.once("exit", resolve); });
      assert.equal(code, 0, output);
      return (Date.now() - started) / 1000;
    };
    const uploadSeconds = await run(["-c", "-d", "@" + input, base + "/upload/large.bin"]);
    const download = path.join(scratch, "download.bin");
    const downloadSeconds = await run(["-c", "--parallel", "4", "-o", download, base + "/download/large.bin"]);
    const result = crypto.createHash("sha256");
    for await (const chunk of fs.createReadStream(download)) result.update(chunk);
    assert.equal(result.digest("hex"), expected);
    const metrics = await stop(); stop = undefined;
    console.log(JSON.stringify({ megabytes, uploadSeconds, downloadSeconds, serverSampledRSS: metrics.serverRSS, clientSampledRSS: metrics.browserAggregateRSS, stagingSampledBytes: metrics.stagingBytes, samples: metrics.samples, sha256: expected }));
  } finally {
    if (stop) await stop();
    for (const child of [client, server]) {
      if (child && child.exitCode === null && child.signalCode === null) {
        const exited = new Promise(resolve => child.once("exit", resolve));
        child.kill(); await exited;
      }
    }
    assert.equal(path.dirname(path.resolve(scratch)), path.resolve(os.tmpdir()));
    assert.ok(path.basename(scratch).startsWith("okserver-cli-large-"));
    await fs.promises.rm(scratch, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  }
});
