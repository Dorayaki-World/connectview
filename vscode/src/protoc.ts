import * as fs from "fs";
import * as path from "path";
import * as os from "os";
import { execFile } from "child_process";
import { Config } from "./config.js";

export type CompileResult =
  | { ok: true; html: string }
  | { ok: false; error: string };

/** Recursively find all .proto files under dir, returning paths relative to dir. */
export async function findProtoFiles(dir: string): Promise<string[]> {
  const results: string[] = [];

  const skip = new Set(["node_modules", "vendor", "third_party"]);

  async function walk(current: string): Promise<void> {
    const entries = await fs.promises.readdir(current, { withFileTypes: true });
    for (const entry of entries) {
      if (entry.isDirectory()) {
        // Skip hidden dirs (.buf, .git, …) and known vendored dirs
        if (entry.name.startsWith(".") || skip.has(entry.name)) continue;
        await walk(path.join(current, entry.name));
      } else if (entry.name.endsWith(".proto")) {
        results.push(path.relative(dir, path.join(current, entry.name)));
      }
    }
  }

  await walk(dir);
  return results;
}

/** Resolve a binary name to its absolute path using the login shell's PATH. */
async function resolveAbsolute(bin: string): Promise<string> {
  if (path.isAbsolute(bin)) return bin;

  const env = await getShellEnv();
  const pathDirs = (env.PATH || "").split(path.delimiter);
  for (const dir of pathDirs) {
    const candidate = path.join(dir, bin);
    try {
      await fs.promises.access(candidate, fs.constants.X_OK);
      return candidate;
    } catch {
      // not found here, continue
    }
  }
  return bin; // fall back to bare name
}

/** Extract import paths from a .proto file. */
async function extractImports(filePath: string): Promise<string[]> {
  try {
    const content = await fs.promises.readFile(filePath, "utf-8");
    const imports: string[] = [];
    const re = /^import\s+"([^"]+)";/gm;
    let m: RegExpExecArray | null;
    while ((m = re.exec(content)) !== null) {
      imports.push(m[1]);
    }
    return imports;
  } catch {
    return [];
  }
}

type ImportDetectionResult = {
  /** Additional -I paths for external dependencies (outside protoRoot). */
  includes: string[];
  /** If the real proto root is a subdirectory of the configured protoRoot, this is it. */
  effectiveRoot: string | null;
};

/**
 * Scan proto files for import statements and compute additional -I paths
 * needed to resolve imports that aren't found under the existing include paths.
 *
 * For each unresolved import (e.g. "user/v1/user.proto"), searches the workspace
 * for a matching file and back-computes the required -I directory.
 *
 * When the computed -I is a subdirectory of protoRoot, it means the real proto
 * root is that subdirectory. Instead of adding a conflicting -I, we return it
 * as `effectiveRoot` so the caller can switch to it (avoiding duplicate file errors).
 */
async function detectImportPaths(
  config: Config,
  protoFiles: string[],
  existingIncludes: string[]
): Promise<ImportDetectionResult> {
  const allIncludes = [config.protoRoot, ...existingIncludes, ...config.includePaths];

  // Collect all unresolved import paths.
  const unresolvedImports = new Set<string>();
  for (const relFile of protoFiles) {
    const absFile = path.join(config.protoRoot, relFile);
    const imports = await extractImports(absFile);
    for (const imp of imports) {
      // Skip well-known google imports — protoc includes them.
      if (imp.startsWith("google/protobuf/")) continue;

      // Check if the import resolves under any existing include path.
      const found = allIncludes.some((inc) => {
        try {
          fs.accessSync(path.join(inc, imp));
          return true;
        } catch {
          return false;
        }
      });
      if (!found) {
        unresolvedImports.add(imp);
      }
    }
  }

  if (unresolvedImports.size === 0) return { includes: [], effectiveRoot: null };

  // Build an index of all proto files in the workspace (relative to workspace root).
  const wsProtos = await findAllProtoFiles(config.workspaceRoot);

  const externalIncludes = new Set<string>();
  let effectiveRoot: string | null = null;

  for (const imp of unresolvedImports) {
    // Find a workspace file whose path ends with the import path.
    for (const wsFile of wsProtos) {
      if (wsFile === imp || wsFile.endsWith(path.sep + imp)) {
        // Back-compute: /workspace/src/proto/user/v1/user.proto - user/v1/user.proto
        //             = /workspace/src/proto/
        const absWsFile = path.join(config.workspaceRoot, wsFile);
        const includeDir = absWsFile.slice(0, absWsFile.length - imp.length).replace(/\/$/, "");

        if (!includeDir || allIncludes.includes(includeDir)) break;

        // If the computed include is a subdirectory of protoRoot, it's the real root.
        // Adding it as a separate -I would cause duplicate symbol errors.
        const protoRootWithSep = config.protoRoot + path.sep;
        if (includeDir.startsWith(protoRootWithSep) || includeDir === config.protoRoot) {
          effectiveRoot = includeDir;
        } else {
          externalIncludes.add(includeDir);
        }
        break;
      }
    }
  }

  return { includes: [...externalIncludes], effectiveRoot };
}

/** Find all .proto files under a directory (searches vendor/third_party but skips dotdirs and node_modules). */
async function findAllProtoFiles(dir: string): Promise<string[]> {
  const results: string[] = [];
  const skip = new Set(["node_modules"]);

  async function walk(current: string): Promise<void> {
    let entries;
    try {
      entries = await fs.promises.readdir(current, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      if (entry.isDirectory()) {
        if (entry.name.startsWith(".") || skip.has(entry.name)) continue;
        await walk(path.join(current, entry.name));
      } else if (entry.name.endsWith(".proto")) {
        results.push(path.relative(dir, path.join(current, entry.name)));
      }
    }
  }

  await walk(dir);
  return results;
}

/** Run protoc + protoc-gen-connectview to produce HTML. */
export async function compile(config: Config): Promise<CompileResult> {
  let protoFiles = await findProtoFiles(config.protoRoot);
  if (protoFiles.length === 0) {
    return { ok: false, error: "No .proto files found in " + config.protoRoot };
  }

  const protocPath = await resolveAbsolute(config.protocPath);
  const pluginPath = await resolveAbsolute(config.pluginPath);

  const tmpDir = await fs.promises.mkdtemp(
    path.join(os.tmpdir(), "connectview-")
  );

  try {
    let autoIncludes = await detectIncludePaths(config);
    const { includes: importIncludes, effectiveRoot } =
      await detectImportPaths(config, protoFiles, autoIncludes);

    // Determine the effective proto root. When the real proto root is a
    // subdirectory of the configured protoRoot, we must switch to it so
    // that file paths passed to protoc are consistent with -I paths.
    // Otherwise protoc sees the same file through two paths and reports
    // "already defined" errors.
    let protoRoot = config.protoRoot;
    let resolvedRoot = effectiveRoot;

    // Also check if any autoInclude is a subdirectory of protoRoot that
    // contains proto files — this means buf.yaml detection already found
    // the real root before import detection could fire.
    if (!resolvedRoot) {
      for (const inc of autoIncludes) {
        if (inc.startsWith(config.protoRoot + path.sep)) {
          const subFiles = await findProtoFiles(inc);
          if (subFiles.length > 0) {
            resolvedRoot = inc;
            break;
          }
        }
      }
    }

    if (resolvedRoot) {
      protoRoot = resolvedRoot;
      protoFiles = await findProtoFiles(protoRoot);
      // Remove the resolved root from autoIncludes — it's now the protoRoot.
      autoIncludes = autoIncludes.filter((p) => p !== resolvedRoot);
    }

    // Deduplicate include paths (order matters: protoRoot first).
    const allIncludePaths = [
      protoRoot,
      ...autoIncludes,
      ...importIncludes,
      ...config.includePaths,
    ];
    const seenIncludes = new Set<string>();
    const uniqueIncludes: string[] = [];
    for (const p of allIncludePaths) {
      if (!seenIncludes.has(p)) {
        seenIncludes.add(p);
        uniqueIncludes.push(p);
      }
    }

    const args: string[] = [
      `--plugin=protoc-gen-connectview=${pluginPath}`,
      `--connectview_out=${tmpDir}`,
      ...uniqueIncludes.map((p) => `-I${p}`),
      ...protoFiles,
    ];

    const stderr = await execProtoc(protocPath, args);

    const htmlPath = path.join(tmpDir, "index.html");
    if (!fs.existsSync(htmlPath)) {
      return {
        ok: false,
        error: stderr || "protoc produced no output (index.html not found)",
      };
    }

    const html = await fs.promises.readFile(htmlPath, "utf-8");
    return { ok: true, html };
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    return { ok: false, error: enhanceErrorMessage(msg, config) };
  } finally {
    await fs.promises.rm(tmpDir, { recursive: true, force: true });
  }
}

/**
 * Auto-detect include paths for protoc by looking for common dependency
 * directories (buf module cache, proto vendor dirs, buf.yaml locations)
 * at the workspace root and proto root.
 */
async function detectIncludePaths(config: Config): Promise<string[]> {
  const candidates = new Set<string>();

  // Check both workspace root and proto root for dependency dirs.
  const roots = [config.workspaceRoot, config.protoRoot];
  for (const root of roots) {
    if (!root) continue;
    // buf module cache (.buf/)
    const bufDir = path.join(root, ".buf");
    if (await isDir(bufDir)) candidates.add(bufDir);
    // proto vendor dirs
    const protoVendor = path.join(root, "proto_vendor");
    if (await isDir(protoVendor)) candidates.add(protoVendor);
    // buf.yaml indicates a buf module root — resolve its deps from cache
    await addBufModuleIncludes(root, candidates);
  }

  // Also search one level of subdirectories for buf.yaml (e.g. proto/buf.yaml)
  if (config.workspaceRoot) {
    try {
      const entries = await fs.promises.readdir(config.workspaceRoot, { withFileTypes: true });
      for (const entry of entries) {
        if (!entry.isDirectory() || entry.name.startsWith(".")) continue;
        await addBufModuleIncludes(path.join(config.workspaceRoot, entry.name), candidates);
      }
    } catch {
      // ignore read errors
    }
  }

  return [...candidates];
}

/**
 * If dir contains buf.yaml, add it as an include path and resolve
 * its dependencies from the buf module cache (~/.cache/buf/).
 */
async function addBufModuleIncludes(dir: string, candidates: Set<string>): Promise<void> {
  let found = false;
  for (const name of ["buf.yaml", "buf.yml"]) {
    if (await isFile(path.join(dir, name))) {
      found = true;
      break;
    }
  }
  if (!found) return;

  candidates.add(dir);

  // Read buf.lock to find dependency module names.
  const lockPath = path.join(dir, "buf.lock");
  if (!await isFile(lockPath)) return;

  let lockContent: string;
  try {
    lockContent = await fs.promises.readFile(lockPath, "utf-8");
  } catch {
    return;
  }

  const depNames: string[] = [];
  const nameRe = /^\s+-\s+name:\s+(.+)$/gm;
  let m: RegExpExecArray | null;
  while ((m = nameRe.exec(lockContent)) !== null) {
    depNames.push(m[1].trim());
  }
  if (depNames.length === 0) return;

  // Resolve each dep from the buf module cache.
  const bufCacheBase = path.join(os.homedir(), ".cache", "buf", "v3", "modules");
  if (!await isDir(bufCacheBase)) return;

  for (const dep of depNames) {
    // Cache structure: ~/.cache/buf/v3/modules/<digest_type>/<module_name>/<hash>/files/
    // Try common digest types.
    for (const digestType of ["b5", "b4", "b3"]) {
      const moduleDir = path.join(bufCacheBase, digestType, dep);
      if (!await isDir(moduleDir)) continue;

      // Pick the most recently modified version.
      let entries;
      try {
        entries = await fs.promises.readdir(moduleDir, { withFileTypes: true });
      } catch {
        continue;
      }

      let latestDir = "";
      let latestMtime = 0;
      for (const entry of entries) {
        if (!entry.isDirectory()) continue;
        try {
          const stat = await fs.promises.stat(path.join(moduleDir, entry.name));
          if (stat.mtimeMs > latestMtime) {
            latestMtime = stat.mtimeMs;
            latestDir = entry.name;
          }
        } catch {
          continue;
        }
      }

      if (latestDir) {
        const filesDir = path.join(moduleDir, latestDir, "files");
        if (await isDir(filesDir)) {
          candidates.add(filesDir);
        }
      }
      break; // Found the module in this digest type, stop trying others.
    }
  }
}

/**
 * Enhance a protoc error message with hints when import resolution fails.
 */
function enhanceErrorMessage(message: string, config: Config): string {
  const importNotFound = /Import "([^"]+)" was not found/;
  const fileNotFound = /([^\s:]+\.proto):\s*File not found/;

  if (importNotFound.test(message) || fileNotFound.test(message)) {
    const hint = [
      "",
      "Hint: proto imports could not be resolved. Try one of:",
      `  1. Set "connectview.protoRoot" to the directory matching your import structure`,
      `  2. Add missing include paths to "connectview.includePaths"`,
      `  Current protoRoot: ${config.protoRoot}`,
    ].join("\n");
    return message + hint;
  }
  return message;
}

async function isDir(p: string): Promise<boolean> {
  try {
    return (await fs.promises.stat(p)).isDirectory();
  } catch {
    return false;
  }
}

async function isFile(p: string): Promise<boolean> {
  try {
    return (await fs.promises.stat(p)).isFile();
  } catch {
    return false;
  }
}

let shellEnv: Record<string, string> | undefined;

/** Get environment with PATH from user's login shell (macOS GUI apps lack it). */
async function getShellEnv(): Promise<Record<string, string>> {
  if (shellEnv) return shellEnv;

  const env = { ...process.env } as Record<string, string>;

  if (process.platform === "darwin") {
    try {
      const shell = process.env.SHELL || "/bin/zsh";
      const result = await new Promise<string>((resolve, reject) => {
        execFile(shell, ["-ilc", "env"], { timeout: 5000 }, (err, stdout) => {
          if (err) reject(err);
          else resolve(stdout);
        });
      });
      for (const line of result.split("\n")) {
        const eq = line.indexOf("=");
        if (eq > 0) {
          env[line.slice(0, eq)] = line.slice(eq + 1);
        }
      }
    } catch {
      // Fall back to process.env
    }
  }

  shellEnv = env;
  return env;
}

function execProtoc(protocPath: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    getShellEnv().then((env) => {
      execFile(protocPath, args, { maxBuffer: 10 * 1024 * 1024, env }, (err, _stdout, stderr) => {
        if (err) {
          reject(new Error(stderr || err.message));
        } else {
          resolve(stderr);
        }
      });
    });
  });
}
