import * as cdk from 'aws-cdk-lib';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import {LoggingFormat} from 'aws-cdk-lib/aws-lambda';
import * as ssm from 'aws-cdk-lib/aws-ssm';
import * as logs from 'aws-cdk-lib/aws-logs';
import {Construct} from 'constructs';
import * as path from 'path';
import {execSync} from 'child_process';
import {isInlineConfig, isParameterStoreConfig, RelayConfig,} from './types';

export interface RelayStackProps extends cdk.StackProps {
  config: RelayConfig;
}

export class RelayStack extends cdk.Stack {
  public readonly functionUrl: lambda.FunctionUrl;

  constructor(scope: Construct, id: string, props: RelayStackProps) {
    super(scope, id, props);

    const { config } = props;

    // Lambda Function URL のホストパターン（リージョンを含む）
    const lambdaFunctionUrlPattern = `*.lambda-url.${this.region}.on.aws`;

    // Lambda 用の環境変数を構築
    const environment: Record<string, string> = {
      // Lambda では $HOME が未設定のため、設定ファイルパス解決用に設定
      HOME: '/tmp',
      BACKLOG_AUDIT_OUTPUT: 'stdout',
      // Lambda Function URL のホストパターンをデフォルトで設定
      BACKLOG_ALLOWED_HOST_PATTERNS: lambdaFunctionUrlPattern,
    };

    // Parameter Store を使う場合
    if (isParameterStoreConfig(config)) {
      // パラメーターを作成する場合
      if (config.createParameter && config.parameterValue) {
        new ssm.StringParameter(this, 'ConfigParameter', {
          parameterName: config.parameterName,
          stringValue: JSON.stringify(config.parameterValue),
          description: 'Backlog OAuth Relay Server configuration',
          tier: ssm.ParameterTier.STANDARD,
        });
      }

      environment['BACKLOG_CONFIG_PARAMETER'] = config.parameterName;
    }

    // インライン設定の場合
    if (isInlineConfig(config)) {
      environment['BACKLOG_COOKIE_SECRET'] = config.cookieSecret;

      // Note: ローカル .env ファイルと同じ形式（大文字ドメイン名）を使用
      if (config.backlog.jp) {
        environment['BACKLOG_CLIENT_ID_JP'] = config.backlog.jp.clientId;
        environment['BACKLOG_CLIENT_SECRET_JP'] = config.backlog.jp.clientSecret;
      }

      if (config.backlog.com) {
        environment['BACKLOG_CLIENT_ID_COM'] = config.backlog.com.clientId;
        environment['BACKLOG_CLIENT_SECRET_COM'] = config.backlog.com.clientSecret;
      }

      if (config.allowedSpaces?.length) {
        environment['BACKLOG_ALLOWED_SPACES'] = config.allowedSpaces.join(',');
      }

      if (config.allowedProjects?.length) {
        environment['BACKLOG_ALLOWED_PROJECTS'] = config.allowedProjects.join(',');
      }

      if (config.allowedHostPatterns) {
        // ユーザー指定のパターンをデフォルトに追加
        environment['BACKLOG_ALLOWED_HOST_PATTERNS'] =
          `${lambdaFunctionUrlPattern};${config.allowedHostPatterns}`;
      }

      if (config.audit !== undefined) {
        environment['BACKLOG_AUDIT_ENABLED'] = config.audit.enabled ? 'true' : 'false';
      }
    }

    // Go バイナリをビルド
    const lambdaDir = path.join(__dirname, '..', 'lambda');
    const outputPath = path.join(lambdaDir, 'bootstrap');

    // GOWORK=off でビルド（go.work がある場合の対応）
    execSync(
      `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 GOWORK=off go build -ldflags="-s -w" -o ${outputPath} .`,
      {
        cwd: lambdaDir,
        stdio: 'inherit',
      }
    );

    // Lambda 関数
    const fn = new lambda.Function(this, 'RelayFunction', {
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      code: lambda.Code.fromAsset(lambdaDir, {
        exclude: ['*.go', 'go.mod', 'go.sum'],
      }),
      memorySize: 128,
      loggingFormat: LoggingFormat.JSON,
      timeout: cdk.Duration.seconds(30),
      environment,
      logRetention: logs.RetentionDays.ONE_MONTH,
      description: 'Backlog CLI OAuth Relay Server',
    });

    // Parameter Store から読み込む場合、読み取り権限を付与
    if (isParameterStoreConfig(config)) {
      fn.addToRolePolicy(new cdk.aws_iam.PolicyStatement({
        actions: ['ssm:GetParameter'],
        resources: [
          `arn:aws:ssm:${this.region}:${this.account}:parameter${config.parameterName}`,
        ],
      }));
    }

    // Function URL を作成
    this.functionUrl = fn.addFunctionUrl({
      authType: lambda.FunctionUrlAuthType.NONE,
      cors: {
        allowedOrigins: ['*'],
        allowedMethods: [lambda.HttpMethod.GET, lambda.HttpMethod.POST],
        allowedHeaders: ['Content-Type'],
      },
    });

    // Function URL を環境変数に設定（コールバック URL 構築用）
    // Note: 循環参照を避けるため、Lambda は起動時に自身の URL を取得する必要がある
    // または、カスタムドメインを使用する場合は事前に設定

    // Outputs
    new cdk.CfnOutput(this, 'FunctionUrl', {
      value: this.functionUrl.url,
      description: 'Relay server URL',
    });

    new cdk.CfnOutput(this, 'CallbackUrl', {
      value: `${this.functionUrl.url}auth/callback`,
      description: 'OAuth callback URL (register this in Backlog)',
    });
  }
}
