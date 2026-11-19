// Real Chromium acceptance with only Node built-ins and the Chrome DevTools Protocol.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const net = require("node:net");
const { createSHA256, sha256 } = require(path.join(__dirname, "..", "web", "node_modules", "hash-wasm"));
const { spawn, spawnSync } = require("node:child_process");

const delay = (ms) => new Promise(resolve => setTimeout(resolve, ms));
const cdpTimeout = Number(process.env.OKSERVER_CDP_TIMEOUT_MS || 45000);
async function until(callback, timeout = 30000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    let result;
    try { result = await callback(); } catch (error) {
      if (String(error).includes("CDP timeout")) throw error;
      result = false;
    }
    if (result) return result;
    await delay(100);
  }
  throw new Error("acceptance condition timed out");
}
async function unusedPort() {
  const server = net.createServer();
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}
async function connectCDP(address) {
  const socket = new WebSocket(address);
  await new Promise((resolve, reject) => {
    socket.addEventListener("open", resolve, { once: true });
    socket.addEventListener("error", reject, { once: true });
  });
  const pending = new Map();
  const events = [];
  socket.addEventListener("close", () => {
    for (const item of pending.values()) {
      clearTimeout(item.timer);
      item.reject(new Error("CDP connection closed"));
    }
    pending.clear();
  });
  let sequence = 0;
  socket.addEventListener("message", event => {
    const message = JSON.parse(event.data);
    if (message.method) {
      let event;
      if (message.method.startsWith("Target.") || message.method === "Inspector.detached") {
        event = { method: message.method, params: message.params };
      } else if (message.method === "Runtime.exceptionThrown") {
        event = { method: message.method, text: message.params.exceptionDetails?.text };
      } else if (message.method === "Runtime.consoleAPICalled") {
        event = { method: message.method, type: message.params.type,
          args: message.params.args?.map(arg => arg.value ?? arg.description).slice(0, 4) };
      } else if (message.method === "Network.requestWillBeSent") {
        event = { method: message.method, request: message.params.request?.method + " " + message.params.request?.url };
      } else if (message.method === "Network.responseReceived") {
        event = { method: message.method, status: message.params.response?.status, url: message.params.response?.url };
      } else if (message.method === "Page.frameNavigated") {
        event = { method: message.method, url: message.params.frame?.url };
      } else if (message.method === "Debugger.paused") {
        event = { method: message.method, frames: message.params.callFrames?.slice(0,12).map(frame=>({function:frame.functionName,url:frame.url,line:frame.location.lineNumber})) };
      }
      if (event) events.push(event);
      if (events.length > 200) events.shift();
    }
    const item = pending.get(message.id);
    if (!item) return;
    pending.delete(message.id);
    clearTimeout(item.timer);
    if (message.error) item.reject(new Error(JSON.stringify(message.error)));
    else item.resolve(message.result);
  });
  return {
    events,
    close: () => socket.close(),
    send(method, params = {}, sessionId) {
      const id = ++sequence;
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
          pending.delete(id); reject(new Error("CDP timeout: " + method));
        }, cdpTimeout);
        pending.set(id, { resolve, reject, timer });
        socket.send(JSON.stringify({ id, method, params, sessionId }));
      });
    },
  };
}
async function terminate(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) return;
  const exited = new Promise(resolve => child.once("exit", resolve));
  child.kill();
  await Promise.race([exited, delay(5000)]);
  if (child.exitCode === null && child.signalCode === null) {
    child.kill("SIGKILL");
    await Promise.race([exited, delay(5000)]);
    if (child.exitCode === null && child.signalCode === null) throw new Error("Test child did not terminate");
  }
}
function browserExecutable() {
  const candidates = process.env.CHROME_PATH ? [process.env.CHROME_PATH] :
    process.platform === "win32" ? [
      path.join(process.env.ProgramFiles || "C:/Program Files", "Google/Chrome/Application/chrome.exe"),
      "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
    ] : ["/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser",
      "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"];
  const executable = candidates.find(candidate => fs.existsSync(candidate));
  if (!executable) throw new Error("Set CHROME_PATH to an installed Chromium browser");
  return executable;
}

const fixtureMB = Number(process.env.OKSERVER_BROWSER_MB || 32);
const transferTimeout = Math.max(60000, fixtureMB * 150);

test("real browser large-file hashing, retry, cancellation and server/browser restart resume",
  { timeout: Math.max(240000, fixtureMB * 600) }, async (t) => {
  const scratch = fs.mkdtempSync(path.join(os.tmpdir(), "okserver-browser-test-"));
  const externalURL = process.env.OKSERVER_BROWSER_URL;
  assert.ok(!externalURL || process.env.OKSERVER_BROWSER_ROOT, "External server requires its dedicated test storage root");
  const root = process.env.OKSERVER_BROWSER_ROOT || path.join(scratch, "root");
  const profile = path.join(scratch, "profile");
  fs.mkdirSync(root, { recursive: true });
  const binary = path.join(scratch, process.platform === "win32" ? "server.exe" : "server");
  let server, browser, cdp, hmrFile, debugPort, cdpTargetId, session;
  let base;
  let stage = "setup";
  let serverOutput = "";
  let stopMonitor;
  if (process.env.OKSERVER_MEASURE_RESOURCES === "1") {
    stopMonitor = require("./resource-monitor.cjs").monitor(root, async () => {
      return { server: [server?.pid], browser: [browser?.pid] };
    });
  }
  try {
    if (!externalURL) {
      const build = spawnSync("go", ["build", "-o", binary, "./cmd/okserver"], {
      cwd: path.resolve(__dirname, ".."), encoding: "utf8", timeout: 120000,
    });
      assert.equal(build.status, 0, build.stderr);
    }
    for (let index = 0; index < 240; index++) {
      fs.writeFileSync(path.join(root, `entry-${String(index).padStart(3, "0")}.txt`), "entry");
    }
    fs.mkdirSync(path.join(root, "nested space"));
    fs.writeFileSync(path.join(root, "nested space", "inside.txt"), "nested");
    const fixture = path.join(scratch, "browser.bin");
    const megabytes = fixtureMB;
    assert.ok(Number.isInteger(megabytes) && megabytes >= 16 && megabytes <= 16384);
    const block = Buffer.alloc(1024 * 1024);
    for (let index = 0; index < block.length; index++) block[index] = index % 256;
    const digest = await createSHA256();
    const descriptor = fs.openSync(fixture, "w");
    for (let index = 0; index < megabytes; index++) { fs.writeSync(descriptor, block); digest.update(block); }
    fs.closeSync(descriptor);
    const expected = digest.digest();
    const port = await unusedPort();
    base = externalURL || `http://127.0.0.1:${port}`;
    const config = path.join(scratch, "server.yaml");
    fs.writeFileSync(config, "{}");
    const startServer = () => {
      const child = spawn(binary, ["-c", config, "-s", `127.0.0.1:${port}`, "-p", root], { stdio: ["ignore", "pipe", "pipe"], windowsHide: true });
      child.stdout.setEncoding("utf8").on("data", value => { serverOutput = (serverOutput + value).slice(-65536); });
      child.stderr.setEncoding("utf8").on("data", value => { serverOutput = (serverOutput + value).slice(-65536); });
      return child;
    };
    if (!externalURL) server = startServer();
    stage = "server health check";
    await until(async () => {
      try { return (await fetch(base + "/healthz")).ok; } catch { return false; }
    });
    browser = spawn(browserExecutable(), ["--headless=new", "--no-sandbox",
      "--disable-gpu", "--disable-background-networking", "--no-first-run",
      "--remote-debugging-port=0", "--user-data-dir=" + profile], { stdio: "ignore", windowsHide: true });
    const activePort = path.join(profile, "DevToolsActivePort");
    stage = "browser debug port startup";
    await until(() => fs.existsSync(activePort));
    debugPort = fs.readFileSync(activePort, "utf8").split(/\r?\n/)[0];
    const version = await (await fetch(`http://127.0.0.1:${debugPort}/json/version`)).json();
    cdp = await connectCDP(version.webSocketDebuggerUrl);
    const target = await cdp.send("Target.createTarget", { url: base });
    cdpTargetId = target.targetId;
    const attached = await cdp.send("Target.attachToTarget", { targetId: cdpTargetId, flatten: true });
    session = attached.sessionId;
    const send = (method, params) => cdp.send(method, params, session);
    const evaluate = async expression => {
      const code = expression.includes("\n") ? "(async()=>{" + expression + "})()" : expression;
      const result = await send("Runtime.evaluate", { expression:code, awaitPromise: true, returnByValue: true });
      if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails));
      return result.result.value;
    };
    await send("Page.enable");
    await send("Network.enable");
    await send("Runtime.enable");
    await send("Debugger.enable");
    stage = "initial page render";
    await until(() => evaluate("document.querySelectorAll('#listing tr').length === 100"));
    const workerAsset = fs.readdirSync(path.resolve(__dirname, "../internal/webui/assets/assets"))
      .find(name => name.startsWith("hash.worker-") && name.endsWith(".js"));
    assert.ok(workerAsset, "embedded checksum worker is missing");
    const workerPrefixSize = 8 << 20;
    const expectedChunkDigest = await sha256("worker");
    const expectedWorkerDigest = await sha256(Buffer.concat([Buffer.alloc(workerPrefixSize, 17), Buffer.from("worker")]));
    await evaluate(`new Promise((resolve,reject)=>{const worker=new Worker('/assets/${workerAsset}',{type:'module'});
      const timeout=setTimeout(()=>finish(new Error('checksum worker timed out')),10000);
      const finish=error=>{clearTimeout(timeout);worker.terminate();error?reject(error):resolve(true);};
      worker.onerror=event=>finish(new Error(event.message||'checksum worker failed'));
      worker.onmessage=event=>{const message=event.data;if(message.type==='error'){finish(new Error(message.message));return;}
        if(message.type==='ready'){worker.postMessage({id:2,type:'process',start:${workerPrefixSize},end:${workerPrefixSize + 6}});return;}
        if(message.type==='chunk'){if(message.checksum!=='${expectedChunkDigest}'||new TextDecoder().decode(new Uint8Array(message.buffer))!=='worker'){
          finish(new Error('checksum worker chunk mismatch'));return;}worker.postMessage({id:3,type:'digest'});return;}
        if(message.type==='digest')finish(message.digest==='${expectedWorkerDigest}'?undefined:new Error('checksum worker full digest mismatch'));};
      worker.postMessage({id:1,type:'reset',file:new Blob([new Uint8Array(${workerPrefixSize}).fill(17),'worker']),offset:${workerPrefixSize}});})`);
    if (process.env.OKSERVER_BROWSER_WORKER_ONLY === "1") return;
    await evaluate("document.getElementById('language-toggle').click()");
    await until(() => evaluate("document.documentElement.lang==='en' && document.querySelector('h1').textContent==='File workspace'"));
    await evaluate("document.getElementById('theme-toggle').click()");
    await until(() => evaluate("document.documentElement.classList.contains('dark')"));
    assert.equal(await evaluate("document.getElementById('toggle-transfers')===null"), true);
    await evaluate("Array.from(document.querySelectorAll('.entry-name')).find(e=>e.textContent==='nested space').click()");
    await until(() => evaluate("location.pathname.endsWith('/nested%20space') && document.getElementById('listing').textContent.includes('inside.txt')"));
    await send("Page.reload");
    await until(() => evaluate("document.getElementById('listing')?.textContent.includes('inside.txt') && document.documentElement.lang==='en' && document.documentElement.classList.contains('dark')"));
    await evaluate("document.querySelector('#breadcrumbs a').click()");
    await until(() => evaluate("document.querySelectorAll('#listing tr').length === 100"));
    await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 1, mobile: true });
    assert.equal(await evaluate("document.documentElement.scrollWidth <= innerWidth"), true, "mobile page overflows horizontally");
    await send("Emulation.clearDeviceMetricsOverride");
    if (process.env.OKSERVER_BROWSER_HMR) {
      const name = `hmr-probe-${process.pid}.css`;
      hmrFile = path.resolve(__dirname, "../web/src", name);
      fs.writeFileSync(hmrFile, ":root{--okserver-test-token:1}");
      await evaluate(`import('/src/${name}')`);
      await until(() => evaluate("getComputedStyle(document.documentElement).getPropertyValue('--okserver-test-token').trim()==='1'"));
      fs.writeFileSync(hmrFile, ":root{--okserver-test-token:2}");
      await until(() => evaluate("getComputedStyle(document.documentElement).getPropertyValue('--okserver-test-token').trim()==='2'"));
    }
    await evaluate("document.getElementById('next-page').click()");
    await until(() => evaluate("document.getElementById('page-summary').textContent.includes('2') && document.querySelectorAll('#listing tr').length === 100"));
    await evaluate("Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(document.getElementById('search-files'),'entry-239'); document.getElementById('search-files').dispatchEvent(new Event('input',{bubbles:true}))");
    await until(() => evaluate("document.querySelectorAll('#listing tr').length===1 && document.getElementById('listing').textContent.includes('entry-239')"));
    await evaluate("Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(document.getElementById('search-files'),'');document.getElementById('search-files').dispatchEvent(new Event('input',{bubbles:true}))");
    await until(() => evaluate("document.querySelectorAll('#listing tr').length === 100"));
    const selectFile = async () => {
      // React/HMR can replace the document between CDP calls. Resolve a fresh
      // node on each attempt; propagate errors unrelated to a detached node.
      for (let attempt = 0; ; attempt++) {
        try {
          const document = await send("DOM.getDocument");
          const input = await send("DOM.querySelector", { nodeId: document.root.nodeId, selector: "#files" });
          if (!input.nodeId) throw new Error("Upload input is not mounted");
          await send("DOM.setFileInputFiles", { nodeId: input.nodeId, files: [fixture] });
          return;
        } catch (error) {
          if (attempt >= 9 || !/Could not find node|No node with given id|Upload input is not mounted/.test(error.message)) throw error;
          await delay(100);
        }
      }
    };

    stage = "browser upload cancellation";
    await send("Page.navigate", { url: base + "/tasks" });
    await until(() => evaluate("document.querySelector('[data-testid=event-status]')!==null"));
    await evaluate(`await fetch('/upload/cancel.bin',{method:'PUT',headers:{'Content-Range':'bytes 0-0/123'},body:'a'});
      document.getElementById('refresh-tasks').click();`);
    await until(() => evaluate("document.querySelector('article button')!==null"));
    await evaluate("document.querySelector('article button').click()");
    await until(async () => (await (await fetch(base + "/api/v1/uploads")).json()).uploads.length === 0);

    await send("Page.navigate", { url: base });
    await until(() => evaluate("document.getElementById('open-upload')!==null"));
    await evaluate("document.getElementById('open-upload').click()");
    assert.equal(await evaluate("document.querySelector('dialog').open"), true);
    await selectFile();
    stage = "browser upload pause and worker response";
    await evaluate(`window.testHashWorkerMessages=[];const NativeWorker=window.Worker;
      window.Worker=class extends NativeWorker{constructor(...args){super(...args);
        this.addEventListener('message',event=>window.testHashWorkerMessages.push({type:event.data.type,offset:event.data.offset,message:event.data.message}));
        this.addEventListener('error',event=>window.testHashWorkerMessages.push({type:'error',message:event.message}));}};`);
    await evaluate(`window.testOriginalFetch=window.fetch;window.testPaused=false;
      window.fetch=async(...args)=>{const response=await testOriginalFetch(...args);
        if(args[1]?.method==='PUT'&&!testPaused){testPaused=true;document.getElementById('stop-upload').click();}
        return response;};setTimeout(()=>document.getElementById('upload').click(),0);`);
    await until(async () => {
      const tasks = await (await fetch(base + "/api/v1/uploads")).json();
      return tasks.uploads.some(task => task.path === "browser.bin" && task.offset >= 8 * 1024 * 1024);
    }, transferTimeout);
    await delay(500);
    const tasks = await (await fetch(base + "/api/v1/uploads")).json();
    assert.equal(tasks.uploads[0].offset, 8 * 1024 * 1024);
    if (!externalURL) {
      stage = "server restart with partial browser upload";
      await terminate(server);
      server = startServer();
      await until(async () => (await fetch(base + "/healthz")).ok);
      const restored = await (await fetch(base + "/api/v1/uploads/" + tasks.uploads[0].id)).json();
      assert.equal(restored.offset, tasks.uploads[0].offset, "restart lost acknowledged progress");
      t.diagnostic("Server restart preserved the upload ID and acknowledged offset");
    }
    stage = "browser restart and upload resume";
    cdp.close();
    cdp = undefined;
    await terminate(browser);
    fs.rmSync(activePort, { force: true });
    browser = spawn(browserExecutable(), ["--headless=new", "--no-sandbox",
      "--disable-gpu", "--disable-background-networking", "--no-first-run",
      "--remote-debugging-port=0", "--user-data-dir=" + profile], { stdio: "ignore", windowsHide: true });
    await until(() => fs.existsSync(activePort));
    debugPort = fs.readFileSync(activePort, "utf8").split(/\r?\n/)[0];
    const resumedVersion = await (await fetch(`http://127.0.0.1:${debugPort}/json/version`)).json();
    cdp = await connectCDP(resumedVersion.webSocketDebuggerUrl);
    const resumedTarget = await cdp.send("Target.createTarget", { url: base });
    cdpTargetId = resumedTarget.targetId;
    const resumedSession = await cdp.send("Target.attachToTarget", { targetId: cdpTargetId, flatten: true });
    session = resumedSession.sessionId;
    await send("Page.enable");
    await send("Network.enable");
    await send("Runtime.enable");
    await send("Debugger.enable");
    await until(() => evaluate("document.querySelectorAll('#listing tr').length===100"));
    await evaluate("document.getElementById('open-upload').click()");
    await selectFile();
    stage = "resumed upload recovery";
    await evaluate(`window.testOriginalFetch=window.fetch;window.testChunkRequests=0;
      window.fetch=async(...args)=>{
        if(args[1]?.method!=='PUT')return testOriginalFetch(...args);
        if(testChunkRequests++===0)throw new TypeError('test transient network failure');
        const response=await testOriginalFetch(...args);
        if(testChunkRequests===2)throw new TypeError('test lost chunk acknowledgement');
        return response;
      };setTimeout(()=>document.getElementById('upload').click(),0);`);
    await until(async () => {
      const tasks = await (await fetch(base + "/api/v1/uploads")).json();
      return !tasks.uploads.some(task => task.path === "browser.bin") &&
        fs.existsSync(path.join(root, "browser.bin")) &&
        fs.statSync(path.join(root, "browser.bin")).size === megabytes * 1024 * 1024;
    }, transferTimeout);
    const uploadedHash = await createSHA256();
    for await (const chunk of fs.createReadStream(path.join(root, "browser.bin"))) uploadedHash.update(chunk);
    const actual = uploadedHash.digest();
    assert.equal(actual, expected);

    stage = "verified parallel browser download";
    cdp.close();
    cdp = await connectCDP(resumedVersion.webSocketDebuggerUrl);
    const downloadSession = await cdp.send("Target.attachToTarget", { targetId: cdpTargetId, flatten: true });
    session = downloadSession.sessionId;
    await send("Page.reload");
    await until(()=>evaluate("document.querySelectorAll('#listing tr').length===100"));
    stage = "install browser download probe";
    await evaluate(`const nativeDownloadFetch=window.fetch;window.testRanges=0;
      window.fetch=(...args)=>{if(args[1]?.headers?.Range)window.testRanges++;return nativeDownloadFetch(...args);};
      window.showSaveFilePicker=async()=>{const root=await navigator.storage.getDirectory();const handle=await root.getFileHandle('browser-download.bin',{create:true});window.testDownloadHandle=handle;return handle;};
      const original=await (await navigator.storage.getDirectory()).getFileHandle('browser-download.bin',{create:true});
      const originalWriter=await original.createWritable();await originalWriter.write('original destination');await originalWriter.close();
      window.pauseDownloadObserver=new MutationObserver(()=>{if(Array.from(document.querySelectorAll('[role=status]')).some(e=>/1\\/\\d+$/.test(e.textContent))){
        window.pauseDownloadObserver.disconnect();Array.from(document.querySelectorAll('button')).find(e=>e.textContent.toLowerCase().includes('cancel')&&e.textContent.toLowerCase().includes('download'))?.click();}});
      window.pauseDownloadObserver.observe(document.body,{subtree:true,childList:true,characterData:true});
      document.getElementById('refresh-files').click();`);
    stage = "locate browser download action";
    await until(()=>evaluate("Array.from(document.querySelectorAll('a.row-action')).some(e=>e.getAttribute('aria-label')?.endsWith('browser.bin'))"));
    await evaluate("Array.from(document.querySelectorAll('a.row-action')).find(e=>e.getAttribute('aria-label')?.endsWith('browser.bin')).click()");
    stage = "durable download pause preserves original";
    await until(()=>evaluate("Array.from(document.querySelectorAll('[role=status]')).some(e=>e.textContent.includes('Paused'))"), transferTimeout);
    assert.equal(await evaluate("(async()=>await (await testDownloadHandle.getFile()).text())()"),"original destination");
    await send("Page.reload");
    await until(()=>evaluate("document.querySelectorAll('#listing tr').length===100"));
    await evaluate(`const originalFetch=window.fetch;window.testRanges=0;
      window.fetch=(...args)=>{if(args[1]?.headers?.Range)window.testRanges++;return originalFetch(...args);};
      window.showSaveFilePicker=async()=>{const root=await navigator.storage.getDirectory();const handle=await root.getFileHandle('browser-download.bin',{create:true});window.testDownloadHandle=handle;return handle;};
      document.getElementById('refresh-files').click();`);
    await until(()=>evaluate("Array.from(document.querySelectorAll('a.row-action')).some(e=>e.getAttribute('aria-label')?.endsWith('browser.bin'))"));
    await evaluate("Array.from(document.querySelectorAll('a.row-action')).find(e=>e.getAttribute('aria-label')?.endsWith('browser.bin')).click()");
    stage = "wait browser download verification";
    await until(()=>evaluate("Array.from(document.querySelectorAll('[role=status]')).some(e=>e.textContent.includes('SHA-256 ✓'))"), transferTimeout);
    assert.ok(await evaluate(`testRanges>0 && testRanges<${Math.ceil(megabytes/8)}`),"browser failed to reuse durable ranges");
    assert.equal(await evaluate("(async()=> (await testDownloadHandle.getFile()).size)()"),megabytes*1024*1024);
    console.log(`Browser verified ${megabytes} MiB, SHA-256 ${actual}`);
  } catch (error) {
    if(cdp && error.message?.includes("CDP timeout")) {try{await cdp.send("Debugger.pause",{},session);}catch{}await delay(100);}
    let diagnostic = "";
    try { diagnostic += `\nServer uploads: ${JSON.stringify(await (await fetch(base + "/api/v1/uploads")).json())}`; } catch (cause) { diagnostic += `\nServer task query error: ${cause}`; }
    diagnostic += `\nserver exit=${server?.exitCode} signal=${server?.signalCode} browser exit=${browser?.exitCode} signal=${browser?.signalCode}`;
    diagnostic += `\nserver output tail: ${serverOutput.split(/\r?\n/).slice(-12).join(" ").slice(-5000)}`;
    diagnostic += `\nCDP events: ${JSON.stringify(cdp?.events.slice(-50))}`;
    t.diagnostic(diagnostic.replace(/\s+/g, " "));
    throw new Error(`${error instanceof Error ? error.message : String(error)} | stage=${stage} | ${diagnostic.replace(/\s+/g, " ")}`);
  } finally {
    if (stopMonitor) console.log("Resource samples: " + JSON.stringify(await stopMonitor()));
    if (hmrFile) fs.rmSync(hmrFile, { force: true });
    if (cdp) {
      if (stage !== "resumed upload recovery") { try { await cdp.send("Browser.close"); } catch {} }
      cdp.close();
    }
    await terminate(browser);
    await terminate(server);
    const cleanupTarget = path.resolve(scratch);
    assert.equal(path.dirname(cleanupTarget), path.resolve(os.tmpdir()));
    assert.ok(path.basename(cleanupTarget).startsWith("okserver-browser-test-"));
    // Keep the event loop free while Chromium descendants finish closing profile files.
    await fs.promises.rm(cleanupTarget, { recursive: true, force: true, maxRetries: 20, retryDelay: 100 });
  }
});
