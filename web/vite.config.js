import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";

export default defineConfig({
  base: "./",
  plugins: [react(), { name: "build-version", generateBundle() {
    const commit = process.env.GITHUB_SHA || execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim();
    const uiVersion = JSON.parse(readFileSync(new URL("./node_modules/@integ-life/ui/package.json", import.meta.url), "utf8")).version;
    this.emitFile({ type: "asset", fileName: "build-version.json", source: JSON.stringify({ schema: "integ.life/build-version/v1", repository: "flyfy1/family-context", service: "family.integ.life", commit, uiVersion, builtAt: new Date().toISOString(), dirty: execFileSync("git", ["status", "--porcelain"], { encoding: "utf8" }).trim().length > 0 }) });
  } }],
  build: {
    outDir: "../backend/web",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
      "/media": "http://localhost:8080",
      "/space-files": "http://localhost:8080",
    },
  },
});
