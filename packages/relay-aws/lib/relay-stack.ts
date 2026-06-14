import * as cdk from "aws-cdk-lib";
import * as lambda from "aws-cdk-lib/aws-lambda";
import { LoggingFormat } from "aws-cdk-lib/aws-lambda";
import { NodejsFunction, OutputFormat } from "aws-cdk-lib/aws-lambda-nodejs";
import * as ssm from "aws-cdk-lib/aws-ssm";
import * as secretsmanager from "aws-cdk-lib/aws-secretsmanager";
import * as ecr from "aws-cdk-lib/aws-ecr";
import { ECRDeployment, DockerImageName } from "cdk-ecr-deployment";
import * as logs from "aws-cdk-lib/aws-logs";
import * as cloudfront from "aws-cdk-lib/aws-cloudfront";
import * as origins from "aws-cdk-lib/aws-cloudfront-origins";
import * as acm from "aws-cdk-lib/aws-certificatemanager";
import * as route53 from "aws-cdk-lib/aws-route53";
import * as route53Targets from "aws-cdk-lib/aws-route53-targets";
import { Construct } from "constructs";
import * as path from "node:path";
import { execSync } from "node:child_process";
import {
  CloudFrontCacheConfig,
  RelayConfig,
  buildSsmParameterValue,
  DEFAULT_PARAMETER_NAME,
  DEFAULT_CLOUDFRONT_ENABLED,
} from "./types.js";
import type { RelayConfig as CoreRelayConfig } from "@yacchi/backlog-relay-core";
import { hashSync } from "bcryptjs";

// ============================================================
// バージョン定数
// ============================================================

// ============================================================
// デフォルト設定値
// ============================================================

/**
 * CloudFront キャッシュのデフォルト設定
 *
 * - assetsMaxAge: 静的アセットは長期キャッシュ（コンテンツハッシュで管理）
 * - apiDefaultTtl: オリジンがCache-Controlを返さない場合のデフォルトTTL
 * - apiMaxTtl: オリジンの max-age が大きくてもこの値でキャップ
 * - apiMinTtl: 0に設定してオリジンの no-cache/no-store を尊重
 */
export const DEFAULT_CACHE_CONFIG: Required<CloudFrontCacheConfig> = {
  assetsMaxAge: 365 * 24 * 60 * 60, // 365日
  apiDefaultTtl: 5 * 60, // 5分（フォールバック用）
  apiMaxTtl: 24 * 60 * 60, // 24時間
  apiMinTtl: 0, // オリジンのヘッダーを尊重
};

/**
 * 統合ランタイムコンテナイメージのデフォルト参照（リポジトリ）。
 * Lambda は同一リージョンの private ECR からしか pull できないため、
 * この公開イメージを CDK が ECR にコピーして DockerImageFunction で使用する。
 * タグは固定バージョンを使う（`latest` は使わない）。未指定時は
 * resolveLatestImageTag でレジストリから最新の semver タグを解決する。
 */
export const DEFAULT_IMAGE_SOURCE = "ghcr.io/yacchi/backlog-relay";

export interface RelayStackProps extends cdk.StackProps {
  config: RelayConfig;
}

export class RelayStack extends cdk.Stack {
  public readonly functionUrl: lambda.FunctionUrl;
  public readonly distribution?: cloudfront.Distribution;
  private configParameter: ssm.IParameter;
  private relaySecretsSecret?: secretsmanager.Secret;
  private readonly config: RelayConfig;
  private lambdaFunction: lambda.Function;

  private get mcpEnabled(): boolean {
    return this.config.mcp != null && this.config.mcp.spaces.length > 0;
  }

  /** SSM パラメータ名（未指定時はデフォルトを使用）。 */
  private get parameterName(): string {
    return this.config.parameterName ?? DEFAULT_PARAMETER_NAME;
  }

  /** CloudFront を有効化するか（未指定時はデフォルトで有効）。 */
  private get cloudFrontEnabled(): boolean {
    return this.config.cloudFront?.enabled ?? DEFAULT_CLOUDFRONT_ENABLED;
  }

  constructor(scope: Construct, id: string, props: RelayStackProps) {
    super(scope, id, props);

    this.config = props.config;
    const cloudFrontEnabled = this.cloudFrontEnabled;

    // Relay secrets (JWKS, passphrase, client_secret) → Secrets Manager
    this.relaySecretsSecret = this.createRelaySecretsSecret();

    // SSM Parameter Store に設定を保存 (secrets are stripped)
    this.configParameter = this.createConfigParameter();

    // Lambda 関数を作成
    this.lambdaFunction = this.createLambdaFunction();

    // Function URL を作成（CloudFront有効時はIAM認証）
    this.functionUrl = this.createFunctionUrl(cloudFrontEnabled);

    // CloudFront ディストリビューションを作成
    this.distribution = this.createCloudFrontDistribution();

    // Outputs を作成
    this.createOutputs(cloudFrontEnabled);
  }

  /**
   * Relay secrets (client_secret, JWKS, passphrase_hash) を Secrets Manager に保存.
   * Auto-generates Ed25519 JWKS and passphrase for tenants that don't provide them.
   * Uses SM rotation schedule with rotateImmediatelyOnUpdate for initialization.
   */
  private createRelaySecretsSecret(): secretsmanager.Secret | undefined {
    const value = this.config.parameterValue;
    if (!value) return undefined;

    const hasApp = !!value.backlog_app;
    const hasTenants = Object.keys(value.tenants ?? {}).length > 0;
    const hasJwks = !!value.jwks;
    if (!hasApp && !hasTenants && !hasJwks) return undefined;

    const secretName = `${this.parameterName}-secrets`;
    const secret = new secretsmanager.Secret(this, "RelaySecretsSecret", {
      secretName,
      generateSecretString: {
        excludePunctuation: true,
        passwordLength: 8,
      },
      description:
        "Backlog relay secrets (client_secret, JWKS, passphrase_hash)",
    });

    const appSecret = value.backlog_app
      ? { client_secret: value.backlog_app.client_secret }
      : undefined;

    // Server-level JWKS (only pass if keys are provided; otherwise auto-generated by rotation)
    const hasKeys = value.jwks?.keys && value.jwks.keys.length > 0;
    const serverJwks = hasKeys ? JSON.stringify(value.jwks) : undefined;

    const tenantConfigs: Record<
      string,
      { passphrase_hash?: string; passphrase_length?: number }
    > = {};
    for (const [name, tenant] of Object.entries(
      value.tenants ?? {},
    )) {
      const config: (typeof tenantConfigs)[string] = {};
      if (tenant.passphrase) {
        config.passphrase_hash = hashSync(tenant.passphrase, 12);
      } else if (tenant.passphrase_hash) {
        config.passphrase_hash = tenant.passphrase_hash;
      }
      if (tenant.passphrase_length) {
        config.passphrase_length = tenant.passphrase_length;
      }
      tenantConfigs[name] = config;
    }

    const rotationLambda = this.createRotationFunction("RelaySecrets", {
      SECRET_TYPE: "relay-secrets",
      APP_SECRET: JSON.stringify(appSecret ?? {}),
      SERVER_JWKS: serverJwks ?? "",
      TENANT_CONFIGS: JSON.stringify(tenantConfigs),
    });

    secret.addRotationSchedule("RelaySecretsRotation", {
      rotationLambda,
      automaticallyAfter: cdk.Duration.days(0),
      rotateImmediatelyOnUpdate: true,
    });

    return secret;
  }


  /**
   * Rotation Lambda factory — shared rotation-handler.ts, dispatched by SECRET_TYPE env var.
   */
  private createRotationFunction(
    id: string,
    environment: Record<string, string>,
  ): lambda.Function {
    return new NodejsFunction(this, `${id}RotationFunction`, {
      entry: path.join(import.meta.dirname, "rotation-handler.ts"),
      handler: "handler",
      runtime: lambda.Runtime.NODEJS_24_X,
      architecture: lambda.Architecture.ARM_64,
      memorySize: 256,
      timeout: cdk.Duration.seconds(60),
      loggingFormat: LoggingFormat.JSON,
      description: `Secrets rotation handler (${id})`,
      environment,
      bundling: {
        format: OutputFormat.ESM,
        target: "node24",
      },
    });
  }

  /**
   * SSM Parameter Store に設定を保存
   * Secrets are stripped — only non-secret config is stored here.
   */
  private createConfigParameter(): ssm.StringParameter {
    const parameterName = this.parameterName;

    // 許可するホストパターンを構築
    const patterns: string[] = [];

    // Lambda Function URL パターン（常に追加）
    patterns.push(`*.lambda-url.${this.region}.on.aws`);

    // CloudFront パターン（CloudFront 有効時）
    if (this.cloudFrontEnabled) {
      patterns.push("*.cloudfront.net");

      // カスタムドメイン（設定されている場合）
      const customDomain = this.config.cloudFront?.customDomain;
      if (customDomain) {
        patterns.push(customDomain.domainName);
      }
    }

    const baseValue = this.config.parameterValue ?? {
      server: { port: 8080 },
      backlog_app: { client_id: "", client_secret: "" },
    };

    // Build SSM value: strips secrets, converts tenants to relay-core array
    const { config: ssmConfig, mcpSpaces, mcpScript, mcpDefaultSpaces, mcpAudit, mcpLogging } = buildSsmParameterValue(baseValue, this.config.mcp);

    // NOTE: MCP's base_url (OAuth issuer) is derived at runtime from the request
    // host (x-original-host / Host). It is intentionally NOT injected here: the
    // Function URL / CloudFront domain are not known at synth without a circular
    // dependency, and runtime derivation covers all cases uniformly. Set
    // server.base_url explicitly only to force a fixed issuer.

    // Add allowed host patterns
    const valueWithPatterns = addAllowedHostPatterns(
      ssmConfig as CoreRelayConfig,
      patterns,
    );

    // Inject MCP config into SSM parameter (handler extracts before relay-core parsing)
    const finalValue: Record<string, unknown> = { ...valueWithPatterns };
    if (mcpSpaces && mcpSpaces.length > 0) {
      finalValue.mcp_spaces = mcpSpaces;
      if (mcpScript) finalValue.mcp_script = mcpScript;
      if (mcpDefaultSpaces && mcpDefaultSpaces.length > 0) finalValue.mcp_default_spaces = mcpDefaultSpaces;
      if (mcpAudit) finalValue.mcp_audit = mcpAudit;
      if (mcpLogging) finalValue.mcp_logging = mcpLogging;
    }

    const parameterValue = JSON.stringify(finalValue);

    return new ssm.StringParameter(this, "ConfigParameter", {
      parameterName,
      stringValue: parameterValue,
      description: "Backlog OAuth Relay Server configuration",
      tier: ssm.ParameterTier.STANDARD,
    });
  }

  /**
   * Lambda 関数を作成（統合コンテナイメージ）。
   *
   * Relay + MCP のロジックは統合ランタイムイメージ（relay-docker）に集約され、
   * Lambda Web Adapter 経由で HTTP サーバーとして動作する。Go CLI / Deno worker /
   * portal / Node バンドルはすべてイメージ内に同梱済みのため、ここでのビルドは不要。
   *
   * Lambda は同一リージョンの private ECR からしか pull できない（GHCR や
   * pull-through cache は不可）。cdk-ecr-deployment(cdklabs 製) が Skopeo Lambda で
   * 公開イメージ（GHCR 等）を専用 ECR へ registry→registry コピーする。デプロイ時に
   * Docker は不要。専用 repo は DESTROY + emptyOnDelete + maxImageCount で
   * クリーンに管理でき（イメージは GHCR から再現可能）、stack 削除で消える。
   */
  private createLambdaFunction(): lambda.Function {
    const mcpEnabled = this.mcpEnabled;

    // Relay secrets の環境変数
    const relaySecretsEnv: Record<string, string> = this.relaySecretsSecret
      ? { RELAY_SECRETS_NAME: this.relaySecretsSecret.secretName }
      : {};

    const imageSource = this.config.image?.source ?? DEFAULT_IMAGE_SOURCE;
    const imageTag = this.config.image?.tag;
    if (!imageTag) {
      // The tag must be resolved before constructing the stack (CDK constructs
      // cannot be async). bin/app.ts resolves it via resolveLatestImageTag.
      throw new Error(
        "image tag is not set. Resolve it with resolveLatestImageTag() before " +
          "constructing RelayStack, or set config.image.tag explicitly. " +
          "(`latest` is intentionally not supported.)",
      );
    }

    // 統合ランタイムイメージ用の専用 private ECR リポジトリ。
    // イメージは GHCR から再現可能なため DESTROY + emptyOnDelete で問題なく、
    // maxImageCount で古いタグを自動的にキャップする。タグは IMMUTABLE で固定
    // （同一タグの上書きを禁止し、デプロイ済みバージョンの同一性を保証）。
    const repository = new ecr.Repository(this, "RelayImageRepo", {
      imageScanOnPush: true,
      imageTagMutability: ecr.TagMutability.IMMUTABLE,
      emptyOnDelete: true,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
      lifecycleRules: [{ maxImageCount: 5 }],
    });

    // 公開イメージ（GHCR 等）をデプロイ時に専用 ECR へ registry→registry コピー
    // （Skopeo を使う prebuilt Lambda の custom resource。Docker 不要）。
    // imageArch はコピー対象アーキテクチャ。コピー Lambda は既定 amd64 だが、
    // 統合イメージは arm64 専用ビルドなので arm64 を明示する（既定 ['amd64'] のままだと
    // arm64-only の image index から amd64 を選べず "no image found for amd64" で失敗する）。
    const imageCopy = new ECRDeployment(this, "RelayImageCopy", {
      src: new DockerImageName(`${imageSource}:${imageTag}`),
      dest: new DockerImageName(`${repository.repositoryUri}:${imageTag}`),
      imageArch: ["arm64"],
    });

    const fn = new lambda.DockerImageFunction(this, "RelayFunction", {
      code: lambda.DockerImageCode.fromEcr(repository, {
        tagOrDigest: imageTag,
      }),
      architecture: lambda.Architecture.ARM_64,
      memorySize: mcpEnabled ? 1024 : 512,
      loggingFormat: LoggingFormat.JSON,
      timeout: mcpEnabled
        ? cdk.Duration.seconds(120)
        : cdk.Duration.seconds(10),
      environment: {
        // AwsConfigSource がこれらから SSM / Secrets Manager を読む
        CONFIG_PARAMETER_NAME: this.configParameter.parameterName,
        // Lambda Web Adapter: /health が 200 を返したらトラフィックを流す
        AWS_LWA_READINESS_CHECK_PATH: "/health",
        DEPLOY_VERSION: "2026-06-13-container",
        ...relaySecretsEnv,
      },
      description: mcpEnabled
        ? "Backlog CLI Relay + MCP Server (container)"
        : "Backlog CLI OAuth Relay Server (container)",
    });

    // イメージが ECR に push されてから関数を作成/更新する
    fn.node.addDependency(imageCopy);

    new logs.LogGroup(this, "RelayFunctionLogGroup", {
      logGroupName: `/aws/lambda/${fn.functionName}`,
      retention: logs.RetentionDays.ONE_MONTH,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    // Parameter Store の読み取り権限を付与
    this.configParameter.grantRead(fn);

    // Secrets Manager の読み取り権限を付与
    if (this.relaySecretsSecret) {
      this.relaySecretsSecret.grantRead(fn);
    }

    return fn;
  }

  /**
   * Function URL を作成
   * CloudFront 有効時は IAM 認証を使用し、直接アクセスを防止
   * CloudFront 経由の場合、CORS は CloudFront 側で処理するため不要
   */
  private createFunctionUrl(useIamAuth: boolean): lambda.FunctionUrl {
    return this.lambdaFunction.addFunctionUrl({
      authType: useIamAuth
        ? lambda.FunctionUrlAuthType.AWS_IAM
        : lambda.FunctionUrlAuthType.NONE,
      // CloudFront 無効時のみ CORS を設定（直接アクセス用）
      ...(!useIamAuth && {
        cors: {
          allowedOrigins: ["*"],
          allowedMethods: [lambda.HttpMethod.GET, lambda.HttpMethod.POST],
          allowedHeaders: ["Content-Type"],
        },
      }),
    });
  }

  /**
   * CloudFront ディストリビューションを作成
   */
  private createCloudFrontDistribution(): cloudfront.Distribution | undefined {
    if (!this.cloudFrontEnabled) {
      return;
    }
    const customDomain = this.config.cloudFront?.customDomain;

    // オリジン: Lambda Function URL (OAC 自動設定)
    const origin = this.createOrigin();

    // キャッシュポリシー
    const { staticAssetsCachePolicy, dynamicContentsCachePolicy } =
      this.createCachePolicies();

    // オリジンリクエストポリシー: Content-* ヘッダーと Cookie を転送
    const originRequestPolicy = this.createOriginRequestPolicy();

    // CloudFront Function: x-original-host ヘッダーを追加 (viewer-request)
    const forwardHostFunction = this.createForwardHostFunction();

    // Lambda@Edge: コンテンツハッシュを計算 (origin-request)
    // OAC + Lambda Function URL で POST/PUT を使うために必要
    const contentHashEdgeFunction = this.createContentHashEdgeFunction();

    // Viewer Response: Lambda Function URL がリネームした
    // WWW-Authenticate ヘッダーを復元（MCP OAuth で必要）
    const restoreAuthHeaderFunction = this.createRestoreAuthHeaderFunction();

    // 共通のビヘイビア設定（最大限の機能を含む）
    // - X-Original-Host ヘッダーを注入・転送
    // - Lambda@Edge でPOST/PUTのボディハッシュを計算
    // - WWW-Authenticate ヘッダーの復元
    // 新しいエンドポイント追加時に設定漏れを防ぐため、
    // デフォルトは全機能有効とし、不要なルートのみ上書きする
    const baseBehavior = {
      origin,
      viewerProtocolPolicy: cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
      originRequestPolicy,
      functionAssociations: [
        {
          function: forwardHostFunction,
          eventType: cloudfront.FunctionEventType.VIEWER_REQUEST,
        },
        {
          function: restoreAuthHeaderFunction,
          eventType: cloudfront.FunctionEventType.VIEWER_RESPONSE,
        },
      ],
      edgeLambdas: [
        {
          functionVersion: contentHashEdgeFunction.currentVersion,
          eventType: cloudfront.LambdaEdgeEventType.ORIGIN_REQUEST,
          includeBody: true,
        },
      ],
    } as const satisfies Partial<cloudfront.BehaviorOptions>;

    // ディストリビューション
    const distribution = new cloudfront.Distribution(this, "Distribution", {
      ...(customDomain && {
        domainNames: [customDomain.domainName],
        certificate: acm.Certificate.fromCertificateArn(
          this,
          "Certificate",
          customDomain.certificateArn,
        ),
      }),
      comment: customDomain
        ? `Backlog CLI Relay Server (${customDomain.domainName})`
        : "Backlog CLI Relay Server",
      // デフォルト: オリジンのCache-Controlヘッダーを尊重
      // アプリケーション側で適切なキャッシュ制御を返す
      defaultBehavior: {
        ...baseBehavior,
        allowedMethods: cloudfront.AllowedMethods.ALLOW_ALL,
        cachePolicy: dynamicContentsCachePolicy,
      },
      additionalBehaviors: {
        // 静的アセット: ホスト情報不要、長期キャッシュ（immutable）
        "/assets/*": {
          origin,
          viewerProtocolPolicy:
            cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
          allowedMethods: cloudfront.AllowedMethods.ALLOW_GET_HEAD,
          cachePolicy: staticAssetsCachePolicy,
          compress: true,
        },
      },
      priceClass: cloudfront.PriceClass.PRICE_CLASS_200,
      httpVersion: cloudfront.HttpVersion.HTTP2_AND_3,
    });

    // Route 53 DNS レコード
    if (customDomain?.hostedZoneId) {
      this.createDnsRecords(
        distribution,
        customDomain.domainName,
        customDomain.hostedZoneId,
      );
    }

    // CloudFront 用 Outputs
    this.createCloudFrontOutputs(distribution, customDomain?.domainName);

    return distribution;
  }

  /**
   * Lambda Function URL をオリジンとして作成（OAC 自動設定）
   */
  private createOrigin(): cloudfront.IOrigin {
    return origins.FunctionUrlOrigin.withOriginAccessControl(this.functionUrl);
  }

  /**
   * Lambda@Edge: POST/PUT リクエストのコンテンツハッシュを計算
   * OAC + Lambda Function URL で POST を使うために必要
   */
  private createContentHashEdgeFunction(): cloudfront.experimental.EdgeFunction {
    // esbuild で TypeScript をバンドル
    const currentDir = import.meta.dirname;
    const lambdaEdgeDir = path.join(currentDir, "..", "lambda-edge");
    const distDir = path.join(lambdaEdgeDir, "dist");

    execSync(
      `npx esbuild index.ts --bundle --platform=node --target=node20 --outfile=dist/index.js`,
      { cwd: lambdaEdgeDir, stdio: "inherit" },
    );

    return new cloudfront.experimental.EdgeFunction(
      this,
      "ContentHashEdgeFunction",
      {
        runtime: lambda.Runtime.NODEJS_20_X,
        handler: "index.handler",
        code: lambda.Code.fromAsset(distDir),
        description: "Calculate content hash for POST/PUT requests (OAC)",
      },
    );
  }

  /**
   * CloudFront Function: x-amzn-remapped-www-authenticate → www-authenticate
   * Lambda Function URL は WWW-Authenticate を自動リネームするため復元が必要
   */
  private createRestoreAuthHeaderFunction(): cloudfront.Function {
    const functionPath = path.join(
      import.meta.dirname,
      "..",
      "cloudfront-functions",
      "restore-auth-header.js",
    );

    return new cloudfront.Function(this, "RestoreAuthHeaderFunction", {
      comment: "Restore WWW-Authenticate header (remapped by Lambda Function URL)",
      code: cloudfront.FunctionCode.fromFile({ filePath: functionPath }),
      runtime: cloudfront.FunctionRuntime.JS_2_0,
    });
  }

  /**
   * CloudFront Function: x-original-host ヘッダーを追加
   * Viewer Request で Host ヘッダーを x-original-host にコピー
   * 注意: X-Forwarded-Host は予約ヘッダーのため使用不可
   */
  private createForwardHostFunction(): cloudfront.Function {
    const functionPath = path.join(
      import.meta.dirname,
      "..",
      "cloudfront-functions",
      "forward-host.js",
    );

    return new cloudfront.Function(this, "ForwardHostFunction", {
      comment: "Add x-original-host header from viewer Host",
      code: cloudfront.FunctionCode.fromFile({ filePath: functionPath }),
      runtime: cloudfront.FunctionRuntime.JS_2_0,
    });
  }

  /**
   * オリジンリクエストポリシーを作成
   * OAC + Lambda Function URL の署名検証に必要なヘッダーを転送
   * 注意:
   * - Lambda@Edge で x-amz-content-sha256 を計算するため、Cookie 転送が可能
   * - X-Original-Host はアプリケーション用（CloudFront Function で設定）
   */
  private createOriginRequestPolicy(): cloudfront.OriginRequestPolicy {
    return new cloudfront.OriginRequestPolicy(this, "OriginRequestPolicy", {
      originRequestPolicyName: `${this.stackName}-origin-request`,
      comment: "Forward headers for OAC signature verification",
      headerBehavior: cloudfront.OriginRequestHeaderBehavior.allowList(
        "Accept",
        "Accept-Language",
        "Content-Type",
        "Origin",
        "Referer",
        "X-Original-Host",
        "X-Original-User-Agent",
        "X-MCP-Authorization",
      ),
      queryStringBehavior: cloudfront.OriginRequestQueryStringBehavior.all(),
      cookieBehavior: cloudfront.OriginRequestCookieBehavior.all(),
    });
  }

  /**
   * キャッシュポリシーを作成
   * 設定値がない場合は DEFAULT_CACHE_CONFIG のデフォルト値を使用
   */
  private createCachePolicies(): {
    staticAssetsCachePolicy: cloudfront.CachePolicy;
    dynamicContentsCachePolicy: cloudfront.CachePolicy;
  } {
    // 設定とデフォルト値をマージ
    const cacheConfig = {
      ...DEFAULT_CACHE_CONFIG,
      ...this.config.cloudFront?.cache,
    };

    // 静的アセット用（長期キャッシュ、immutable）
    const staticAssetsCachePolicy = new cloudfront.CachePolicy(
      this,
      "StaticAssetsCachePolicy",
      {
        cachePolicyName: `${this.stackName}-static`,
        comment: "Cache policy for static assets (immutable, content hash)",
        defaultTtl: cdk.Duration.seconds(cacheConfig.assetsMaxAge),
        maxTtl: cdk.Duration.seconds(cacheConfig.assetsMaxAge),
        minTtl: cdk.Duration.days(1),
        enableAcceptEncodingGzip: true,
        enableAcceptEncodingBrotli: true,
      },
    );

    // 動的コンテンツ用（オリジンのCache-Controlを尊重）
    // minTtl = 0 でオリジンの no-cache/no-store を尊重
    // defaultTtl はオリジンがCache-Controlを返さない場合のフォールバック
    // Accept-Language をキャッシュキーに含めて言語別にキャッシュ
    //
    // queryStringBehavior を all にしているのは、/auth/start?port=...&state=...
    // のように OAuth のセッション固有 query を持つ動的エンドポイントを区別するため。
    // 既定の `none` だと port/state を含まない同一キャッシュキーになり、別ログインの
    // 古い 302 レスポンスが新しいログインに返されてしまう（旧 port への
    // ERR_CONNECTION_REFUSED の原因となる）。
    const dynamicContentsCachePolicy = new cloudfront.CachePolicy(
      this,
      "DynamicContentsCachePolicy",
      {
        cachePolicyName: `${this.stackName}-dynamic`,
        comment: "Cache policy for dynamic contents (respects origin headers)",
        defaultTtl: cdk.Duration.seconds(cacheConfig.apiDefaultTtl),
        maxTtl: cdk.Duration.seconds(cacheConfig.apiMaxTtl),
        minTtl: cdk.Duration.seconds(cacheConfig.apiMinTtl),
        enableAcceptEncodingGzip: true,
        enableAcceptEncodingBrotli: true,
        headerBehavior: cloudfront.CacheHeaderBehavior.allowList(
          "Accept-Language"
        ),
        queryStringBehavior: cloudfront.CacheQueryStringBehavior.all(),
      },
    );

    return { staticAssetsCachePolicy, dynamicContentsCachePolicy };
  }

  /**
   * Route 53 DNS レコードを作成
   */
  private createDnsRecords(
    distribution: cloudfront.Distribution,
    domainName: string,
    hostedZoneId: string,
  ): void {
    const hostedZone = route53.HostedZone.fromHostedZoneAttributes(
      this,
      "HostedZone",
      {
        hostedZoneId,
        zoneName: domainName.split(".").slice(-2).join("."),
      },
    );

    new route53.ARecord(this, "AliasRecord", {
      zone: hostedZone,
      recordName: domainName,
      target: route53.RecordTarget.fromAlias(
        new route53Targets.CloudFrontTarget(distribution),
      ),
    });

    new route53.AaaaRecord(this, "AliasRecordV6", {
      zone: hostedZone,
      recordName: domainName,
      target: route53.RecordTarget.fromAlias(
        new route53Targets.CloudFrontTarget(distribution),
      ),
    });
  }

  /**
   * CloudFront 用の Outputs を作成
   */
  private createCloudFrontOutputs(
    distribution: cloudfront.Distribution,
    customDomainName?: string,
  ): void {
    new cdk.CfnOutput(this, "DistributionId", {
      value: distribution.distributionId,
      description: "CloudFront distribution ID (for cache invalidation)",
    });

    new cdk.CfnOutput(this, "DistributionUrl", {
      value: `https://${distribution.distributionDomainName}`,
      description: "CloudFront distribution URL",
    });

    new cdk.CfnOutput(this, "DistributionCallbackUrl", {
      value: `https://${distribution.distributionDomainName}/auth/callback`,
      description: "OAuth callback URL (register this in Backlog)",
    });

    if (customDomainName) {
      new cdk.CfnOutput(this, "CustomDomainUrl", {
        value: `https://${customDomainName}`,
        description: "Custom domain URL",
      });

      new cdk.CfnOutput(this, "CustomDomainCallbackUrl", {
        value: `https://${customDomainName}/auth/callback`,
        description: "OAuth callback URL for custom domain",
      });
    }
  }

  /**
   * 基本の Outputs を作成
   */
  private createOutputs(cloudFrontEnabled: boolean): void {
    // CloudFront 有効時は Function URL への直接アクセスが不可のため、
    // CloudFront経由のURLのみを案内
    if (!cloudFrontEnabled) {
      new cdk.CfnOutput(this, "FunctionUrl", {
        value: this.functionUrl.url,
        description: "Relay server URL (Lambda Function URL)",
      });

      new cdk.CfnOutput(this, "CallbackUrl", {
        value: `${this.functionUrl.url}auth/callback`,
        description: "OAuth callback URL (register this in Backlog)",
      });
    }

    new cdk.CfnOutput(this, "ConfigParameterName", {
      value: this.configParameter.parameterName,
      description: "SSM Parameter Store name for relay server config",
    });

    if (this.mcpEnabled) {
      const base = cloudFrontEnabled
        ? this.distribution
          ? `https://${this.distribution.distributionDomainName}`
          : undefined
        : this.functionUrl.url.replace(/\/$/, "");
      if (base) {
        new cdk.CfnOutput(this, "McpEndpoint", {
          value: `${base}/mcp`,
          description: "MCP endpoint URL (add to Claude Desktop config)",
        });
      }
    }
  }

}

// ============================================================
// ヘルパー関数
// ============================================================

/**
 * Add allowed_host_patterns to the configuration.
 * Used to add CloudFront and custom domain patterns.
 */
function addAllowedHostPatterns(
  value: CoreRelayConfig,
  patterns: string[],
): CoreRelayConfig {
  const server = value.server ?? { port: 8080 };
  const current = server.allowed_host_patterns ?? "";

  // Merge existing and new patterns
  const existingPatterns = current
    .split(";")
    .map((p: string) => p.trim())
    .filter((p: string) => p !== "");
  const allPatterns = [...new Set([...existingPatterns, ...patterns])];

  return {
    ...value,
    server: {
      ...server,
      allowed_host_patterns: allPatterns.join(";"),
    },
  };
}
