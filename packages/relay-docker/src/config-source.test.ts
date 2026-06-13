import { describe, it, expect } from "vitest";
import {
  EnvConfigSource,
  AwsConfigSource,
  selectConfigSource,
  mergeSecrets,
  CONFIG_ENV_VARS,
  type RelaySecrets,
} from "./config-source.js";

describe("EnvConfigSource", () => {
  it("parses inline JSON config", async () => {
    const source = new EnvConfigSource(
      JSON.stringify({ server: { port: 9000 }, foo: "bar" }),
    );
    const raw = await source.loadRawConfig();
    expect(raw).toEqual({ server: { port: 9000 }, foo: "bar" });
  });

  it("throws on invalid JSON", async () => {
    const source = new EnvConfigSource("{ not json");
    await expect(source.loadRawConfig()).rejects.toThrow();
  });
});

describe("mergeSecrets", () => {
  it("injects client_secret into backlog_app", () => {
    const raw: Record<string, unknown> = {
      backlog_app: { client_id: "cid" },
    };
    const secrets: RelaySecrets = { app: { client_secret: "shhh" } };
    mergeSecrets(raw, secrets);
    expect(raw.backlog_app).toEqual({
      client_id: "cid",
      client_secret: "shhh",
    });
  });

  it("uses server.jwks when present", () => {
    const raw: Record<string, unknown> = {};
    mergeSecrets(raw, { server: { jwks: "SERVER_JWKS" } });
    expect(raw.jwks).toBe("SERVER_JWKS");
  });

  it("falls back to first tenant jwks for legacy configs", () => {
    const raw: Record<string, unknown> = {};
    mergeSecrets(raw, {
      tenants: {
        "a.backlog.jp": {},
        "b.backlog.jp": { jwks: "TENANT_JWKS" },
      },
    });
    expect(raw.jwks).toBe("TENANT_JWKS");
  });

  it("merges per-tenant passphrase_hash by name", () => {
    const raw: Record<string, unknown> = {
      tenants: [
        { name: "a.backlog.jp", default_space: "a.backlog.jp" },
        { name: "b.backlog.jp" },
      ],
    };
    mergeSecrets(raw, {
      tenants: {
        "a.backlog.jp": { passphrase_hash: "$2a$hashA" },
        "b.backlog.jp": { passphrase_hash: "$2a$hashB" },
      },
    });
    expect(raw.tenants).toEqual([
      {
        name: "a.backlog.jp",
        default_space: "a.backlog.jp",
        passphrase_hash: "$2a$hashA",
      },
      { name: "b.backlog.jp", passphrase_hash: "$2a$hashB" },
    ]);
  });

  it("does nothing when no matching secrets", () => {
    const raw: Record<string, unknown> = { backlog_app: { client_id: "cid" } };
    mergeSecrets(raw, {});
    expect(raw).toEqual({ backlog_app: { client_id: "cid" } });
  });
});

describe("selectConfigSource", () => {
  it("selects EnvConfigSource when RELAY_CONFIG is set", () => {
    const source = selectConfigSource({
      [CONFIG_ENV_VARS.RELAY_CONFIG]: "{}",
    } as NodeJS.ProcessEnv);
    expect(source).toBeInstanceOf(EnvConfigSource);
  });

  it("selects AwsConfigSource when CONFIG_PARAMETER_NAME is set", () => {
    const source = selectConfigSource({
      [CONFIG_ENV_VARS.CONFIG_PARAMETER_NAME]: "/backlog-relay/config",
    } as NodeJS.ProcessEnv);
    expect(source).toBeInstanceOf(AwsConfigSource);
  });

  it("prefers RELAY_CONFIG over CONFIG_PARAMETER_NAME", () => {
    const source = selectConfigSource({
      [CONFIG_ENV_VARS.RELAY_CONFIG]: "{}",
      [CONFIG_ENV_VARS.CONFIG_PARAMETER_NAME]: "/x",
    } as NodeJS.ProcessEnv);
    expect(source).toBeInstanceOf(EnvConfigSource);
  });

  it("throws when neither is set", () => {
    expect(() => selectConfigSource({} as NodeJS.ProcessEnv)).toThrow();
  });
});
