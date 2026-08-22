import assert from "node:assert/strict";
import test from "node:test";

import { APIError, createAPI, isUnauthorized } from "./api.js";

test("authenticated 401 responses are identifiable as logout conditions", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => new Response(JSON.stringify({ error: "成员访问令牌无效" }), {
    status: 401,
    headers: { "Content-Type": "application/json" },
  });

  try {
    const api = createAPI({ apiBase: "https://family-api.example", sessionToken: "expired", adminToken: "", language: "en" });
    await assert.rejects(api("/api/v1/me"), error => {
      assert.ok(error instanceof APIError);
      assert.equal(error.status, 401);
      assert.equal(isUnauthorized(error), true);
      return true;
    });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("other failures remain connection errors instead of logout conditions", () => {
  assert.equal(isUnauthorized(new APIError("Request failed (503)", 503)), false);
  assert.equal(isUnauthorized(new Error("Request failed (401)")), false);
});
