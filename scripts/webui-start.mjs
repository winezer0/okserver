import { spawn } from "node:child_process";
import { mkdir, access } from "node:fs/promises";
import { createServer } from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";

const project = fileURLToPath(new URL("../", import.meta.url));
const frontend = path.join(project, "web");
const children = new Set();
const windows = process.platform === "win32";
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
let stopping = false;

function start(command, args, options = {}) {
  const child = spawn(command, args, { cwd: project, stdio: "inherit", windowsHide: true, ...options });
  children.add(child);
  child.once("exit", () => children.delete(child));
  child.completion = new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code, signal) => code === 0 ? resolve() : reject(new Error(`${command} exited: ${signal || code}`)));
  });
  // Startup and shutdown inspect this promise separately.
  child.completion.catch(() => {});
  return child;
}

async function run(command, args, options) {
  await start(command, args, options).completion;
}

async function npm(args) {
  // Only fixed npm commands are passed through cmd; user paths and ports never enter shell text.
  if (windows) await run(process.env.ComSpec || "cmd.exe", ["/d", "/s", "/c", `npm ${args.join(" ")}`], { cwd: frontend });
  else await run("npm", args, { cwd: frontend });
}

async function stop(code) {
  if (stopping) return;
  stopping = true;
  for (const child of children) {
    if (windows && child.pid) {
      await new Promise(resolve => {
        const killer = spawn("taskkill.exe", ["/PID", String(child.pid), "/T", "/F"], { stdio:"ignore", windowsHide:true });
        killer.once("error", resolve); killer.once("exit", resolve);
      });
    } else child.kill();
  }
  await Promise.race([Promise.allSettled([...children].map(child => child.completion)), sleep(3000)]);
  for (const child of children) child.kill("SIGKILL");
  if (process.connected) process.disconnect();
  process.exitCode = code;
}

function port(value, fallback) {
  if (value === undefined) return fallback;
  if (!/^\d+$/.test(value) || Number(value) < 1 || Number(value) > 65535) throw new Error(`Invalid port: ${value}`);
  return Number(value);
}

async function available(port) {
  await new Promise((resolve, reject) => {
    const probe = createServer();
    probe.once("error", () => reject(new Error(`Port ${port} is already in use`)));
    probe.listen(port, "127.0.0.1", () => probe.close(resolve));
  });
}

async function ready(url, child) {
  for (let attempt = 0; attempt < 150; attempt++) {
    if (child.exitCode !== null) throw new Error(`Server stopped before ready: ${url}`);
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(1000) });
      await response.body?.cancel();
      if (response.status < 500) return;
    } catch { /* Server may still be starting. */ }
    await sleep(200);
  }
  throw new Error(`Server readiness timed out: ${url}`);
}

function openBrowser(url) {
  const command = windows ? "rundll32.exe" : process.platform === "darwin" ? "open" : "xdg-open";
  const args = windows ? ["url.dll,FileProtocolHandler", url] : [url];
  const browser = spawn(command, args, { stdio: "ignore", windowsHide: true });
  browser.on("error", error => console.warn(`[WARN] Open ${url} manually: ${error.message}`));
  browser.unref();
}

async function main() {
  const [mode, ...args] = process.argv.slice(2);
  if (!["build", "dev"].includes(mode)) throw new Error("Expected build or dev mode");
  const flags = args.filter(arg => arg.startsWith("--"));
  if (flags.some(flag => !["--no-open", "--build-only"].includes(flag))) throw new Error("Unknown option");
  if (mode === "dev" && flags.includes("--build-only")) throw new Error("--build-only requires build mode");
  const positional = args.filter(arg => !arg.startsWith("--"));
  if (positional.length > (mode === "dev" ? 4 : 3)) throw new Error("Too many arguments");
  const frontendPort = mode === "dev" ? port(positional[0], 28081) : null;
  const apiPort = port(positional[mode === "dev" ? 1 : 0], 28080);
  if (frontendPort === apiPort) throw new Error("Frontend and API ports must differ");
  const config = path.resolve(project, positional[mode === "dev" ? 2 : 1] || "server.yaml");
  const root = path.resolve(project, positional[mode === "dev" ? 3 : 2] || "uploads");
  const buildOnly = flags.includes("--build-only");
  await access(config);
  if (!buildOnly) {
    await available(apiPort);
    if (frontendPort) await available(frontendPort);
  }
  const [nodeMajor, nodeMinor, nodePatch] = process.versions.node.split(".").map(Number);
  const supportedNode = nodeMajor >= 26 || (nodeMajor === 24 && nodeMinor >= 15) || (nodeMajor === 22 && (nodeMinor > 22 || (nodeMinor === 22 && nodePatch >= 2)));
  if (!supportedNode) throw new Error("Use Node.js 22.22.2+, 24.15+ or 26+ (dependency engine requirements)");
  await run("go", ["version"]);
  await npm(["ci"]);
  await npm(["test"]);
  await npm(["run", "build"]);
  const binaryDirectory = path.join(project, "bin");
  await mkdir(binaryDirectory, { recursive: true });
  const binary = path.join(binaryDirectory, `okserver-${mode}${windows ? ".exe" : ""}`);
  console.log(`[INFO] Building ${binary}`);
  const buildArgs = ["build", "-trimpath"];
  if (mode === "build") buildArgs.push("-ldflags", "-s -w");
  await run("go", [...buildArgs, "-o", binary, "./cmd/okserver"], { env: { ...process.env, GOWORK: "off" } });
  if (buildOnly) return;
  await mkdir(root, { recursive: true });
  const backend = start(binary, ["-c", config, "-s", `127.0.0.1:${apiPort}`, "-p", root]);
  const target = process.env.OKSERVER_WEBUI_API_TARGET || `http://127.0.0.1:${apiPort}`;
  await Promise.race([ready(`${target}/healthz`, backend), backend.completion.then(() => { throw new Error("Backend stopped"); })]);
  let url = target;
  let vite;
  if (mode === "dev") {
    vite = start(process.execPath, [path.join(frontend, "node_modules/vite/bin/vite.js"), "--host", "127.0.0.1", "--port", String(frontendPort), "--strictPort"], {
      cwd: frontend, env: { ...process.env, OKSERVER_WEBUI_API_TARGET: target },
    });
    url = `http://127.0.0.1:${frontendPort}`;
    await Promise.race([ready(url, vite), vite.completion.then(() => { throw new Error("Vite stopped"); })]);
  }
  console.log(`[READY] ${url}/\n[INFO] Storage: ${root}\n[INFO] Press Ctrl+C to stop${vite ? " both servers" : " the server"}.`);
  if (!flags.includes("--no-open")) openBrowser(url);
  await Promise.race([backend.completion, ...(vite ? [vite.completion] : [])]);
}

process.once("SIGINT", () => void stop(0));
process.once("SIGTERM", () => void stop(0));
// Test harnesses may request a graceful stop without terminating a Windows process tree.
if (process.send) process.on("message", message => { if (message === "stop") void stop(0); });
main().then(() => stop(0), error => {
  if (!stopping) console.error(`[ERROR] ${error.message}`);
  return stop(stopping ? 0 : 1);
});
