import * as cdk from "aws-cdk-lib";
import * as lambda from "aws-cdk-lib/aws-lambda";
import { LoggingFormat } from "aws-cdk-lib/aws-lambda";
import { NodejsFunction, OutputFormat } from "aws-cdk-lib/aws-lambda-nodejs";
import * as ssm from "aws-cdk-lib/aws-ssm";
import * as logs from "aws-cdk-lib/aws-logs";
import * as cloudfront from "aws-cdk-lib/aws-cloudfront";
import * as origins from "aws-cdk-lib/aws-cloudfront-origins";
import * as acm from "aws-cdk-lib/aws-certificatemanager";
import * as route53 from "aws-cdk-lib/aws-route53";
import * as route53Targets from "aws-cdk-lib/aws-route53-targets";
import { Construct } from "constructs";
import * as path from "path";
import { execSync } from "child_process";
import {
  CloudFrontCacheConfig,
  RelayConfig,
  serializeParameterValue,
} from "./types.js";
import type { RelayConfig as CoreRelayConfig } from "@backlog-cli/relay-core";

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

export interface RelayStackProps extends cdk.StackProps {
  config: RelayConfig;
}

export class RelayStack extends cdk.Stack {
  public readonly functionUrl: lambda.FunctionUrl;
  public readonly distribution?: cloudfront.Distribution;
  private configParameter: ssm.IParameter;
  private readonly config: RelayConfig;
  private lambdaFunction: lambda.Function;

  constructor(scope: Construct, id: string, props: RelayStackProps) {
    super(scope, id, props);

    this.config = props.config;
    const cloudFrontEnabled = this.config.cloudFront?.enabled ?? false;

    // SSM Parameter Store に設定を保存
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
   * SSM Parameter Store に設定を保存
   */
  private createConfigParameter(): ssm.StringParameter {
    const parameterName = this.config.parameterName;

    // 許可するホストパターンを構築
    const patterns: string[] = [];

    // Lambda Function URL パターン（常に追加）
    patterns.push(`*.lambda-url.${this.region}.on.aws`);

    // CloudFront パターン（CloudFront 有効時）
    if (this.config.cloudFront?.enabled) {
      patterns.push("*.cloudfront.net");

      // カスタムドメイン（設定されている場合）
      if (this.config.cloudFront.domainName) {
        patterns.push(this.config.cloudFront.domainName);
      }
    }

    // JWKS オブジェクトを文字列化し、ホストパターンを追加
    const baseValue = this.config.parameterValue ?? {
      server: { port: 8080 },
      backlog_apps: [],
    };
    const serializedValue = serializeParameterValue(baseValue);

    // Add allowed host patterns
    const valueWithPatterns = addAllowedHostPatterns(
      serializedValue as CoreRelayConfig,
      patterns
    );
    const parameterValue = JSON.stringify(valueWithPatterns);

    return new ssm.StringParameter(this, "ConfigParameter", {
      parameterName,
      stringValue: parameterValue,
      description: "Backlog OAuth Relay Server configuration",
      tier: ssm.ParameterTier.STANDARD,
    });
  }

  /**
   * Lambda 関数を作成
   * NodejsFunctionで自動的にesbuildバンドルを行う
   */
  private createLambdaFunction(): lambda.Function {
    // Web assets source directory (relative to monorepo root)
    const webDistDir = path.resolve(import.meta.dirname, "..", "..", "web", "dist");

    const fn = new NodejsFunction(this, "RelayFunction", {
      entry: path.join(import.meta.dirname, "handler.ts"),
      handler: "handler",
      runtime: lambda.Runtime.NODEJS_20_X,
      architecture: lambda.Architecture.ARM_64,
      memorySize: 512,
      loggingFormat: LoggingFormat.JSON,
      timeout: cdk.Duration.seconds(10),
      environment: {
        HOME: "/tmp",
        CONFIG_PARAMETER_NAME: this.configParameter.parameterName,
      },
      description: "Backlog CLI OAuth Relay Server (TypeScript)",
      bundling: {
        format: OutputFormat.ESM,
        target: "node20",
        externalModules: ["@aws-sdk/*"],
        mainFields: ["module", "main"],
        banner:
          "import { createRequire } from 'module';const require = createRequire(import.meta.url);",
        commandHooks: {
          beforeBundling(): string[] {
            return [];
          },
          beforeInstall(): string[] {
            return [];
          },
          afterBundling(_inputDir: string, outputDir: string): string[] {
            // Copy web assets to the Lambda bundle
            // The portal-assets.ts expects assets at web-dist/ relative to handler
            return [
              `if [ -d "${webDistDir}" ]; then cp -r "${webDistDir}" "${outputDir}/web-dist"; fi`,
            ];
          },
        },
      },
    });

    new logs.LogGroup(this, "RelayFunctionLogGroup", {
      logGroupName: `/aws/lambda/${fn.functionName}`,
      retention: logs.RetentionDays.ONE_MONTH,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    // Parameter Store の読み取り権限を付与
    this.configParameter.grantRead(fn);

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
    const cloudFrontConfig = this.config.cloudFront;
    if (!cloudFrontConfig?.enabled) {
      return;
    }
    const { domainName, certificateArn, hostedZoneId } = cloudFrontConfig;
    const useCustomDomain = domainName != null && certificateArn != null;

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

    // 共通のビヘイビア設定（最大限の機能を含む）
    // - X-Original-Host ヘッダーを注入・転送
    // - Lambda@Edge でPOST/PUTのボディハッシュを計算
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
      ...(useCustomDomain && {
        domainNames: [domainName!],
        certificate: acm.Certificate.fromCertificateArn(
          this,
          "Certificate",
          certificateArn!,
        ),
      }),
      comment: useCustomDomain
        ? `Backlog CLI OAuth Relay Server (${domainName})`
        : "Backlog CLI OAuth Relay Server",
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
    if (useCustomDomain && hostedZoneId) {
      this.createDnsRecords(distribution, domainName!, hostedZoneId);
    }

    // CloudFront 用 Outputs
    this.createCloudFrontOutputs(
      distribution,
      useCustomDomain ? domainName : undefined,
    );

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
   * CloudFront Function: x-original-host ヘッダーを追加
   * Viewer Request で Host ヘッダーを x-original-host にコピー
   * 注意: X-Forwarded-Host は予約ヘッダーのため使用不可
   */
  private createForwardHostFunction(): cloudfront.Function {
    return new cloudfront.Function(this, "ForwardHostFunction", {
      functionName: `${this.stackName}-forward-host`,
      comment: "Add x-original-host header from viewer Host",
      code: cloudfront.FunctionCode.fromInline(
        `
function handler(event) {
  var request = event.request;
  var host = request.headers.host ? request.headers.host.value : '';
  request.headers['x-original-host'] = { value: host };
  return request;
}
      `.trim(),
      ),
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
