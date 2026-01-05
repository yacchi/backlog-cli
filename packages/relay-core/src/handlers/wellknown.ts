/**
 * Well-known endpoint handler for relay server discovery.
 */

import { Hono } from "hono";
import type { RelayConfig } from "../config/types.js";

/**
 * Well-known response type.
 */
interface WellKnownResponse {
  /** Version of the well-known response format */
  version: string;
  /** Relay server capabilities */
  capabilities: string[];
  /** Supported Backlog domains */
  supported_domains: string[];
}

/**
 * Landing page content for different languages.
 */
interface LandingPageContent {
  lang: string;
  title: string;
  heading: string;
  description: string;
  howToUseTitle: string;
  howToUseText: string;
  gettingStartedTitle: string;
  gettingStartedText: string;
  githubLink: string;
  apiDiscoveryLink: string;
  healthCheckLink: string;
}

const CONTENT_EN: LandingPageContent = {
  lang: "en",
  title: "Backlog CLI OAuth Relay Server",
  heading: "Backlog CLI OAuth Relay Server",
  description:
    "This is an OAuth 2.0 relay server for <strong>Backlog CLI</strong>.<br>It securely handles authentication without exposing client secrets.",
  howToUseTitle: "How to Use",
  howToUseText:
    "This server is designed to be accessed via the Backlog CLI tool.<br>Direct browser access is not intended.",
  gettingStartedTitle: "Getting Started",
  gettingStartedText: "Install Backlog CLI and run:",
  githubLink: "GitHub Repository",
  apiDiscoveryLink: "API Discovery",
  healthCheckLink: "Health Check",
};

const CONTENT_JA: LandingPageContent = {
  lang: "ja",
  title: "Backlog CLI OAuth 中継サーバー",
  heading: "Backlog CLI OAuth 中継サーバー",
  description:
    "これは <strong>Backlog CLI</strong> 用の OAuth 2.0 中継サーバーです。<br>クライアントシークレットを公開せずに安全に認証を処理します。",
  howToUseTitle: "使い方",
  howToUseText:
    "このサーバーは Backlog CLI ツールからアクセスするために設計されています。<br>ブラウザから直接アクセスする用途ではありません。",
  gettingStartedTitle: "はじめに",
  gettingStartedText: "Backlog CLI をインストールして、以下を実行してください:",
  githubLink: "GitHub リポジトリ",
  apiDiscoveryLink: "API ディスカバリー",
  healthCheckLink: "ヘルスチェック",
};

/**
 * Generate landing page HTML with the given content.
 */
function generateLandingPageHtml(content: LandingPageContent): string {
  return `<!DOCTYPE html>
<html lang="${content.lang}">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>${content.title}</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
      color: #e8e8e8;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 20px;
    }
    .container {
      max-width: 600px;
      text-align: center;
    }
    .logo {
      font-size: 3rem;
      margin-bottom: 1rem;
    }
    h1 {
      font-size: 1.8rem;
      font-weight: 600;
      margin-bottom: 1.5rem;
      color: #fff;
    }
    .description {
      font-size: 1rem;
      line-height: 1.6;
      color: #b0b0b0;
      margin-bottom: 2rem;
    }
    .info-box {
      background: rgba(255, 255, 255, 0.05);
      border: 1px solid rgba(255, 255, 255, 0.1);
      border-radius: 12px;
      padding: 1.5rem;
      margin-bottom: 2rem;
    }
    .info-box h2 {
      font-size: 1rem;
      color: #42b883;
      margin-bottom: 0.75rem;
    }
    .info-box p {
      font-size: 0.9rem;
      color: #a0a0a0;
    }
    code {
      background: rgba(66, 184, 131, 0.15);
      color: #42b883;
      padding: 0.2rem 0.5rem;
      border-radius: 4px;
      font-family: "SF Mono", Monaco, "Cascadia Code", monospace;
      font-size: 0.85rem;
    }
    .links {
      display: flex;
      gap: 1rem;
      justify-content: center;
      flex-wrap: wrap;
    }
    .links a {
      color: #42b883;
      text-decoration: none;
      font-size: 0.9rem;
      padding: 0.5rem 1rem;
      border: 1px solid rgba(66, 184, 131, 0.3);
      border-radius: 6px;
      transition: all 0.2s;
    }
    .links a:hover {
      background: rgba(66, 184, 131, 0.1);
      border-color: #42b883;
    }
    .footer {
      margin-top: 3rem;
      font-size: 0.8rem;
      color: #666;
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="logo">🔐</div>
    <h1>${content.heading}</h1>
    <p class="description">
      ${content.description}
    </p>
    <div class="info-box">
      <h2>${content.howToUseTitle}</h2>
      <p>
        ${content.howToUseText}
      </p>
    </div>
    <div class="info-box">
      <h2>${content.gettingStartedTitle}</h2>
      <p>
        ${content.gettingStartedText} <code>backlog auth login</code>
      </p>
    </div>
    <div class="links">
      <a href="https://github.com/yacchi/backlog-cli" target="_blank" rel="noopener">${content.githubLink}</a>
      <a href="/.well-known/backlog-oauth-relay">${content.apiDiscoveryLink}</a>
      <a href="/health">${content.healthCheckLink}</a>
    </div>
    <p class="footer">
      Backlog CLI OAuth Relay Server
    </p>
  </div>
</body>
</html>`;
}

/**
 * Detect preferred language from Accept-Language header.
 * Returns true if Japanese is preferred.
 */
function preferJapanese(acceptLanguage: string | undefined): boolean {
  if (!acceptLanguage) return false;

  // Parse Accept-Language header and check if Japanese has higher priority
  const languages = acceptLanguage.split(",").map((lang) => {
    const [code, qValue] = lang.trim().split(";q=");
    return {
      code: code.toLowerCase().trim(),
      q: qValue ? parseFloat(qValue) : 1.0,
    };
  });

  // Sort by quality value (descending)
  languages.sort((a, b) => b.q - a.q);

  // Check if any Japanese variant appears before English
  for (const { code } of languages) {
    if (code.startsWith("ja")) return true;
    if (code.startsWith("en")) return false;
  }

  return false;
}

/**
 * Create well-known handlers with the given configuration.
 */
export function createWellKnownHandlers(config: RelayConfig): Hono {
  const app = new Hono();

  /**
   * GET / - Landing page for direct browser access.
   * Language is determined by Accept-Language header.
   */
  app.get("/", (c) => {
    const acceptLanguage = c.req.header("Accept-Language");
    const content = preferJapanese(acceptLanguage) ? CONTENT_JA : CONTENT_EN;

    c.header("Cache-Control", "public, max-age=3600");
    c.header("Vary", "Accept-Language");
    return c.html(generateLandingPageHtml(content));
  });

  /**
   * GET /.well-known/backlog-oauth-relay - Server discovery endpoint.
   */
  app.get("/.well-known/backlog-oauth-relay", (c) => {
    const response: WellKnownResponse = {
      version: "1.0",
      capabilities: ["oauth2", "token-exchange", "token-refresh"],
      supported_domains: config.backlog_apps.map((app) => app.domain),
    };

    // Set cache headers for discovery endpoint
    c.header("Cache-Control", "public, max-age=3600");

    return c.json(response);
  });

  /**
   * GET /health - Health check endpoint.
   */
  app.get("/health", (c) => {
    return c.json({ status: "ok" });
  });

  return app;
}
