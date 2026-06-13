import { defineConfig } from "tsup";

// Publishable CDK construct library build.
//
// - Bundles the (type-only) workspace dep @yacchi/backlog-relay-core so its types
//   are inlined into the emitted .d.ts and the package is self-contained — that
//   workspace package does not need to be published.
// - Keeps aws-cdk-lib / constructs / cdk-ecr-deployment EXTERNAL: CDK construct
//   identity requires the consumer's own copies; these are peer/runtime deps.
// - ESM only. The runtime resolves sibling asset files (rotation-handler.ts,
//   ../lambda-edge, ../cloudfront-functions) via import.meta.dirname, which works
//   in ESM. rotation-handler.ts is copied into dist/ so its path resolves.
export default defineConfig({
  entry: ["lib/index.ts"],
  format: ["esm"],
  // resolve: inline the relay-core types into the emitted .d.ts (noExternal only
  // affects the JS bundle, not the dts step) so the package is type-self-contained.
  dts: { resolve: [/@yacchi\/backlog-relay-core/] },
  clean: true,
  sourcemap: true,
  platform: "node",
  target: "node22",
  noExternal: ["@yacchi/backlog-relay-core"],
  external: [
    "aws-cdk-lib",
    "constructs",
    "cdk-ecr-deployment",
    "bcryptjs",
    "semver",
    "esbuild",
  ],
  // NodejsFunction (rotation) resolves its entry as
  // `${import.meta.dirname}/rotation-handler.ts` → dist/rotation-handler.ts.
  onSuccess: "cp lib/rotation-handler.ts dist/rotation-handler.ts",
});
