import {
  chromium,
  type BrowserContext,
  type Page,
  type Request,
  type Response,
} from "@playwright/test";
import { readFile, rename, mkdir, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";

export type ImplementationStatus =
  | "implemented"
  | "partially_implemented"
  | "unsupported"
  | "unexplored"
  | "inaccessible";

export type MappingRule = {
  pattern: string;
  command: string;
  status?: ImplementationStatus;
};

export type TargetConfig = {
  name?: string;
  baseUrl: string;
  entryPaths?: string[];
  allowedHosts?: string[];
  storageState?: string;
  authProfile?: string;
  domain?: string;
  headers?: Record<string, string>;
  headless?: boolean;
  maxDepth?: number;
  maxPages?: number;
  maxRequests?: number;
  timeoutMs?: number;
  waitMs?: number;
  probeActions?: boolean;
  mappings?: MappingRule[];
};

export type ExplorationConfig = {
  targets: TargetConfig[];
  output?: string;
};

type NormalizedTarget = Required<
  Omit<TargetConfig, "storageState" | "domain" | "headers" | "mappings">
> & {
  storageState?: string;
  domain?: string;
  headers: Record<string, string>;
  mappings: MappingRule[];
};

type PageLink = {
  ref: string;
  href: string;
  text: string;
  download: string | null;
  intent: string;
  has_onclick: boolean;
  same_origin: boolean;
};

type FormControl = {
  name: string;
  type: string;
  required: boolean;
  disabled: boolean;
  file: boolean;
  options: string[];
};

type PageForm = {
  ref: string;
  action: string;
  method: string;
  enctype: string;
  intent: string;
  write: boolean;
  controls: FormControl[];
};

type PageAction = {
  ref: string;
  label: string;
  type: string;
  intent: string;
  write: boolean;
  form_ref: string | null;
};

type PageScript = {
  ref: string;
  src: string | null;
  type: string;
  inline: boolean;
  source?: string;
};

type PageFrame = {
  ref: string;
  src: string;
  name: string;
  title: string;
};

type PageSnapshot = {
  title: string;
  kind: string;
  links: PageLink[];
  forms: PageForm[];
  actions: PageAction[];
  scripts: PageScript[];
  iframes: PageFrame[];
  signals: string[];
};

type DryRunProbe = {
  ref: string;
  action: string;
  method: string;
  fields: string[];
  valid: boolean;
  dispatched: boolean;
};

type DownloadEvidence = {
  url: string;
  suggested_filename: string;
  failure: string | null;
};

type BodySummary = {
  kind: string;
  bytes: number;
  keys: string[];
};

export type RequestEvidence = {
  id: string;
  page_id: string;
  url: string;
  method: string;
  resource_type: string;
  request_body?: BodySummary;
  response?: {
    status: number;
    content_type: string;
    bytes?: number;
    shape?: string[];
  };
  failed?: string;
  dry_run: boolean;
};

export type ScriptEvidence = {
  id: string;
  page_id: string;
  url: string;
  status: number | null;
  bytes: number;
  endpoints: string[];
  routes: string[];
  actions: string[];
  skipped?: string;
  error?: string;
};

type PageRecord = {
  id: string;
  requested_url: string;
  url: string;
  depth: number;
  discovered_from: string | null;
  access: "accessible" | "inaccessible";
  status: number | null;
  redirects: string[];
  title: string;
  kind: string;
  authentication: {
    profile: string;
    storage_state: boolean;
    signals: string[];
  };
  snapshot: PageSnapshot | null;
  dry_run_probes: DryRunProbe[];
  downloads: DownloadEvidence[];
  request_ids: string[];
  script_ids: string[];
  error?: string;
};

type Evidence = {
  kind: "page" | "request" | "script" | "element" | "dry_run" | "download";
  ref: string;
  detail: string;
};

type CliMapping = {
  status: "mapped" | "unmapped";
  command?: string;
  suggested_command?: string;
  matched_pattern?: string;
  reason?: string;
};

export type Capability = {
  id: string;
  capability: string;
  kind: string;
  entry: string;
  page: string;
  underlying_protocol: Record<string, unknown>;
  authentication: PageRecord["authentication"];
  inputs: Record<string, unknown>;
  outputs: Record<string, unknown>;
  read_write: "read" | "write";
  verification_method: string[];
  confidence: "high" | "medium" | "low";
  evidence: Evidence[];
  cli_mapping: CliMapping;
  implementation_status: ImplementationStatus;
};

type TargetResult = {
  target: {
    name: string;
    base_url: string;
    auth_profile: string;
    allowed_hosts: string[];
    probe_actions: boolean;
  };
  pages: PageRecord[];
  requests: RequestEvidence[];
  scripts: ScriptEvidence[];
  capabilities: Capability[];
  unexplored: string[];
  issues: { url?: string; code: string; message: string }[];
  audit: {
    status_counts: Record<ImplementationStatus, number>;
    total_capabilities: number;
    unexplained_unexplored: string[];
    inaccessible_pages: string[];
    unexplored_urls: string[];
  };
};

export type ExplorationResult = {
  schema_version: 1;
  tool: { name: string; version: string };
  generated_at: string;
  targets: TargetResult[];
  summary: {
    target_count: number;
    page_count: number;
    request_count: number;
    capability_count: number;
    status_counts: Record<ImplementationStatus, number>;
  };
};

const DEFAULTS = {
  entryPaths: ["/"],
  headless: true,
  maxDepth: 1,
  maxPages: 50,
  maxRequests: 1_000,
  timeoutMs: 20_000,
  waitMs: 150,
  probeActions: false,
};

const WRITE_METHODS = new Set(["POST", "PUT", "PATCH", "DELETE"]);
const WRITE_INTENTS = new Set([
  "create",
  "update",
  "delete",
  "submit",
  "apply",
  "write",
]);
const SECRET_NAME = /password|passwd|token|ticket|captcha|secret|authorization|cookie|api[_-]?key|code/i;

export async function loadConfig(file: string): Promise<ExplorationConfig> {
  const path = resolve(file);
  const parsed: unknown = JSON.parse(await readFile(path, "utf8"));
  if (!parsed || typeof parsed !== "object") {
    throw new Error("配置文件必须是 JSON 对象");
  }
  const root = parsed as Record<string, unknown>;
  const rawTargets = Array.isArray(root.targets) ? root.targets : [root];
  if (rawTargets.length === 0) throw new Error("至少需要一个 targets 项");

  const targets = rawTargets.map((item, index) => {
    if (!item || typeof item !== "object") {
      throw new Error(`targets[${index}] 必须是对象`);
    }
    const target = normalizeTarget(item as TargetConfig, dirname(path));
    return target;
  });
  const output = typeof root.output === "string" ? resolve(dirname(path), root.output) : undefined;
  return { targets, output };
}

export async function runExploration(input: ExplorationConfig): Promise<ExplorationResult> {
  const targets = input.targets.map((target) => normalizeTarget(target, process.cwd()));
  const results: TargetResult[] = [];
  for (const target of targets) {
    results.push(await exploreTarget(target));
  }

  const statusCounts = emptyStatusCounts();
  for (const result of results) {
    for (const [status, count] of Object.entries(result.audit.status_counts)) {
      statusCounts[status as ImplementationStatus] += count;
    }
  }
  return {
    schema_version: 1,
    tool: { name: "csust-website-capability-exploration", version: "0.1.0" },
    generated_at: new Date().toISOString(),
    targets: results,
    summary: {
      target_count: results.length,
      page_count: results.reduce((sum, item) => sum + item.pages.length, 0),
      request_count: results.reduce((sum, item) => sum + item.requests.length, 0),
      capability_count: results.reduce((sum, item) => sum + item.capabilities.length, 0),
      status_counts: statusCounts,
    },
  };
}

export async function writeExplorationResult(file: string, result: ExplorationResult): Promise<string> {
  const output = resolve(file);
  await mkdir(dirname(output), { recursive: true });
  const temporary = `${output}.${process.pid}.tmp`;
  await writeFile(temporary, `${JSON.stringify(result, null, 2)}\n`, "utf8");
  await rename(temporary, output);
  return output;
}

function normalizeTarget(input: TargetConfig, baseDir: string): NormalizedTarget {
  if (!input || typeof input !== "object" || typeof input.baseUrl !== "string") {
    throw new Error("每个 target 都必须提供 baseUrl");
  }
  const base = new URL(input.baseUrl);
  if (!/^https?:$/.test(base.protocol)) throw new Error("baseUrl 只支持 HTTP(S)");
  const number = (value: unknown, fallback: number, name: string, min: number, max: number) => {
    const result = value === undefined ? fallback : Number(value);
    if (!Number.isInteger(result) || result < min || result > max) {
      throw new Error(`${name} 必须是 ${min} 到 ${max} 之间的整数`);
    }
    return result;
  };
  const entryPaths = input.entryPaths ?? DEFAULTS.entryPaths;
  if (!Array.isArray(entryPaths) || entryPaths.length === 0 || entryPaths.some((item) => typeof item !== "string")) {
    throw new Error("entryPaths 必须是非空字符串数组");
  }
  const allowedHosts = input.allowedHosts ?? [base.hostname];
  if (!Array.isArray(allowedHosts) || allowedHosts.length === 0 || allowedHosts.some((item) => typeof item !== "string")) {
    throw new Error("allowedHosts 必须是非空字符串数组");
  }
  const mappings = input.mappings ?? [];
  if (!Array.isArray(mappings)) throw new Error("mappings 必须是数组");
  for (const mapping of mappings) {
    if (!mapping || typeof mapping !== "object" || typeof mapping.pattern !== "string" || typeof mapping.command !== "string" || !mapping.pattern || !mapping.command) throw new Error("每条 mapping 都需要 pattern 和 command");
    if (mapping.status && !["implemented", "partially_implemented", "unsupported", "unexplored", "inaccessible"].includes(mapping.status)) {
      throw new Error(`mapping.status 无效: ${mapping.status}`);
    }
  }
  const headers = input.headers ?? {};
  if (!headers || typeof headers !== "object" || Array.isArray(headers) || Object.values(headers).some((value) => typeof value !== "string")) {
    throw new Error("headers 必须是字符串键值对象");
  }
  if (input.name !== undefined && typeof input.name !== "string") throw new Error("name 必须是字符串");
  if (input.authProfile !== undefined && typeof input.authProfile !== "string") throw new Error("authProfile 必须是字符串");
  if (input.domain !== undefined && typeof input.domain !== "string") throw new Error("domain 必须是字符串");
  if (input.storageState !== undefined && typeof input.storageState !== "string") throw new Error("storageState 必须是字符串路径");
  if (input.headless !== undefined && typeof input.headless !== "boolean") throw new Error("headless 必须是布尔值");
  if (input.probeActions !== undefined && typeof input.probeActions !== "boolean") throw new Error("probeActions 必须是布尔值");
  const storageState = input.storageState
    ? resolve(baseDir, input.storageState)
    : undefined;
  return {
    name: input.name?.trim() || base.hostname,
    baseUrl: base.toString(),
    entryPaths,
    allowedHosts,
    storageState,
    authProfile: input.authProfile?.trim() || (input.storageState ? "authenticated" : "anonymous"),
    domain: input.domain?.trim() || undefined,
    headers,
    headless: input.headless ?? DEFAULTS.headless,
    maxDepth: number(input.maxDepth, DEFAULTS.maxDepth, "maxDepth", 0, 5),
    maxPages: number(input.maxPages, DEFAULTS.maxPages, "maxPages", 1, 2_000),
    maxRequests: number(input.maxRequests, DEFAULTS.maxRequests, "maxRequests", 1, 10_000),
    timeoutMs: number(input.timeoutMs, DEFAULTS.timeoutMs, "timeoutMs", 1_000, 120_000),
    waitMs: number(input.waitMs, DEFAULTS.waitMs, "waitMs", 0, 5_000),
    probeActions: input.probeActions ?? DEFAULTS.probeActions,
    mappings,
  };
}

async function exploreTarget(target: NormalizedTarget): Promise<TargetResult> {
  const browser = await chromium.launch({ headless: target.headless });
  const contextOptions: Parameters<typeof browser.newContext>[0] = {
    extraHTTPHeaders: target.headers,
  };
  if (target.storageState) contextOptions.storageState = target.storageState;
  const context = await browser.newContext(contextOptions);
  if (target.probeActions) {
    await context.route("**/*", async (route) => {
      const request = route.request();
      if (isWriteMethod(request.method())) {
        await route.fulfill({
          status: 204,
          contentType: "application/json",
          body: "",
          headers: { "X-CSUST-Exploration": "dry-run" },
        });
        return;
      }
      await route.continue();
    });
  }

  const pages: PageRecord[] = [];
  const requests: RequestEvidence[] = [];
  const scripts: ScriptEvidence[] = [];
  const issues: { url?: string; code: string; message: string }[] = [];
  const visited = new Set<string>();
  const queue: { url: string; depth: number; from: string | null }[] = [];
  for (const entry of target.entryPaths) {
    const url = resolveUrl(target.baseUrl, entry);
    if (url && isAllowedUrl(url, target.allowedHosts)) queue.push({ url, depth: 0, from: null });
    else issues.push({ url: redactUrl(entry), code: "outside_allowlist", message: "入口不在 allowedHosts 内" });
  }

  try {
    while (queue.length > 0 && pages.length < target.maxPages) {
      const item = queue.shift()!;
      const key = normalizeUrl(item.url);
      if (!key || visited.has(key)) continue;
      visited.add(key);
      const result = await visitPage(context, target, item, pages.length + 1);
      pages.push(result.page);
      requests.push(...result.requests);
      scripts.push(...result.scripts);
      if (result.page.access === "inaccessible") {
        issues.push({ url: result.page.url, code: "inaccessible", message: result.page.error ?? "页面无法访问" });
        continue;
      }
      if (!result.page.snapshot || item.depth >= target.maxDepth) continue;
      for (const destination of result.discovered) {
        const next = normalizeUrl(destination);
        if (next && isAllowedUrl(next, target.allowedHosts) && !visited.has(next)) {
          queue.push({ url: next, depth: item.depth + 1, from: result.page.url });
        }
      }
    }
  } finally {
    await context.close();
    await browser.close();
  }

  const unexplored = [...new Set(queue.map((item) => redactUrl(item.url)))];
  if (unexplored.length > 0) {
    issues.push({ code: "max_pages_reached", message: `${unexplored.length} 个允许页面未访问` });
  }
  const capabilities = buildCapabilities(target, pages, requests, scripts);
  const statusCounts = emptyStatusCounts();
  for (const capability of capabilities) statusCounts[capability.implementation_status]++;
  return {
    target: {
      name: target.name,
      base_url: redactUrl(target.baseUrl),
      auth_profile: target.authProfile,
      allowed_hosts: target.allowedHosts,
      probe_actions: target.probeActions,
    },
    pages,
    requests,
    scripts,
    capabilities,
    unexplored,
    issues,
    audit: {
      status_counts: statusCounts,
      total_capabilities: capabilities.length,
      unexplained_unexplored: capabilities.filter((item) => item.implementation_status === "unexplored").map((item) => item.id),
      inaccessible_pages: pages.filter((item) => item.access === "inaccessible").map((item) => item.url),
      unexplored_urls: unexplored,
    },
  };
}

async function visitPage(
  context: BrowserContext,
  target: NormalizedTarget,
  item: { url: string; depth: number; from: string | null },
  pageNumber: number,
): Promise<{ page: PageRecord; requests: RequestEvidence[]; scripts: ScriptEvidence[]; discovered: string[] }> {
  const page = await context.newPage();
  const pageId = `page-${pageNumber}`;
  const captured: RequestEvidence[] = [];
  const byRequest = new Map<Request, RequestEvidence>();
  const pendingResponses: Promise<void>[] = [];
  const pendingDownloads: Promise<void>[] = [];
  const downloads: DownloadEvidence[] = [];
  let requestNumber = 0;
  const captureRequest = (request: Request) => {
    if (captured.length >= target.maxRequests) return;
    const evidence: RequestEvidence = {
      id: `${pageId}-request-${++requestNumber}`,
      page_id: pageId,
      url: redactUrl(request.url()),
      method: request.method(),
      resource_type: request.resourceType(),
      request_body: summarizeBody(request.postData(), request.headers()["content-type"]),
      dry_run: target.probeActions && isWriteMethod(request.method()),
    };
    captured.push(evidence);
    byRequest.set(request, evidence);
  };
  page.on("request", captureRequest);
  page.on("response", (response) => {
    const evidence = byRequest.get(response.request());
    if (!evidence) return;
    pendingResponses.push(fillResponse(evidence, response));
  });
  page.on("requestfailed", (request) => {
    const evidence = byRequest.get(request);
    if (evidence) evidence.failed = request.failure()?.errorText ?? "request failed";
  });
  page.on("download", (download) => {
    pendingDownloads.push((async () => {
      downloads.push({ url: redactUrl(download.url()), suggested_filename: download.suggestedFilename(), failure: await download.failure() });
    })());
  });

  let response: Response | null = null;
  let snapshot: PageSnapshot | null = null;
  let liveSnapshot: PageSnapshot | null = null;
  let dryRunProbes: DryRunProbe[] = [];
  let error: string | undefined;
  try {
    response = await page.goto(item.url, { waitUntil: "domcontentloaded", timeout: target.timeoutMs });
    await page.waitForLoadState("networkidle", { timeout: Math.min(target.timeoutMs, 5_000) }).catch(() => undefined);
    if (target.waitMs > 0) await page.waitForTimeout(target.waitMs);
    // The TS runtime adds __name around browser-local callbacks; provide that
    // tiny identity helper when evaluating the serialized extractor.
    liveSnapshot = await page.evaluate(`(() => { const __name = (fn) => fn; return (${extractPageSnapshot.toString()})(); })()`) as PageSnapshot;
    snapshot = redactSnapshot(liveSnapshot);
  } catch (cause) {
    error = cleanError(cause);
  }
  if (target.probeActions && snapshot) {
    const probeInputs = liveSnapshot?.forms.map(({ ref, method, write }) => ({ ref, method, write })) ?? [];
    try {
      dryRunProbes = (await page.evaluate(`(() => { const __name = (fn) => fn; return (${probeWriteForms.toString()})(${JSON.stringify(probeInputs)}); })()`) as DryRunProbe[]).map((item) => ({ ...item, action: redactUrl(item.action) }));
      if (dryRunProbes.length > 0) await page.waitForTimeout(Math.min(target.waitMs + 50, 500));
    } catch {
      // A page-specific submit handler must not stop reconnaissance.
    }
  }
  await Promise.allSettled(pendingResponses);
  await Promise.allSettled(pendingDownloads);

  const actualUrl = normalizeUrl(page.url()) || item.url;
  const status = response?.status() ?? null;
  const redirects = response ? redirectChain(response.request()) : [];
  const access = response && status !== null && status >= 400 ? "inaccessible" : error ? "inaccessible" : "accessible";
  const authentication = authenticationState(target, snapshot, captured, status, redirects);
  const record: PageRecord = {
    id: pageId,
    requested_url: redactUrl(item.url),
    url: redactUrl(actualUrl),
    depth: item.depth,
    discovered_from: item.from ? redactUrl(item.from) : null,
    access,
    status,
    redirects,
    title: snapshot?.title ?? "",
    kind: snapshot?.kind ?? "unknown",
    authentication,
    snapshot,
    dry_run_probes: dryRunProbes,
    downloads,
    request_ids: captured.map((item) => item.id),
    script_ids: [],
    ...(error ? { error } : {}),
  };
  const pageScripts = await inspectScripts(context, target, record, liveSnapshot?.scripts ?? []);
  record.script_ids = pageScripts.map((item) => item.id);
  const discovered = liveSnapshot
    ? [...liveSnapshot.links.filter((item) => item.same_origin && !isDownloadUrl(item.href)).map((item) => item.href), ...liveSnapshot.iframes.map((item) => item.src)]
    : [];
  await page.close();
  return { page: record, requests: captured, scripts: pageScripts, discovered };
}

function probeWriteForms(forms: { ref: string; method: string; write: boolean }[]): DryRunProbe[] {
  const secret = /password|passwd|token|ticket|captcha|secret|authorization|cookie|api[_-]?key|code/i;
  const result: DryRunProbe[] = [];
  for (const item of forms) {
    if (!item.write || item.method.toUpperCase() === "GET") continue;
    const index = Number(item.ref.replace("form-", "")) - 1;
    const form = document.forms[index];
    if (!form) continue;
    for (const element of Array.from(form.elements)) {
      const input = element as HTMLInputElement;
      if (input.disabled || !input.required || input.value || !input.name) continue;
      if (input.type === "checkbox" || input.type === "radio") input.checked = true;
      else if (input.type !== "file" && input.type !== "hidden") input.value = "exploration-probe";
    }
    let fields: string[] = [];
    try {
      fields = [...new FormData(form).keys()].filter((name) => !secret.test(name)).slice(0, 100);
    } catch {
      // Keep the probe record even when a malformed control rejects FormData.
    }
    let dispatched = false;
    try {
      const event = typeof SubmitEvent === "function"
        ? new SubmitEvent("submit", { bubbles: true, cancelable: true })
        : new Event("submit", { bubbles: true, cancelable: true });
      dispatched = form.dispatchEvent(event);
    } catch {
      // A page-specific handler must not stop the rest of exploration.
    }
    result.push({ ref: item.ref, action: form.action || location.href, method: item.method.toUpperCase(), fields, valid: form.checkValidity(), dispatched });
  }
  return result;
}

function redactSnapshot(snapshot: PageSnapshot): PageSnapshot {
  return {
    ...snapshot,
    links: snapshot.links.map((item) => ({ ...item, href: redactUrl(item.href) })),
    forms: snapshot.forms.map((item) => ({ ...item, action: redactUrl(item.action) })),
    scripts: snapshot.scripts.map((item) => ({ ...item, src: item.src ? redactUrl(item.src) : null, source: undefined })),
    iframes: snapshot.iframes.map((item) => ({ ...item, src: redactUrl(item.src) })),
  };
}

async function inspectScripts(
  context: BrowserContext,
  target: NormalizedTarget,
  page: PageRecord,
  scripts: PageScript[],
): Promise<ScriptEvidence[]> {
  const result: ScriptEvidence[] = [];
  for (const script of scripts) {
    const id = `${page.id}-${script.ref}`;
    const url = script.src ? normalizeUrl(script.src) : `${page.url}#${script.ref}`;
    if (!url || (!script.src && !script.source)) continue;
    if (!script.src) {
      const source = script.source ?? "";
      const hints = extractScriptHints(source.slice(0, 500_000));
      result.push({ id, page_id: page.id, url: redactUrl(url), status: null, bytes: Buffer.byteLength(source), ...hints });
      continue;
    }
    if (!isAllowedUrl(url, target.allowedHosts)) {
      result.push({ id, page_id: page.id, url: redactUrl(url), status: null, bytes: 0, endpoints: [], routes: [], actions: [], skipped: "external" });
      continue;
    }
    try {
      const response = await context.request.get(url, { timeout: target.timeoutMs });
      const source = await response.text();
      const hints = extractScriptHints(source.slice(0, 500_000));
      result.push({
        id,
        page_id: page.id,
        url: redactUrl(url),
        status: response.status(),
        bytes: Buffer.byteLength(source),
        ...hints,
      });
    } catch (cause) {
      result.push({ id, page_id: page.id, url: redactUrl(url), status: null, bytes: 0, endpoints: [], routes: [], actions: [], error: cleanError(cause) });
    }
  }
  return result;
}

async function fillResponse(evidence: RequestEvidence, response: Response): Promise<void> {
  const headers = response.headers();
  const contentType = headers["content-type"]?.split(";", 1)[0] ?? "";
  const item: NonNullable<RequestEvidence["response"]> = {
    status: response.status(),
    content_type: contentType,
  };
  const declaredLength = Number(headers["content-length"]);
  if (!contentType.includes("json")) {
    if (Number.isFinite(declaredLength)) item.bytes = declaredLength;
    evidence.response = item;
    return;
  }
  // ponytail: JSON shape inspection stops at 2 MiB; stream/sample larger responses if needed.
  if (Number.isFinite(declaredLength) && declaredLength > 2_000_000) {
    item.bytes = declaredLength;
    evidence.response = item;
    return;
  }
  try {
    const body = await response.body();
    item.bytes = body.byteLength;
    if (contentType.includes("json")) {
      const parsed: unknown = JSON.parse(body.toString("utf8"));
      item.shape = shapeOf(parsed);
    }
  } catch {
    const length = Number(headers["content-length"]);
    if (Number.isFinite(length)) item.bytes = length;
  }
  evidence.response = item;
}

function buildCapabilities(
  target: NormalizedTarget,
  pages: PageRecord[],
  requests: RequestEvidence[],
  scripts: ScriptEvidence[],
): Capability[] {
  const result: Capability[] = [];
  for (const page of pages) {
    const snapshot = page.snapshot;
    const pageEvidence: Evidence[] = [{ kind: "page", ref: page.id, detail: `${page.access} page observation` }];
    result.push(capability(target, page, "page.view", "page", page.url, {
      type: "browser-navigation",
      method: "GET",
      url: page.url,
    }, { depth: page.depth, requested_url: page.requested_url }, { title: page.title, kind: page.kind, status: page.status, access: page.access, redirects: page.redirects }, "read", ["navigation result and HTTP status"], pageEvidence));
    if (!snapshot) continue;

    const internalLinks = snapshot.links.filter((link) => link.same_origin);
    if (snapshot.links.length > 0) {
      result.push(capability(target, page, "navigation.links", "navigation", page.url, {
        type: "HTML link",
        method: "GET",
        url: page.url,
      }, { link_count: snapshot.links.length, internal_count: internalLinks.length }, { destinations: snapshot.links.slice(0, 100).map((link) => ({ ref: link.ref, url: link.href, intent: link.intent })) }, "read", ["destination page or response status"], pageEvidence));
    }
    for (const form of snapshot.forms) {
      const write = form.write;
      const evidence: Evidence[] = [{ kind: "element", ref: `${page.id}-${form.ref}`, detail: "HTML form" }];
      result.push(capability(target, page, `form.${form.intent}`, "form", page.url, {
        type: "HTML form",
        method: form.method,
        url: form.action,
        enctype: form.enctype,
      }, { ref: form.ref, controls: form.controls, required: form.controls.filter((item) => item.required).map((item) => item.name) }, { intent: form.intent, submitted: false, dry_run: write, probe: page.dry_run_probes.find((item) => item.ref === form.ref) ?? null }, write ? "write" : "read", write ? ["response status", "success message or post-state; not executed by default"] : ["response status", "resulting page or data state"], evidence));
      if (form.controls.some((control) => control.file)) {
        result.push(capability(target, page, "file.upload", "upload", page.url, {
          type: "multipart/form-data",
          method: form.method,
          url: form.action,
        }, { form_ref: form.ref, file_fields: form.controls.filter((item) => item.file).map((item) => item.name) }, { submitted: false, dry_run: true }, "write", ["upload response and resulting resource state; not executed by default"], evidence));
      }
    }
    for (const action of snapshot.actions) {
      const evidence: Evidence[] = [{ kind: "element", ref: `${page.id}-${action.ref}`, detail: `${action.type}: ${action.label}` }];
      result.push(capability(target, page, `action.${action.intent}`, "action", page.url, {
        type: "browser action",
        ref: action.ref,
      }, { label: action.label, form_ref: action.form_ref }, { intent: action.intent, executed: false, dry_run: true }, action.write ? "write" : "read", action.write ? ["request status and success state; action not executed"] : ["resulting page or response status"], evidence));
    }
    for (const link of snapshot.links.filter((item) => item.has_onclick)) {
      const write = WRITE_INTENTS.has(link.intent);
      result.push(capability(target, page, `action.onclick.${link.intent}`, "action", page.url, {
        type: "browser action",
        ref: link.ref,
      }, { label: link.text, href: link.href }, { intent: link.intent, executed: false, dry_run: true }, write ? "write" : "read", write ? ["request status and success state; action not executed"] : ["resulting page or response status"], [{ kind: "element", ref: `${page.id}-${link.ref}`, detail: "link with onclick handler" }]));
    }
    for (const link of snapshot.links.filter((item) => item.download || item.intent === "download" || item.intent === "export")) {
      result.push(capability(target, page, "file.download", "download", page.url, {
        type: "HTML link",
        method: "GET",
        url: link.href,
      }, { ref: link.ref, label: link.text }, { downloaded: false }, "read", ["download response and saved file metadata"], [{ kind: "element", ref: `${page.id}-${link.ref}`, detail: "download/export link" }]));
    }
    for (const download of page.downloads) {
      result.push(capability(target, page, "file.download", "download", page.url, {
        type: "browser download",
        method: "GET",
        url: download.url,
      }, { suggested_filename: download.suggested_filename }, { downloaded: true, failure: download.failure }, "read", ["download response and saved file metadata"], [{ kind: "download", ref: `${page.id}-${download.url}`, detail: "download event" }]));
    }
    for (const frame of snapshot.iframes) {
      result.push(capability(target, page, "embedded.frame", "iframe", page.url, {
        type: "iframe navigation",
        method: "GET",
        url: frame.src,
      }, { ref: frame.ref, name: frame.name, title: frame.title }, { discovered: true }, "read", ["frame navigation status and frame page snapshot"], [{ kind: "element", ref: `${page.id}-${frame.ref}`, detail: "iframe" }]));
    }
  }

  for (const request of requests.filter(isApiRequest)) {
    const page = pages.find((item) => item.id === request.page_id);
    if (!page) continue;
    const write = isWriteMethod(request.method);
    const evidence: Evidence[] = [{ kind: "request", ref: request.id, detail: `${request.method} ${request.url}` }];
    if (request.dry_run) evidence.push({ kind: "dry_run", ref: request.id, detail: "mutating request intercepted before network delivery" });
    result.push(capability(target, page, `api.${request.method.toLowerCase()}`, "api", page.url, {
      type: "HTTP",
      method: request.method,
      url: request.url,
      resource_type: request.resource_type,
    }, { query_or_body: request.request_body ?? null }, { response: request.response ?? null, failed: request.failed ?? null, submitted: write && !request.dry_run }, write ? "write" : "read", request.dry_run ? ["request was intercepted; real success state not verified"] : ["HTTP response status", "response shape/content type"], evidence));
  }
  for (const script of scripts) {
    const page = pages.find((item) => item.id === script.page_id);
    if (!page) continue;
    result.push(capability(target, page, "bundle.inspect", "bundle", page.url, {
      type: "JavaScript bundle",
      method: "GET",
      url: script.url,
    }, { script_id: script.id }, { status: script.status, bytes: script.bytes, endpoints: script.endpoints, routes: script.routes, actions: script.actions }, "read", ["bundle response status", "cross-check extracted route/API hints against observed requests"], [{ kind: "script", ref: script.id, detail: "JavaScript bundle inspection" }]));
  }
  return result;
}

function capability(
  target: NormalizedTarget,
  page: PageRecord,
  name: string,
  kind: string,
  entry: string,
  protocol: Record<string, unknown>,
  inputs: Record<string, unknown>,
  outputs: Record<string, unknown>,
  readWrite: "read" | "write",
  verification: string[],
  evidence: Evidence[],
): Capability {
  const mapping = findMapping(target, name, page.url, entry);
  const implementationStatus = page.access === "inaccessible"
    ? "inaccessible"
    : mapping.rule?.status ?? (mapping.rule ? "partially_implemented" : "unexplored");
  return {
    id: `${target.name}:${page.id}:${name}:${evidence[0]?.ref ?? "item"}`,
    capability: name,
    kind,
    entry: redactUrl(entry),
    page: page.url,
    underlying_protocol: redactRecord(protocol),
    authentication: page.authentication,
    inputs: redactRecord(inputs),
    outputs: redactRecord(outputs),
    read_write: readWrite,
    verification_method: verification,
    confidence: page.access === "inaccessible" ? "low" : evidence.some((item) => item.kind === "request") ? "high" : "medium",
    evidence,
    cli_mapping: mapping.value,
    implementation_status: implementationStatus,
  };
}

function findMapping(target: NormalizedTarget, capabilityName: string, page: string, entry: string): { value: CliMapping; rule?: MappingRule } {
  const haystack = `${capabilityName} ${page} ${entry}`.toLowerCase();
  const rule = target.mappings.find((item) => haystack.includes(item.pattern.toLowerCase()));
  if (rule) {
    return { rule, value: { status: "mapped", command: rule.command, matched_pattern: rule.pattern } };
  }
  return {
    value: {
      status: "unmapped",
      ...(target.domain ? { suggested_command: `${target.domain} ${suggestIntent(capabilityName)}` } : {}),
      reason: "no semantic CLI mapping configured; discovery is not implementation",
    },
  };
}

function suggestIntent(capabilityName: string): string {
  const intent = capabilityName.split(".").slice(1).join("-");
  return intent || "inspect";
}

function authenticationState(target: NormalizedTarget, snapshot: PageSnapshot | null, requests: RequestEvidence[], status: number | null, redirects: string[]): PageRecord["authentication"] {
  const signals = new Set(snapshot?.signals ?? []);
  if (snapshot?.kind === "login") signals.add("login_page");
  if (status === 401 || status === 403 || requests.some((item) => item.response?.status === 401 || item.response?.status === 403)) signals.add("authorization_failure");
  if (redirects.some((item) => /cas|sso|login|auth/i.test(item))) signals.add("auth_redirect");
  if (target.storageState) signals.add("storage_state_loaded");
  return { profile: target.authProfile, storage_state: Boolean(target.storageState), signals: [...signals] };
}

function redirectChain(request: Request): string[] {
  const chain: string[] = [];
  let current: Request | null = request.redirectedFrom();
  while (current) {
    chain.unshift(redactUrl(current.url()));
    current = current.redirectedFrom();
  }
  if (chain.length > 0) chain.push(redactUrl(request.url()));
  return chain;
}

function extractPageSnapshot(): PageSnapshot {
  const clean = (value: string | null | undefined, length = 180) => (value ?? "").replace(/\s+/g, " ").trim().slice(0, length);
  const writeIntents = new Set(["create", "update", "delete", "submit", "apply", "write"]);
  const intent = (value: string) => {
    const text = value.toLowerCase();
    if (/delete|remove|撤销|删除/.test(text)) return "delete";
    if (/update|edit|修改|更新/.test(text)) return "update";
    if (/create|add|new|创建|新增/.test(text)) return "create";
    if (/submit|save|apply|register|报名|保存|提交|申请/.test(text)) return "submit";
    if (/upload|import|上传|导入/.test(text)) return "upload";
    if (/download|export|下载|导出/.test(text)) return text.includes("export") || text.includes("导出") ? "export" : "download";
    if (/search|query|filter|find|查询|搜索|筛选/.test(text)) return "search";
    if (/next|prev|page|分页|下一页|上一页/.test(text)) return "pagination";
    return "navigate";
  };
  const isWrite = (method: string, kind: string) => method !== "GET" || writeIntents.has(kind);
  const links = Array.from(document.querySelectorAll<HTMLAnchorElement>("a[href]")).slice(0, 300).map((link, index) => {
    const href = link.href;
    const label = clean(link.innerText || link.getAttribute("aria-label") || link.title);
    return {
      ref: `link-${index + 1}`,
      href,
      text: label,
      download: link.getAttribute("download"),
      intent: intent(`${label} ${href} ${link.getAttribute("onclick") ?? ""}`),
      has_onclick: Boolean(link.getAttribute("onclick")),
      same_origin: (() => {
        try { return new URL(href).origin === location.origin; } catch { return false; }
      })(),
    };
  });
  const forms = Array.from(document.forms).slice(0, 100).map((form, index) => {
    const action = form.action || location.href;
    const method = (form.method || "GET").toUpperCase();
    const controls = Array.from(form.elements).slice(0, 100).map((element) => {
      const input = element as HTMLInputElement;
      const select = element as HTMLSelectElement;
      return {
        name: input.name || input.id || "",
        type: input.type || element.tagName.toLowerCase(),
        required: input.required,
        disabled: input.disabled,
        file: input.type === "file",
        options: element instanceof HTMLSelectElement ? Array.from(select.options).slice(0, 30).map((option) => clean(option.text)) : [],
      };
    });
    const label = clean(`${form.getAttribute("aria-label") ?? ""} ${form.innerText}`);
    const kind = intent(`${label} ${action} ${controls.map((item) => item.name).join(" ")}`);
    return {
      ref: `form-${index + 1}`,
      action,
      method,
      enctype: form.enctype || "application/x-www-form-urlencoded",
      intent: kind,
      write: isWrite(method, kind),
      controls,
    };
  });
  const actions: PageAction[] = [];
  const actionElements = Array.from(document.querySelectorAll<HTMLElement>("button, input[type=submit], input[type=button], [role=button]")).slice(0, 200);
  for (const [index, element] of actionElements.entries()) {
    const label = clean(element.innerText || (element as HTMLInputElement).value || element.getAttribute("aria-label") || element.title);
    const kind = intent(`${label} ${element.getAttribute("name") ?? ""} ${element.getAttribute("data-action") ?? ""}`);
    const form = element.closest("form");
    const formRef = form ? `form-${Array.from(document.forms).indexOf(form) + 1}` : null;
    const formMethod = form ? (form.method || "GET").toUpperCase() : "GET";
    actions.push({ ref: `action-${index + 1}`, label, type: element.tagName.toLowerCase(), intent: kind, write: isWrite(formMethod, kind), form_ref: formRef });
  }
  const scripts = Array.from(document.scripts).slice(0, 200).map((script, index) => ({ ref: `script-${index + 1}`, src: script.src || null, type: script.type || "text/javascript", inline: !script.src, ...(script.src ? {} : { source: (script.textContent ?? "").slice(0, 500_000) }) }));
  const iframes = Array.from(document.querySelectorAll<HTMLIFrameElement>("iframe")).slice(0, 100).map((frame, index) => ({ ref: `iframe-${index + 1}`, src: frame.src, name: frame.name, title: frame.title }));
  const bodyText = (document.body?.innerText ?? "").slice(0, 50_000).toLowerCase();
  const signals = new Set<string>();
  if (document.querySelector('input[type="password"]') || /login|sign in|登录|统一认证|cas/.test(`${location.href} ${document.title} ${bodyText}`)) signals.add("login");
  if (forms.some((form) => form.intent === "search") || /search|query|查询|搜索/.test(bodyText)) signals.add("search");
  if (links.some((link) => link.intent === "pagination") || /next page|上一页|下一页|第\s*\d+\s*页/.test(bodyText)) signals.add("pagination");
  if (forms.some((form) => form.controls.some((control) => control.file))) signals.add("upload");
  if (links.some((link) => link.download || link.intent === "download" || link.intent === "export")) signals.add("download");
  const kind = document.querySelector('input[type="password"]') || /login|sign in|登录|统一认证|cas/.test(`${location.href} ${document.title}`)
    ? "login"
    : document.querySelector("table")
      ? "data"
      : signals.has("search")
        ? "search"
        : "document";
  return { title: clean(document.title, 240), kind, links, forms, actions, scripts, iframes, signals: [...signals] };
}

function extractScriptHints(source: string): { endpoints: string[]; routes: string[]; actions: string[] } {
  const values = new Set<string>();
  const pattern = /["'`]((?:https?:\/\/|\/)[^"'`\\\s]{2,240})["'`]/g;
  for (const match of source.matchAll(pattern)) {
    const value = match[1];
    if (!value || value.startsWith("//") || /\.(?:js|css|png|jpg|jpeg|gif|svg|woff2?)(?:[?#]|$)/i.test(value)) continue;
    values.add(value);
  }
  const routes = [...values];
  const endpoints = routes.filter((value) => /\/(?:api|ajax|graphql|rest|v\d+)(?:\/|$)|\.(?:json|do|action)(?:[?#]|$)|query|search|save|submit|delete|update|login|logout/i.test(value));
  const actions = [...new Set([...source.matchAll(/\b(create|read|update|delete|query|search|save|submit|login|logout|upload|download)\b/gi)].map((match) => match[1].toLowerCase()))];
  return { endpoints: endpoints.slice(0, 100).map(redactUrl), routes: routes.slice(0, 100).map(redactUrl), actions: actions.slice(0, 50) };
}

function isApiRequest(request: RequestEvidence): boolean {
  return ["xhr", "fetch"].includes(request.resource_type) || Boolean(request.response?.content_type.includes("json")) || /\/(?:api|ajax|graphql|rest|v\d+)(?:\/|$)|\.(?:json|do|action)(?:[?#]|$)/i.test(request.url);
}

function summarizeBody(body: string | null, contentType: string | undefined): BodySummary | undefined {
  if (!body) return undefined;
  const keys = new Set<string>();
  const kind = contentType?.split(";", 1)[0] ?? "unknown";
  try {
    const parsed: unknown = JSON.parse(body);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      for (const key of Object.keys(parsed)) if (!SECRET_NAME.test(key)) keys.add(key);
    }
  } catch {
    for (const part of body.split("&")) {
      const key = decodeURIComponent(part.split("=", 1)[0] ?? "");
      if (key && !SECRET_NAME.test(key)) keys.add(key);
    }
  }
  return { kind, bytes: Buffer.byteLength(body), keys: [...keys].slice(0, 100) };
}

function shapeOf(value: unknown): string[] {
  if (Array.isArray(value)) return ["array", ...(value[0] && typeof value[0] === "object" ? Object.keys(value[0] as object).filter((key) => !SECRET_NAME.test(key)).slice(0, 50) : [])];
  if (value && typeof value === "object") return Object.keys(value).filter((key) => !SECRET_NAME.test(key)).slice(0, 100);
  return [typeof value];
}

function resolveUrl(base: string, value: string): string | null {
  try { return normalizeUrl(new URL(value, base).toString()); } catch { return null; }
}

function normalizeUrl(value: string): string {
  try {
    const url = new URL(value);
    url.hash = "";
    return url.toString();
  } catch { return ""; }
}

function isAllowedUrl(value: string, allowedHosts: string[]): boolean {
  try {
    const hostname = new URL(value).hostname.toLowerCase();
    return allowedHosts.some((item) => {
      const raw = item.toLowerCase().replace(/^https?:\/\//, "").replace(/\/$/, "");
      const wildcard = raw.startsWith("*.");
      const host = raw.replace(/^\*\./, "");
      try {
        const normalized = new URL(`http://${host}`).hostname.toLowerCase();
        return wildcard ? hostname.endsWith(`.${normalized}`) : hostname === normalized;
      } catch {
        return false;
      }
    });
  } catch { return false; }
}

function isDownloadUrl(value: string): boolean {
  return /\.(?:pdf|docx?|xlsx?|pptx?|zip|rar|7z|csv|jpg|jpeg|png)(?:[?#]|$)/i.test(value);
}

function isWriteMethod(method: string): boolean {
  return WRITE_METHODS.has(method.toUpperCase());
}

function redactUrl(value: string): string {
  const relative = value.startsWith("/") && !value.startsWith("//");
  try {
    const url = new URL(value, "http://redaction.invalid");
    for (const key of [...url.searchParams.keys()]) if (SECRET_NAME.test(key)) url.searchParams.set(key, "[redacted]");
    if (url.username || url.password) { url.username = ""; url.password = ""; }
    return relative ? `${url.pathname}${url.search}${url.hash}` : url.toString();
  } catch { return value; }
}

function redactRecord(value: Record<string, unknown>): Record<string, unknown> {
  return redactValue(value) as Record<string, unknown>;
}

function redactValue(value: unknown, key = ""): unknown {
  if (SECRET_NAME.test(key)) return "[redacted]";
  if (typeof value === "string") return /url|href|action/i.test(key) || /^https?:\/\//.test(value) ? redactUrl(value) : value;
  if (Array.isArray(value)) return value.map((item) => redactValue(item, key));
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([childKey, item]) => [childKey, redactValue(item, childKey)]));
  }
  return value;
}

function cleanError(cause: unknown): string {
  const message = cause instanceof Error ? cause.message : String(cause);
  return message.replace(/https?:\/\/\S+/g, "[url]").slice(0, 500);
}

function emptyStatusCounts(): Record<ImplementationStatus, number> {
  return { implemented: 0, partially_implemented: 0, unsupported: 0, unexplored: 0, inaccessible: 0 };
}
