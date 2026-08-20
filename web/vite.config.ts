import { defineConfig } from "vitest/config";

export default defineConfig({
  build: {
    emptyOutDir: true,
    outDir: "../internal/webassets/dist",
    target: "es2022",
  },
  test: {
    environment: "jsdom",
    globals: true,
    restoreMocks: true,
  },
});
