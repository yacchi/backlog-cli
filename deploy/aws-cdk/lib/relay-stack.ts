import * as cdk from "aws-cdk-lib";
import * as lambda from "aws-cdk-lib/aws-lambda";
import { LoggingFormat } from "aws-cdk-lib/aws-lambda";
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
import { ParameterStoreValue, RelayConfig } from "./types";

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
    const parameterValue = JSON.stringify(
      withLambdaFunctionUrlPattern(
        this.config.parameterValue ?? {},
        this.region,
      ),
    );

    return new ssm.StringParameter(this, "ConfigParameter", {
      parameterName,
      stringValue: parameterValue,
      description: "Backlog OAuth Relay Server configuration",
      tier: ssm.ParameterTier.STANDARD,
    });
  }

  /**
   * Lambda 関数を作成
   */
  private createLambdaFunction(): lambda.Function {
    // Go バイナリをビルド
    const lambdaDir = path.join(__dirname, "..", "lambda");
    const outputPath = path.join(lambdaDir, "bootstrap");

    execSync(
      `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o ${outputPath} .`,
      { cwd: lambdaDir, stdio: "inherit" },
    );

    // Lambda 関数
    const fn = new lambda.Function(this, "RelayFunction", {
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: "bootstrap",
      code: lambda.Code.fromAsset(lambdaDir, {
        exclude: ["*.go", "go.mod", "go.sum"],
      }),
      memorySize: 512,
      loggingFormat: LoggingFormat.JSON,
      timeout: cdk.Duration.seconds(10),
      environment: {
        HOME: "/tmp",
        BACKLOG_CONFIG_PARAMETER: this.configParameter.parameterName,
      },
      description: "Backlog CLI OAuth Relay Server",
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
   */
  private createFunctionUrl(useIamAuth: boolean): lambda.FunctionUrl {
    return this.lambdaFunction.addFunctionUrl({
      authType: useIamAuth
        ? lambda.FunctionUrlAuthType.AWS_IAM
        : lambda.FunctionUrlAuthType.NONE,
      cors: {
        allowedOrigins: ["*"],
        allowedMethods: [lambda.HttpMethod.GET, lambda.HttpMethod.POST],
        allowedHeaders: ["Content-Type"],
      },
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

    // オリジン: Lambda Function URL
    const origin = this.createOrigin();

    // キャッシュポリシー
    const { assetsCachePolicy, dynamicCachePolicy } =
      this.createCachePolicies();

    // オリジンリクエストポリシー
    const originRequestPolicy = this.createOriginRequestPolicy();

    // CloudFront Functions
    const forwardHostFunction = this.createForwardHostFunction();

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
      defaultBehavior: {
        origin,
        viewerProtocolPolicy: cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
        allowedMethods: cloudfront.AllowedMethods.ALLOW_ALL,
        cachePolicy: dynamicCachePolicy,
        originRequestPolicy,
        functionAssociations: [
          {
            function: forwardHostFunction,
            eventType: cloudfront.FunctionEventType.VIEWER_REQUEST,
          },
        ],
      },
      additionalBehaviors: {
        "/assets/*": {
          origin,
          viewerProtocolPolicy:
            cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
          allowedMethods: cloudfront.AllowedMethods.ALLOW_GET_HEAD,
          cachePolicy: assetsCachePolicy,
          compress: true,
        },
      },
      priceClass: cloudfront.PriceClass.PRICE_CLASS_100,
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
  private createOrigin(): origins.FunctionUrlOrigin {
    return new origins.FunctionUrlOrigin(this.functionUrl);
  }

  /**
   * キャッシュポリシーを作成
   */
  private createCachePolicies(): {
    assetsCachePolicy: cloudfront.CachePolicy;
    dynamicCachePolicy: cloudfront.CachePolicy;
  } {
    const assetsCachePolicy = new cloudfront.CachePolicy(
      this,
      "AssetsCachePolicy",
      {
        cachePolicyName: `${this.stackName}-assets`,
        comment: "Cache policy for static assets with content hash",
        defaultTtl: cdk.Duration.days(365),
        maxTtl: cdk.Duration.days(365),
        minTtl: cdk.Duration.days(1),
        enableAcceptEncodingGzip: true,
        enableAcceptEncodingBrotli: true,
      },
    );

    const dynamicCachePolicy = new cloudfront.CachePolicy(
      this,
      "DynamicCachePolicy",
      {
        cachePolicyName: `${this.stackName}-dynamic`,
        comment: "No cache policy for dynamic content",
        defaultTtl: cdk.Duration.seconds(0),
        maxTtl: cdk.Duration.seconds(0),
        minTtl: cdk.Duration.seconds(0),
      },
    );

    return { assetsCachePolicy, dynamicCachePolicy };
  }

  /**
   * オリジンリクエストポリシーを作成
   */
  private createOriginRequestPolicy(): cloudfront.OriginRequestPolicy {
    return new cloudfront.OriginRequestPolicy(this, "OriginRequestPolicy", {
      originRequestPolicyName: `${this.stackName}-origin-request`,
      comment: "Forward headers for OAuth flow",
      headerBehavior: cloudfront.OriginRequestHeaderBehavior.allowList(
        "Accept",
        "Accept-Language",
        "Content-Type",
        "Origin",
        "Referer",
        "X-Forwarded-Host",
      ),
      queryStringBehavior: cloudfront.OriginRequestQueryStringBehavior.all(),
      cookieBehavior: cloudfront.OriginRequestCookieBehavior.all(),
    });
  }

  /**
   * X-Forwarded-Host ヘッダーを追加する CloudFront Functions を作成
   */
  private createForwardHostFunction(): cloudfront.Function {
    return new cloudfront.Function(this, "ForwardHostFunction", {
      functionName: `${this.stackName}-forward-host`,
      comment: "Add X-Forwarded-Host header from viewer Host",
      code: cloudfront.FunctionCode.fromInline(
        `
function handler(event) {
  var request = event.request;
  var host = request.headers.host ? request.headers.host.value : '';
  request.headers['x-forwarded-host'] = { value: host };
  return request;
}
      `.trim(),
      ),
      runtime: cloudfront.FunctionRuntime.JS_2_0,
    });
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

function withLambdaFunctionUrlPattern(
  value: ParameterStoreValue | Record<string, unknown>,
  region: string,
): Record<string, unknown> {
  const pattern = `*.lambda-url.${region}.on.aws`;
  const server =
    (value as { server?: { allowed_host_patterns?: string } }).server ?? {};
  const current =
    typeof server.allowed_host_patterns === "string"
      ? server.allowed_host_patterns
      : "";
  const merged = mergeAllowedHostPatterns(current, pattern);
  return {
    ...value,
    server: {
      ...server,
      allowed_host_patterns: merged,
    },
  };
}

function mergeAllowedHostPatterns(current: string, pattern: string): string {
  if (current.trim() === "") {
    return pattern;
  }
  const items = current.split(";").map((item) => item.trim());
  if (items.includes(pattern)) {
    return current;
  }
  return `${current};${pattern}`;
}
