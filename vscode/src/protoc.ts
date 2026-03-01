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

/** Run protoc + protoc-gen-connectview to produce HTML. */
export async function compile(config: Config): Promise<CompileResult> {
  const protoFiles = await findProtoFiles(config.protoRoot);
  if (protoFiles.length === 0) {
    return { ok: false, error: "No .proto files found in " + config.protoRoot };
  }

  const protocPath = await resolveAbsolute(config.protocPath);
  const pluginPath = await resolveAbsolute(config.pluginPath);

  const tmpDir = await fs.promises.mkdtemp(
    path.join(os.tmpdir(), "connectview-")
  );

  try {
    // Auto-detect buf module cache (.buf/) as an include path for imports.
    const autoIncludes: string[] = [];
    const bufDir = path.join(config.protoRoot, ".buf");
    try {
      const st = await fs.promises.stat(bufDir);
      if (st.isDirectory()) autoIncludes.push(bufDir);
    } catch {
      // no .buf dir, skip
    }

    const args: string[] = [
      `--plugin=protoc-gen-connectview=${pluginPath}`,
      `--connectview_out=${tmpDir}`,
      `-I${config.protoRoot}`,
      ...autoIncludes.map((p) => `-I${p}`),
      ...config.includePaths.map((p) => `-I${p}`),
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
  } finally {
    await fs.promises.rm(tmpDir, { recursive: true, force: true });
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
