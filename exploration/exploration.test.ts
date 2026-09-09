import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type Server } from "node:http";
import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { loadConfig, runExploration } from "./index.js";

test("explores pages, API, bundles, files and dry-run writes", async () => {
  let writes = 0;
  const server = createServer((request, response) => {
    if (request.method === "POST") writes++;
    if (request.url === "/") {
      response.setHeader("content-type", "text/html");
      response.end(`<!doctype html><title>Home</title>
        <a href="/details?token=fixture-secret">Details</a>
        <a href="/report.csv" download>Export</a>
        <form action="/save" method="post" enctype="multipart/form-data" onsubmit="fetch('/api/save', {method: 'POST'}); return false">
          <input name="name" required><input type="file" name="attachment"><button>Save</button>
        </form>
        <form action="/search" method="get"><input name="query"><button>Search</button></form>
        <iframe src="/frame" title="embedded"></iframe>
        <script src="/app.js"></script>
        <script>fetch('/api/items'); fetch('/api/auto-save', {method: 'POST', body: 'name=demo'});</script>`);
      return;
    }
    if (request.url === "/details" || request.url === "/frame") {
      response.setHeader("content-type", "text/html");
      response.end("<!doctype html><title>Details</title><p>ok</p>");
      return;
    }
    if (request.url === "/app.js") {
      response.setHeader("content-type", "application/javascript");
      response.end("fetch('/api/items'); router.push('/details'); fetch('/api/save', {method:'POST'});");
      return;
    }
    if (request.url === "/api/items") {
      response.setHeader("content-type", "application/json");
      response.end(JSON.stringify({ items: [{ id: 1 }] }));
      return;
    }
    if (request.url === "/report.csv") {
      response.setHeader("content-type", "text/csv");
      response.end("id\n1\n");
      return;
    }
    response.statusCode = 404;
    response.end("not found");
  });
  await listen(server);
  try {
    const address = server.address();
    assert(address && typeof address === "object");
    const result = await runExploration({
      targets: [{
        name: "fixture",
        baseUrl: `http://127.0.0.1:${address.port}`,
        maxDepth: 1,
        maxPages: 10,
        probeActions: true,
      }],
    });
    const target = result.targets[0];
    assert.equal(writes, 0, "dry-run requests must not reach the server");
    assert(target.pages.length >= 3);
    assert(target.requests.some((item) => item.url.endsWith("/api/items") && item.response?.status === 200));
    assert(target.requests.some((item) => item.url.endsWith("/api/auto-save") && item.dry_run));
    assert(target.scripts.some((item) => item.endpoints.includes("/api/items")));
    assert(target.capabilities.some((item) => item.capability === "file.upload"));
    assert(target.capabilities.some((item) => item.capability === "file.download"));
    assert(target.capabilities.some((item) => item.capability === "embedded.frame"));
    assert(target.capabilities.some((item) => item.capability.startsWith("form.") && item.read_write === "write"));
    assert(target.pages.some((item) => item.dry_run_probes.length === 1 && item.dry_run_probes[0].fields.includes("name")));
    assert(target.audit.status_counts.unexplored > 0);
    assert(!JSON.stringify(result).includes("fixture-secret"), "JSON artifacts must redact URL secrets");
  } finally {
    await close(server);
  }
});

test("loads relative storageState and output paths from JSON config", async () => {
  const directory = await mkdtemp(join(tmpdir(), "csust-exploration-"));
  const configPath = join(directory, "config.json");
  await writeFile(configPath, JSON.stringify({
    output: "artifacts/result.json",
    targets: [{ baseUrl: "https://example.org", storageState: "auth/state.json" }],
  }));
  const config = await loadConfig(configPath);
  assert.equal(config.output, join(directory, "artifacts/result.json"));
  assert.equal(config.targets[0].storageState, join(directory, "auth/state.json"));
  assert.equal(config.targets[0].authProfile, "authenticated");
  assert.equal((await readFile(configPath, "utf8")).includes("example.org"), true);
});

function listen(server: Server): Promise<void> {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => resolve());
  });
}

function close(server: Server): Promise<void> {
  return new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
}
