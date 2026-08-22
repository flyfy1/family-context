export class APIError extends Error {
  constructor(message, status) {
    super(message);
    this.name = "APIError";
    this.status = status;
  }
}

export function isUnauthorized(error) {
  return error instanceof APIError && error.status === 401;
}

export function logoutOnUnauthorized(error, logout) {
  if (!isUnauthorized(error)) return false;
  logout();
  return true;
}

export function createAPI(config) {
  return async (path, options = {}) => {
    const { admin = false, ...fetchOptions } = options;
    const headers = new Headers(fetchOptions.headers || {});
    if (admin) headers.set("X-Admin-Token", config.adminToken || ""); else headers.set("Authorization", `Bearer ${config.sessionToken || ""}`);
    if (fetchOptions.body && !(fetchOptions.body instanceof FormData)) headers.set("Content-Type", "application/json");
    const response = await fetch(`${config.apiBase}${path}`, { ...fetchOptions, headers });
    if (options.raw) {
      if (!response.ok) throw new APIError(config.language === "zh" ? "暂时无法读取文件" : "Unable to read the file", response.status);
      return response.blob();
    }
    if (response.status === 204) return null;
    const body = await response.json().catch(() => ({}));
    if (!response.ok) {
      const message = config.language === "zh" ? (body.error || `请求失败（${response.status}）`) : `Request failed (${response.status})`;
      throw new APIError(message, response.status);
    }
    return body;
  };
}
