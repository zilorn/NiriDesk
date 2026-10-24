export class ApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(code: string, status = 0) {
    super(code);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
  }
}

export interface ApiOptions {
  method?: "GET" | "POST";
  body?: unknown;
}

export async function api<T = void>(path: string, options: ApiOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  const request: RequestInit = {
    method: options.method ?? "GET",
    credentials: "same-origin",
    headers,
  };

  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
    request.body = JSON.stringify(options.body);
  }

  let response: Response;
  try {
    response = await fetch(path, request);
  } catch {
    throw new ApiError("network_unreachable");
  }

  let payload: unknown = null;
  if (response.status !== 204) {
    const contentType = response.headers.get("Content-Type") ?? "";
    if (contentType.includes("application/json")) {
      payload = await response.json().catch(() => null);
    }
  }

  if (!response.ok) {
    const code =
      typeof payload === "object" && payload !== null && "error" in payload
        ? String((payload as { error: unknown }).error)
        : `http_${response.status}`;
    throw new ApiError(code, response.status);
  }

  return payload as T;
}

const messages: Record<string, string> = {
  invalid_password: "密码不正确，请重试。",
  too_many_attempts: "尝试次数过多，请稍后再试。",
  invalid_request: "请求格式无效。",
  authentication_required: "会话已过期，请重新登录。",
  vnc_unavailable: "桌面服务尚未就绪，请稍后重试。",
  session_limit_reached: "当前连接数已达到上限。",
  network_unreachable: "无法连接到 NiriDesk 服务。",
  output_switch_failed: "切换显示器失败。",
  output_refresh_failed: "无法刷新显示器列表。",
  invalid_output: "无效的显示器输出。",
};

export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return messages[error.code] ?? "操作失败，请稍后重试。";
  }
  return "操作失败，请稍后重试。";
}

export function isUnauthorized(error: unknown): boolean {
  return error instanceof ApiError && error.status === 401;
}
