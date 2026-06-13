/**
 * @yacchi/backlog-relay-aws-cdk
 *
 * AWS CDK construct library to deploy the Backlog OAuth Relay + MCP server as a
 * Lambda container behind CloudFront. The runtime ships as a published container
 * image (ghcr.io/yacchi/backlog-relay); this library wires up the AWS resources.
 *
 * Public API:
 * - {@link RelayStack} — the deployable stack/construct.
 * - config types ({@link RelayConfig} etc.) for typing the stack props.
 * - {@link resolveLatestImageTag} — resolve the image tag from the registry.
 */

// Stack + defaults
export { RelayStack, DEFAULT_IMAGE_SOURCE, DEFAULT_CACHE_CONFIG } from "./relay-stack.js";
export type { RelayStackProps } from "./relay-stack.js";

// Image tag resolution
export {
  resolveLatestImageTag,
  fetchImageTags,
  parseImageRef,
} from "./resolve-image-tag.js";
export type { ResolveImageTagOptions } from "./resolve-image-tag.js";

// Configuration types (relay-core types are inlined at build time).
export * from "./types.js";
