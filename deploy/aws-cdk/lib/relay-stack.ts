import * as cdk from "aws-cdk-lib";
import * as lambda from "aws-cdk-lib/aws-lambda";
import { LoggingFormat } from "aws-cdk-lib/aws-lambda";
import * as ssm from "aws-cdk-lib/aws-ssm";
import * as logs from "aws-cdk-lib/aws-logs";
import { Construct } from "constructs";
import * as path from "path";
import { execSync } from "child_process";
import { ParameterStoreValue, RelayConfig } from "./types";

export interface RelayStackProps extends cdk.StackProps {
  config: RelayConfig;
}

export class RelayStack extends cdk.Stack {
  public readonly functionUrl: lambda.FunctionUrl;

  constructor(scope: Construct, id: string, props: RelayStackProps) {
    super(scope, id, props);

    const { config } = props;

    // Lambda 用の環境変数を構築
    const environment: Record<string, string> = {
      // Lambda では $HOME が未設定のため、設定ファイルパス解決用に設定
      HOME: "/tmp",
    };

    // Parameter Store を使う場合
    let configParameterName: string | null = null;

    configParameterName = config.parameterName;

    const parameterValue = JSON.stringify(
      withLambdaFunctionUrlPattern(config.parameterValue ?? {}, this.region),
    );

    new ssm.StringParameter(this, "ConfigParameter", {
      parameterName: configParameterName,
      stringValue: parameterValue,
      description: "Backlog OAuth Relay Server configuration",
      tier: ssm.ParameterTier.STANDARD,
    });

    if (configParameterName) {
      environment["BACKLOG_CONFIG_PARAMETER"] = configParameterName;
    }

    // Go バイナリをビルド
    const lambdaDir = path.join(__dirname, "..", "lambda");
    const outputPath = path.join(lambdaDir, "bootstrap");

    // GOWORK=off でビルド（go.work がある場合の対応）
    execSync(
      `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o ${outputPath} .`,
      {
        cwd: lambdaDir,
        stdio: "inherit",
      },
    );

    // Lambda 関数
    const fn = new lambda.Function(this, "RelayFunction", {
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: "bootstrap",
      code: lambda.Code.fromAsset(lambdaDir, {
        exclude: ["*.go", "go.mod", "go.sum"],
      }),
      memorySize: 128,
      loggingFormat: LoggingFormat.JSON,
      timeout: cdk.Duration.seconds(30),
      environment,
      logRetention: logs.RetentionDays.ONE_MONTH,
      description: "Backlog CLI OAuth Relay Server",
    });

    // Parameter Store から読み込む場合、読み取り権限を付与
    fn.addToRolePolicy(
      new cdk.aws_iam.PolicyStatement({
        actions: ["ssm:GetParameter"],
        resources: [
          `arn:aws:ssm:${this.region}:${this.account}:parameter${configParameterName}`,
        ],
      }),
    );

    // Function URL を作成
    this.functionUrl = fn.addFunctionUrl({
      authType: lambda.FunctionUrlAuthType.NONE,
      cors: {
        allowedOrigins: ["*"],
        allowedMethods: [lambda.HttpMethod.GET, lambda.HttpMethod.POST],
        allowedHeaders: ["Content-Type"],
      },
    });

    // Function URL を環境変数に設定（コールバック URL 構築用）
    // Note: 循環参照を避けるため、Lambda は起動時に自身の URL を取得する必要がある
    // または、カスタムドメインを使用する場合は事前に設定

    // Outputs
    new cdk.CfnOutput(this, "FunctionUrl", {
      value: this.functionUrl.url,
      description: "Relay server URL",
    });

    new cdk.CfnOutput(this, "CallbackUrl", {
      value: `${this.functionUrl.url}auth/callback`,
      description: "OAuth callback URL (register this in Backlog)",
    });

    new cdk.CfnOutput(this, "ConfigParameterName", {
      value: configParameterName,
      description: "SSM Parameter Store name for relay server config",
    });
  }
}

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
