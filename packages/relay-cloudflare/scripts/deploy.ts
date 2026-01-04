#!/usr/bin/env npx tsx
/**
 * Deploy script for Cloudflare Workers
 *
 * このスクリプトは以下を実行します:
 * 1. public/ ディレクトリの存在を確認
 * 2. config.ts から設定を読み込み
 * 3. RELAY_CONFIG シークレットを設定
 * 4. wrangler deploy を実行
 *
 * 注意: webアセットのビルドとコピーは事前に行う必要があります。
 *   make assets   # このディレクトリで実行
 *   make deploy   # アセットビルド + デプロイ
 *
 * 使用方法:
 *   pnpm deploy        # 本番環境
 *   pnpm deploy:dev    # 開発環境
 */

import { spawn, spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));
const packageRoot = resolve(__dirname, "..");
const publicPath = resolve(packageRoot, "public");

interface DeployOptions {
  environment?: string;
  skipSecrets?: boolean;
}

/**
 * Check that public directory exists with assets.
 */
function checkPublicDir(): void {
  if (!existsSync(publicPath)) {
    console.error("Error: public/ directory not found");
    console.error("");
    console.error("Run 'make assets' to build and copy web assets first.");
    console.error("Or run 'make deploy' for full deploy with assets.");
    process.exit(1);
  }

  const indexHtml = resolve(publicPath, "index.html");
  if (!existsSync(indexHtml)) {
    console.error("Error: public/index.html not found");
    console.error("");
    console.error("Run 'make assets' to build and copy web assets.");
    process.exit(1);
  }
}

async function loadConfig(): Promise<{
  config: unknown;
  cloudflareConfig?: { environment?: string };
}> {
  const configPath = resolve(packageRoot, "config.ts");

  if (!existsSync(configPath)) {
    console.error("Error: config.ts not found");
    console.error("");
    console.error("Please create config.ts from the example:");
    console.error("  cp config.example.ts config.ts");
    console.error("");
    console.error("Then edit config.ts with your settings.");
    process.exit(1);
  }

  // Dynamic import with file:// URL for Windows compatibility
  const configModule = await import(`file://${configPath}`);
  return {
    config: configModule.config,
    cloudflareConfig: configModule.cloudflareConfig,
  };
}

function setSecret(
  name: string,
  value: string,
  environment?: string
): Promise<void> {
  return new Promise((resolve, reject) => {
    const args = ["wrangler", "secret", "put", name];
    if (environment) {
      args.push("--env", environment);
    }

    console.log(`Setting secret: ${name}${environment ? ` (env: ${environment})` : ""}`);

    const proc = spawn("npx", args, {
      cwd: packageRoot,
      stdio: ["pipe", "inherit", "inherit"],
    });

    proc.stdin.write(value);
    proc.stdin.end();

    proc.on("close", (code) => {
      if (code === 0) {
        resolve();
      } else {
        reject(new Error(`Failed to set secret ${name} (exit code: ${code})`));
      }
    });

    proc.on("error", reject);
  });
}

function deploy(environment?: string): void {
  const args = ["wrangler", "deploy"];
  if (environment) {
    args.push("--env", environment);
  }

  console.log("");
  console.log(`Deploying${environment ? ` to ${environment}` : ""}...`);
  console.log("");

  const result = spawnSync("npx", args, {
    cwd: packageRoot,
    stdio: "inherit",
  });

  if (result.status !== 0) {
    console.error("Deploy failed");
    process.exit(result.status ?? 1);
  }
}

async function main(): Promise<void> {
  const args = process.argv.slice(2);
  const options: DeployOptions = {};

  // Parse arguments
  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === "--env" || arg === "-e") {
      options.environment = args[++i];
    } else if (arg === "--skip-secrets") {
      options.skipSecrets = true;
    } else if (arg === "--help" || arg === "-h") {
      console.log("Usage: deploy.ts [options]");
      console.log("");
      console.log("Options:");
      console.log("  --env, -e <env>   Deploy to specific environment (dev, staging)");
      console.log("  --skip-secrets    Skip setting secrets (use existing)");
      console.log("  --help, -h        Show this help");
      console.log("");
      console.log("Note: Web assets must be built first with 'make assets'");
      console.log("Or use 'make deploy' to build assets and deploy together.");
      process.exit(0);
    }
  }

  // Check public directory exists
  checkPublicDir();

  // Load config
  const { config, cloudflareConfig } = await loadConfig();

  // Use environment from config if not specified in args
  const environment = options.environment ?? cloudflareConfig?.environment;

  // Set secrets
  if (!options.skipSecrets) {
    const configJson = JSON.stringify(config);
    await setSecret("RELAY_CONFIG", configJson, environment);
    console.log("Secret set successfully");
  }

  // Deploy
  deploy(environment);

  console.log("");
  console.log("Deploy completed successfully!");
}

main().catch((err) => {
  console.error("Error:", err.message);
  process.exit(1);
});
