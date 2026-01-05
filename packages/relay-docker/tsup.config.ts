import { defineConfig } from "tsup";

export default defineConfig({
  entry: ["src/index.ts"],
  format: ["esm"],
  clean: true,
  // Bundle all dependencies into a single file
  noExternal: [/.*/],
  // Generate sourcemap for debugging
  sourcemap: true,
  // Target Node.js
  platform: "node",
  target: "node22",
  // Enable shims for __dirname in ESM
  shims: true,
});
