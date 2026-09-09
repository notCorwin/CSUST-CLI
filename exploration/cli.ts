#!/usr/bin/env node
import { loadConfig, runExploration, writeExplorationResult } from "./index.js";

type Options = { config: string; output?: string; headed: boolean; probeActions: boolean };

async function main(): Promise<void> {
  const options = parseArgs(process.argv.slice(2));
  if (options.config === "") {
    printHelp();
    process.exitCode = 2;
    return;
  }
  const config = await loadConfig(options.config);
  if (options.headed || options.probeActions) {
    config.targets = config.targets.map((target) => ({
      ...target,
      ...(options.headed ? { headless: false } : {}),
      ...(options.probeActions ? { probeActions: true } : {}),
    }));
  }
  const result = await runExploration(config);
  const output = options.output ?? config.output;
  if (output) {
    const path = await writeExplorationResult(output, result);
    process.stdout.write(`${JSON.stringify({ ok: true, output: path, ...result.summary }, null, 2)}\n`);
  } else {
    process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
  }
}

function parseArgs(args: string[]): Options {
  const options: Options = { config: "", headed: false, probeActions: false };
  for (let index = 0; index < args.length; index++) {
    const arg = args[index];
    if (arg === "--help" || arg === "-h") {
      printHelp();
      process.exit(0);
    }
    if (arg === "--headed") { options.headed = true; continue; }
    if (arg === "--probe-actions") { options.probeActions = true; continue; }
    const [name, inline] = arg.split("=", 2);
    if (name === "--config" || name === "--output") {
      const value = inline ?? args[++index];
      if (!value || value.startsWith("--")) throw new Error(`${name} 缺少参数值`);
      if (name === "--config") options.config = value;
      else options.output = value;
      continue;
    }
    throw new Error(`未知参数: ${arg}`);
  }
  return options;
}

function printHelp(): void {
  process.stdout.write(`用法: npm run explore -- --config <file> [选项]

选项:
  --config FILE       JSON 探索配置
  --output FILE       覆盖配置中的输出路径；不提供则输出完整 JSON
  --headed            显示浏览器，适合人工观察站点登录/跳转流程
  --probe-actions     拦截页面产生的 POST/PUT/PATCH/DELETE，记录 dry-run 证据
  --help              显示帮助
`);
}

main().catch((cause) => {
  const message = cause instanceof Error ? cause.message : String(cause);
  process.stderr.write(`探索失败: ${message}\n`);
  process.exitCode = 2;
});
