import * as cdk from "aws-cdk-lib";
import * as lambda from "aws-cdk-lib/aws-lambda";
import { LoggingFormat } from "aws-cdk-lib/aws-lambda";
import { NodejsFunction, OutputFormat } from "aws-cdk-lib/aws-lambda-nodejs";
import * as ssm from "aws-cdk-lib/aws-ssm";
import * as secretsmanager from "aws-cdk-lib/aws-secretsmanager";
import * as logs from "aws-cdk-lib/aws-logs";
import { Construct } from "constructs";
import * as path from "node:path";
import type { McpStackConfig } from "./types.js";

export interface McpStackProps extends cdk.StackProps {
    config: McpStackConfig;
}

export class McpStack extends cdk.Stack {
    public readonly functionUrl: lambda.FunctionUrl;
    private configParameter: ssm.IParameter;
    private tokenKeySecret: secretsmanager.ISecret;
    private readonly config: McpStackConfig;
    private lambdaFunction: lambda.Function;

    constructor(scope: Construct, id: string, props: McpStackProps) {
        super(scope, id, props);
        this.config = props.config;

        this.configParameter = this.createConfigParameter();
        this.tokenKeySecret = this.createTokenKeySecret();
        this.lambdaFunction = this.createLambdaFunction();
        this.functionUrl = this.createFunctionUrl();
        this.createOutputs();
    }

    private createConfigParameter(): ssm.StringParameter {
        const parameterValue = this.config.parameterValue
            ? JSON.stringify(this.config.parameterValue)
            : undefined;

        return new ssm.StringParameter(this, "ConfigParameter", {
            parameterName: this.config.parameterName,
            stringValue: parameterValue ?? "{}",
            description: "Backlog MCP Server configuration (token keys are in Secrets Manager)",
            tier: ssm.ParameterTier.STANDARD,
        });
    }

    private createTokenKeySecret(): secretsmanager.ISecret {
        return secretsmanager.Secret.fromSecretNameV2(
            this,
            "TokenKeySecret",
            this.config.secretName,
        );
    }

    private createLambdaFunction(): lambda.Function {
        const repoRoot = path.resolve(import.meta.dirname, "..", "..", "..");
        const mcsServerDir = path.resolve(import.meta.dirname, "..", "..", "mcp-server");

        const fn = new NodejsFunction(this, "McpFunction", {
            entry: path.join(import.meta.dirname, "handler.ts"),
            handler: "handler",
            runtime: lambda.Runtime.NODEJS_22_X,
            architecture: lambda.Architecture.ARM_64,
            memorySize: 1024,
            loggingFormat: LoggingFormat.JSON,
            timeout: cdk.Duration.seconds(120),
            environment: {
                HOME: "/tmp",
                CONFIG_PARAMETER_NAME: this.configParameter.parameterName,
                TOKEN_KEY_SECRET_NAME: this.config.secretName,
                BACKLOG_BIN_PATH: "/var/task/bin/backlog",
                SANDBOX_WORKER_PATH: "/var/task/bin/sandbox-worker",
            },
            description: "Backlog MCP Server (Streamable HTTP)",
            bundling: {
                format: OutputFormat.ESM,
                target: "node22",
                commandHooks: {
                    beforeBundling(): string[] {
                        return [];
                    },
                    beforeInstall(): string[] {
                        return [];
                    },
                    afterBundling(_inputDir: string, outputDir: string): string[] {
                        const workerSrc = `${mcsServerDir}/src/sandbox/sandbox-worker.mjs`;
                        const compiledWorker = `${mcsServerDir}/vendor/sandbox-worker`;
                        return [
                            `mkdir -p "${outputDir}/bin"`,
                            // Go CLI binary (linux/arm64)
                            `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -C "${repoRoot}" -o "${outputDir}/bin/backlog" ./cmd/backlog`,
                            `chmod +x "${outputDir}/bin/backlog"`,
                            // Sandbox worker — compile to standalone binary with deno compile
                            // Uses isolated temp dir to avoid node_modules interference from mcp-server
                            `if [ -f "${compiledWorker}" ]; then cp "${compiledWorker}" "${outputDir}/bin/sandbox-worker"; else tmpdir=$(mktemp -d) && cp "${workerSrc}" "$tmpdir/" && DENO_TLS_CA_STORE=system deno compile --target aarch64-unknown-linux-gnu --allow-read --allow-write=/tmp --allow-net=127.0.0.1 --output "${compiledWorker}" "$tmpdir/sandbox-worker.mjs" && rm -rf "$tmpdir" && cp "${compiledWorker}" "${outputDir}/bin/sandbox-worker"; fi`,
                            `chmod +x "${outputDir}/bin/sandbox-worker"`,
                        ];
                    },
                },
            },
        });

        new logs.LogGroup(this, "McpFunctionLogGroup", {
            logGroupName: `/aws/lambda/${fn.functionName}`,
            retention: logs.RetentionDays.ONE_MONTH,
            removalPolicy: cdk.RemovalPolicy.DESTROY,
        });

        this.configParameter.grantRead(fn);
        this.tokenKeySecret.grantRead(fn);

        return fn;
    }

    private createFunctionUrl(): lambda.FunctionUrl {
        return this.lambdaFunction.addFunctionUrl({
            authType: lambda.FunctionUrlAuthType.NONE,
        });
    }

    private createOutputs(): void {
        new cdk.CfnOutput(this, "FunctionUrl", {
            value: this.functionUrl.url,
            description: "MCP Server URL (Lambda Function URL)",
        });

        new cdk.CfnOutput(this, "McpEndpoint", {
            value: `${this.functionUrl.url}mcp`,
            description: "MCP endpoint URL (add to Claude Desktop config)",
        });

        new cdk.CfnOutput(this, "ConfigParameterName", {
            value: this.configParameter.parameterName,
            description: "SSM Parameter Store name for MCP server config",
        });
    }
}
