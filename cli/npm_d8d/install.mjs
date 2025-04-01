import { chmodSync, cpSync, createWriteStream, existsSync, linkSync, readFileSync, statSync, unlinkSync } from "node:fs";
import { createRequire } from "node:module";
import { Writable } from "node:stream";

// 1. Attempt to resolve "@esm.d8d.fun/cli-{os}-{arch}", if not found, then try to download the binary from GitHub.
// 2. On macOS/Linux, link "bin/esm.d8d.fun" to "@esm.d8d.fun/cli-{os}-{arch}/bin/esm.d8d.fun" if exists.
install();

async function install() {
  const binPath = toPackagePath("bin/esm.d8d.fun" + getBinExtension());
  try {
    const nativeBinPath = resolveBinaryPath();
    if (process.platform !== "win32") {
      unlinkSync(binPath);
      linkSync(nativeBinPath, binPath);
      chmodAddX(binPath);
    }
  } catch {
    try {
      console.log("[esm.d8d.fun] Trying to download esm.d8d.fun binary from GitHub...");
      const readable = await downloadBinaryFromGitHub();
      await readable.pipeTo(Writable.toWeb(createWriteStream(binPath)));
      chmodAddX(binPath);
    } catch (err) {
      console.error("[esm.d8d.fun] Failed to install esm.d8d.fun binary:", err);
      throw err;
    }
  }
}

function resolveBinaryPath() {
  const cliBinPackage = `@esm.d8d.fun/cli-${getOS()}-${getArch()}`;
  const binPath = createRequire(import.meta.url).resolve(cliBinPackage + "/bin/esm.d8d.fun" + getBinExtension());
  if (!existsSync(binPath)) {
    throw new Error(`Could not find the binary of '${cliBinPackage}'`);
  }
  return binPath;
}

async function downloadBinaryFromGitHub() {
  const pkgInfo = JSON.parse(readFileSync(toPackagePath("package.json"), "utf8"));
  const [_, minor, patch] = pkgInfo.version.split(".");
  const tag = "v" + minor + (Number(patch) > 0 ? "_" + patch : "");
  const url = `https://github.com/zyh320888/esm.sh/releases/download/${tag}/esm.d8d.fun-cli-${getOS()}-${getArch()}.gz`;
  const res = await fetch(url);
  if (!res.ok) {
    res.body?.cancel();
    throw new Error(`Download ${url}: <${res.statusText}>`);
  }
  return res.body.pipeThrough(new DecompressionStream("gzip"));
}

function getOS() {
  switch (process.platform) {
    case "darwin":
      return "darwin";
    case "linux":
      return "linux";
    case "win32":
      return "windows";
    default:
      throw new Error(`Unsupported platform: ${process.platform}`);
  }
}

function getArch() {
  switch (process.arch) {
    case "arm64":
      return "arm64";
    case "x64":
      return "amd64";
    default:
      throw new Error(`Unsupported architecture: ${process.arch}`);
  }
}

function getBinExtension() {
  return process.platform === "win32" ? ".exe" : "";
}

function toPackagePath(filename) {
  return new URL(filename, import.meta.url).pathname;
}

function chmodAddX(path) {
  chmodSync(path, statSync(path).mode | 0o111);
}
