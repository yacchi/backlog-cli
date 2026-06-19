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
// tag including prereleases when image.prerelease is true (a newer stable
// still wins).
const imageSource = config.image?.source ?? DEFAULT_IMAGE_SOURCE;
const imageTag =
  process.env.IMAGE_TAG ??
  config.image?.tag ??
  (await resolveLatestImageTag(imageSource, {
    prerelease: config.image?.prerelease,
  }));

const tagSource = process.env.IMAGE_TAG
  ? "env IMAGE_TAG"
  : config.image?.tag
    ? "pinned"
    : config.image?.prerelease
      ? "latest incl. prerelease"
      : "latest stable";

// eslint-disable-next-line no-console
console.log(`Using container image ${imageSource}:${imageTag} (${tagSource})`);

new RelayStack(app, "BacklogRelay", {
  config: { ...config, image: { ...config.image, source: imageSource, tag: imageTag } },
  env: {
    account: process.env.CDK_DEFAULT_ACCOUNT,
    region: process.env.CDK_DEFAULT_REGION ?? "ap-northeast-1",
  },
  description: "Backlog CLI Relay Server (OAuth relay + MCP server)",
});
