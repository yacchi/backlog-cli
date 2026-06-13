#!/usr/bin/env node
import "source-map-support/register";
import * as cdk from "aws-cdk-lib";
import {
  RelayStack,
  DEFAULT_IMAGE_SOURCE,
  resolveLatestImageTag,
} from "@yacchi/backlog-relay-aws-cdk";
import { config } from "../config.js";

const app = new cdk.App();

// Resolve the container image tag before constructing the stack (CDK constructs
// cannot be async). An explicit config.image.tag wins; otherwise the latest
// semver tag is resolved from the registry — stable by default, or the latest
// prerelease when image.prerelease is true.
const imageSource = config.image?.source ?? DEFAULT_IMAGE_SOURCE;
const imageTag =
  config.image?.tag ??
  (await resolveLatestImageTag(imageSource, {
    prerelease: config.image?.prerelease,
  }));

// eslint-disable-next-line no-console
console.log(
  `Using container image ${imageSource}:${imageTag}` +
    (config.image?.tag ? " (pinned)" : config.image?.prerelease ? " (latest prerelease)" : " (latest stable)"),
);

new RelayStack(app, "BacklogRelay", {
  config: { ...config, image: { ...config.image, source: imageSource, tag: imageTag } },
  env: {
    account: process.env.CDK_DEFAULT_ACCOUNT,
    region: process.env.CDK_DEFAULT_REGION ?? "ap-northeast-1",
  },
  description: "Backlog CLI Relay Server (OAuth relay + MCP server)",
});
