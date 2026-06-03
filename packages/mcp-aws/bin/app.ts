#!/usr/bin/env node
import "source-map-support/register";
import * as cdk from "aws-cdk-lib";
import { McpStack } from "../lib/mcp-stack.js";
import { config } from "../config.js";

const app = new cdk.App();

new McpStack(app, "BacklogMcpServer", {
    config,
    env: {
        account: process.env.CDK_DEFAULT_ACCOUNT,
        region: process.env.CDK_DEFAULT_REGION ?? "ap-northeast-1",
    },
    description: "Backlog MCP Server (Remote, Streamable HTTP)",
});
