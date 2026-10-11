import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";

const sourceCommit = process.env.GITHUB_SHA || execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim();
const sourceDirty = execFileSync("git", ["status", "--porcelain"], { encoding: "utf8" }).trim().length > 0;

export default defineConfig({
  base: "./",
  plugins: [react(), { name: "build-version", generateBundle() {
    const uiVersion = JSON.parse(readFileSync(new URL("./node_modules/@integ-life/ui/package.json", import.meta.url), "utf8")).version;
    this.emitFile({ type: "asset", fileName: "build-version.json", source: JSON.stringify({ schema: "integ.life/build-version/v1", repository: "flyfy1/family-context", service: "family.integ.life", version: `git-${sourceCommit.slice(0, 12)}`, commit: sourceCommit, uiVersion, builtAt: new Date().toISOString(), dirty: sourceDirty }) });
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
