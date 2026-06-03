import * as cdk from "aws-cdk-lib";
import * as lambda from "aws-cdk-lib/aws-lambda";
import { LoggingFormat } from "aws-cdk-lib/aws-lambda";
import { NodejsFunction, OutputFormat } from "aws-cdk-lib/aws-lambda-nodejs";
import * as ssm from "aws-cdk-lib/aws-ssm";
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
    private readonly config: McpStackConfig;
    private lambdaFunction: lambda.Function;

    constructor(scope: Construct, id: string, props: McpStackProps) {
        super(scope, id, props);
        this.config = props.config;

        this.configParameter = this.createConfigParameter();
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
            description: "Backlog MCP Server configuration",
            tier: ssm.ParameterTier.STANDARD,
        });
    }

    private createLambdaFunction(): lambda.Function {
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
                BACKLOG_BIN_PATH: "/var/task/bin/backlog",
                DENO_PATH: "/var/task/bin/deno",
                DENO_DIR: "/var/task/.deno-cache",
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
                    afterBundling(inputDir: string, outputDir: string): string[] {
                        const backlogDir = path.join(inputDir, "..", "..", "backlog");
                        return [
                            `mkdir -p "${outputDir}/bin"`,
                            // Go CLI binary (linux/arm64)
                            `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -C "${backlogDir}" -o "${outputDir}/bin/backlog" ./cmd/backlog`,
                            `chmod +x "${outputDir}/bin/backlog"`,
                            // Deno binary — must be pre-placed at mcp-server/vendor/deno
                            `test -f "${mcsServerDir}/vendor/deno" && cp "${mcsServerDir}/vendor/deno" "${outputDir}/bin/deno" && chmod +x "${outputDir}/bin/deno" || echo "WARN: vendor/deno not found — sandbox disabled"`,
                            // Sandbox worker
                            `cp "${mcsServerDir}/src/sandbox/sandbox-worker.mjs" "${outputDir}/sandbox-worker.mjs"`,
                            // Deno cache (Pyodide WASM, pre-resolved via 'deno cache sandbox-worker.mjs')
                            `test -d "${mcsServerDir}/.deno-cache" && cp -r "${mcsServerDir}/.deno-cache" "${outputDir}/.deno-cache" || true`,
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
