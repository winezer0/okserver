// Optional acceptance telemetry. Working sets include shared pages per process.
const fs = require("node:fs");
const path = require("node:path");
const { execFile } = require("node:child_process");
const { promisify } = require("node:util");
const execute = promisify(execFile);

async function memorySnapshot() {
  if (process.platform === "win32") {
    const script = "Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,WorkingSetSize | ConvertTo-Json -Compress";
    const { stdout } = await execute("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", script], { windowsHide: true, timeout: 10000 });
    const rows = JSON.parse(stdout);
    return rows.map(row => ({pid: Number(row.ProcessId), parent: Number(row.ParentProcessId), bytes: Number(row.WorkingSetSize)}));
  }
  if (process.platform === "linux") {
    const rows = [];
    for (const pid of fs.readdirSync("/proc").filter(value => /^\d+$/.test(value))) {
      try {
        const status = fs.readFileSync(`/proc/${pid}/status`, "utf8");
        rows.push({pid: Number(pid), parent: Number(status.match(/^PPid:\s+(\d+)/m)?.[1] || 0), bytes: Number(status.match(/^VmRSS:\s+(\d+)/m)?.[1] || 0)*1024});
      } catch {}
    }
    return rows;
  }
  return [];
}
function treeBytes(rows, roots) {
  const pids = new Set(roots.filter(pid => Number.isSafeInteger(pid) && pid > 0));
  let previous;
  do {
    previous = pids.size;
    for (const row of rows) if (pids.has(row.parent)) pids.add(row.pid);
  } while (previous !== pids.size);
  return rows.reduce((total, row) => total + (pids.has(row.pid) ? row.bytes : 0), 0);
}
function directoryBytes(directory) {
  let bytes = 0;
  try {
    for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
      const name = path.join(directory, entry.name);
      if (entry.isDirectory()) bytes += directoryBytes(name);
      else if (entry.isFile()) { try { bytes += fs.statSync(name).size; } catch {} }
    }
  } catch {}
  return bytes;
}

exports.monitor = function monitor(root, processes) {
  const started = Date.now();
  const peak = { serverRSS: 0, browserAggregateRSS: 0, stagingBytes: 0, samples: 0 };
  let pending = Promise.resolve();
  let busy = false;
  const sample = async () => {
    if (busy) return;
    busy = true;
    try {
      peak.stagingBytes = Math.max(peak.stagingBytes, directoryBytes(path.join(root, ".okserver")));
      const { server, browser } = await processes();
      const rows = await memorySnapshot();
      const serverRSS = treeBytes(rows, server), browserRSS = treeBytes(rows, browser);
      peak.serverRSS = Math.max(peak.serverRSS, serverRSS);
      peak.browserAggregateRSS = Math.max(peak.browserAggregateRSS, browserRSS);
      peak.stagingBytes = Math.max(peak.stagingBytes, directoryBytes(path.join(root, ".okserver")));
      peak.samples++;
    } catch {
      // Restart windows may temporarily remove process handles or CDP sockets.
    } finally { busy = false; }
  };
  const timer = setInterval(() => { if (!busy) pending = sample(); }, 1000);
  return async () => {
    clearInterval(timer);
    await pending;
    return { ...peak, elapsedSeconds: (Date.now() - started) / 1000, samplingIntervalSeconds: 1 };
  };
};
