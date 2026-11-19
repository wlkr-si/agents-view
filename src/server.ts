import { spawn } from "node:child_process";
import {
  readdirSync,
  readFileSync,
  realpathSync,
  statSync,
  watch,
} from "node:fs";
import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";
import { basename, dirname, extname, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import {
  findSection,
  markdown,
  overrides,
  parse,
  tokens,
  update,
} from "./document.js";
import { discover } from "./sources.js";

const appPolicy =
  "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'";
const mermaidVersion = "11.12.2";
const assets = {
  "/": ["index.html", "text/html"],
  "/app.js": ["app.js", "text/javascript"],
  "/style.css": ["style.css", "text/css"],
  "/diagram": ["diagram.html", "text/html"],
  "/diagram.js": ["diagram.js", "text/javascript"],
} as const;

async function requestData(request: IncomingMessage) {
  if (request.headers["content-type"] !== "application/json")
    throw new Error("Expected JSON.");
  let body = "";
  request.setEncoding("utf8");
  for await (const part of request) {
    body += part;
    if (body.length > 256_000) throw new Error("Edit exceeds 256 KB.");
  }
  return JSON.parse(body) as Record<string, unknown>;
}

function openBrowser(url: string) {
  const opener =
    process.platform === "darwin"
      ? "open"
      : process.platform === "win32"
        ? "explorer.exe"
        : "xdg-open";
  spawn(opener, [url], { stdio: "ignore" }).on("error", () =>
    console.error(`Open ${url} in your browser.`),
  );
}

export async function serve(cwd: string, file: string, open: boolean) {
  const root = realpathSync(cwd);
  const streams = new Set<ServerResponse>();
  let origin = "";
  let timer: NodeJS.Timeout | undefined;
  let job: ReturnType<typeof spawn> | undefined;
  let jobStatus = "";

  function broadcast(event: string, data: string) {
    for (const stream of streams)
      stream.write(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
  }

  function runDiagram(section: string, change: string) {
    if (job) throw new Error("A diagram edit is already running.");
    const argv: unknown = JSON.parse(
      process.env.AGENTS_VIEW_AGENT_COMMAND ??
        '["codex","exec","-s","workspace-write","-"]',
    );
    if (
      !Array.isArray(argv) ||
      !argv.length ||
      !argv.every((arg) => typeof arg === "string" && arg.length)
    ) {
      throw new Error(
        "AGENTS_VIEW_AGENT_COMMAND must be a JSON array of command arguments.",
      );
    }
    const cli = fileURLToPath(new URL("./cli.js", import.meta.url));
    const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
    const prompt = [
      `Update only the ${JSON.stringify(section)} diagram section in AGENTS.md in the current working directory.`,
      "Read that section, keep the Mermaid fence and any accompanying agent-readable text, and preserve all other sections.",
      `Feed the complete replacement body on stdin to agents-view write ${quote(section)}. If the bin is unavailable, use ${quote(process.execPath)} ${quote(cli)} write ${quote(section)}.`,
      `Requested change: ${change}`,
    ].join("\n");
    job = spawn(argv[0], argv.slice(1), {
      cwd,
      stdio: ["pipe", "ignore", "ignore"],
      detached: process.platform !== "win32",
    });
    jobStatus = "Diagram edit running…";
    broadcast("job", jobStatus);
    job.stdin?.on("error", () => {});
    job.stdin?.end(prompt);
    job.once("error", (error) => {
      jobStatus = error.message;
      job = undefined;
      broadcast("job", jobStatus);
    });
    job.once("exit", (code) => {
      jobStatus =
        code === 0
          ? "Diagram edit finished."
          : `Diagram edit exited with status ${code ?? "signal"}.`;
      job = undefined;
      broadcast("job", jobStatus);
    });
  }

  const server = createServer(async (request, response) => {
    response.setHeader("Cache-Control", "no-store");
    response.setHeader("X-Content-Type-Options", "nosniff");
    response.setHeader("Referrer-Policy", "no-referrer");
    if (request.headers.host !== new URL(origin).host) {
      response.writeHead(403).end();
      return;
    }
    const url = new URL(request.url ?? "/", origin);
    const path = url.pathname;
    const json = (status: number, value: unknown) =>
      response
        .writeHead(status, { "Content-Type": "application/json" })
        .end(JSON.stringify(value));
    try {
      if (request.method === "GET" && Object.hasOwn(assets, path)) {
        const [name, contentType] = assets[path as keyof typeof assets];
        const policy =
          path === "/diagram"
            ? `default-src 'none'; script-src ${origin} https://cdn.jsdelivr.net; style-src 'unsafe-inline'; img-src data:; frame-ancestors ${origin}`
            : appPolicy;
        response
          .writeHead(200, {
            "Content-Type": `${contentType}; charset=utf-8`,
            "Content-Security-Policy": policy,
          })
          .end(readFileSync(new URL(`../web/${name}`, import.meta.url)));
        return;
      }
      if (request.method === "GET" && path.startsWith("/page/")) {
        const relative = decodeURIComponent(path.slice("/page/".length));
        if (relative.split(/[\\/]/).some((part) => part.startsWith(".")))
          throw new Error("Choose a visible project page.");
        const target = realpathSync(resolve(root, relative));
        if (target !== root && !target.startsWith(root + sep))
          throw new Error("Choose a page inside the project.");
        const title = markdown.utils.escapeHtml(basename(target));
        let content: string;
        if (statSync(target).isDirectory()) {
          if (!path.endsWith("/")) {
            response.writeHead(308, { Location: `${path}/` }).end();
            return;
          }
          const entries = readdirSync(target, { withFileTypes: true }).filter(
            (entry) =>
              !entry.name.startsWith(".") &&
              (entry.isDirectory() ||
                (entry.isFile() &&
                  extname(entry.name).toLowerCase() === ".md")),
          );
          content = `<h1>${title}</h1><ul>${entries.map((entry) => `<li><a href="${encodeURIComponent(entry.name)}${entry.isDirectory() ? "/" : ""}">${markdown.utils.escapeHtml(entry.name)}</a></li>`).join("")}</ul>`;
        } else {
          if (extname(target).toLowerCase() !== ".md")
            throw new Error("Choose a Markdown page.");
          content = markdown.render(readFileSync(target, "utf8"));
        }
        response
          .writeHead(200, {
            "Content-Type": "text/html; charset=utf-8",
            "Content-Security-Policy": appPolicy,
          })
          .end(
            `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>${title}</title><link rel="stylesheet" href="/style.css"><main class="shell content"><p><a href="/">← Project</a></p>${content}</main></html>`,
          );
        return;
      }
      if (request.method === "GET" && path === "/events") {
        response.writeHead(200, {
          "Content-Type": "text/event-stream",
          Connection: "keep-alive",
        });
        response.write(`event: job\ndata: ${JSON.stringify(jobStatus)}\n\n`);
        streams.add(response);
        response.on("close", () => streams.delete(response));
        return;
      }
      if (request.method === "GET" && path === "/project") {
        const document = parse(readFileSync(file, "utf8"));
        const heading = /^#\s+(.+)$/m.exec(document.sections[0].body)?.[1];
        json(200, {
          project: heading || basename(cwd),
          directory: cwd,
          tokens: document.tokens,
          mermaidVersion,
          sections: document.sections.map(({ name, type, body, tokens }) => ({
            name: name ?? "Intro",
            type,
            body,
            tokens,
            html: markdown.render(body),
          })),
        });
        return;
      }
      if (request.method === "GET" && path === "/sources") {
        const entries = overrides(parse(readFileSync(file, "utf8")));
        const groups = discover(cwd);
        for (const group of groups)
          for (const source of group.files) watchFile(source.path);
        const globals = groups.flatMap((group) =>
          group.files.filter((file) => file.origin === "Global"),
        );
        json(200, {
          groups: groups.map((group) => ({
            agent: group.agent,
            files: group.files.map((file) => ({
              ...file,
              tokens: tokens(file.text),
            })),
          })),
          overrides: entries.map((entry) => ({
            ...entry,
            stale:
              !entry.quote ||
              !globals.some((file) => file.text.includes(entry.quote)),
          })),
        });
        return;
      }
      if (
        request.method === "POST" &&
        ["/section", "/override", "/diagram"].includes(path)
      ) {
        if (request.headers.origin !== origin) {
          response.writeHead(403).end();
          return;
        }
        const data = await requestData(request);
        if (path === "/section") {
          if (
            typeof data.name !== "string" ||
            typeof data.body !== "string" ||
            typeof data.expected !== "string"
          )
            throw new Error("Invalid section edit.");
          const current = findSection(
            parse(readFileSync(file, "utf8")),
            data.name,
          );
          if (!current || current.type === "diagram")
            throw new Error("Choose an editable section.");
          update(file, data.name, data.body, data.expected);
        } else if (path === "/override") {
          const { quote, replacement } = data;
          if (
            typeof quote !== "string" ||
            typeof replacement !== "string" ||
            !quote.trim() ||
            !replacement.trim() ||
            /[\r\n]/.test(quote + replacement)
          )
            throw new Error("Use one line for the quote and replacement.");
          if (
            !discover(cwd).some((group) =>
              group.files.some(
                (file) =>
                  file.origin === "Global" &&
                  file.text.split(/\r?\n/).includes(quote),
              ),
            )
          )
            throw new Error("The global line changed on disk.");
          update(
            file,
            "Overrides",
            `- Global: ${JSON.stringify(quote)} → here: ${replacement.trim()}`,
            undefined,
            true,
          );
        } else {
          if (
            typeof data.name !== "string" ||
            typeof data.change !== "string" ||
            !data.change.trim()
          )
            throw new Error("Describe the diagram change.");
          const section = findSection(
            parse(readFileSync(file, "utf8")),
            data.name,
          );
          if (section?.type !== "diagram")
            throw new Error("Choose an existing diagram.");
          runDiagram(data.name, data.change.trim());
        }
        json(200, { ok: true });
        return;
      }
      response.writeHead(404).end();
    } catch (error) {
      json(400, {
        error: error instanceof Error ? error.message : String(error),
      });
    }
  });

  const watchers = new Map<string, ReturnType<typeof watch>>();
  const watchedFiles = new Set<string>();
  function watchFile(path: string) {
    watchPath(path);
    const target = realpathSync(path);
    if (target !== path) watchPath(target);
  }
  function watchPath(path: string) {
    watchedFiles.add(path);
    const directory = dirname(path);
    if (watchers.has(directory)) return;
    const watcher = watch(directory, (_event, name) => {
      if (name && !watchedFiles.has(resolve(directory, name))) return;
      clearTimeout(timer);
      timer = setTimeout(() => broadcast("change", ""), 125);
    });
    watcher.on("error", () => {
      watcher.close();
      watchers.delete(directory);
    });
    watchers.set(directory, watcher);
  }
  watchFile(file);
  server.on("error", (error) => {
    console.error(error.message);
    process.exitCode = 1;
    stop();
  });
  function stop() {
    for (const watcher of watchers.values()) watcher.close();
    clearTimeout(timer);
    for (const stream of streams) stream.end();
    if (job?.pid && process.platform !== "win32") {
      try {
        process.kill(-job.pid, "SIGTERM");
      } catch {
        /* Process already exited. */
      }
    } else job?.kill();
    server.close();
  }
  process.once("SIGINT", stop);
  process.once("SIGTERM", stop);
  server.listen(0, "127.0.0.1", () => {
    const address = server.address();
    if (!address || typeof address === "string") return;
    origin = `http://127.0.0.1:${address.port}`;
    console.log(origin);
    if (open) openBrowser(origin);
  });
}
