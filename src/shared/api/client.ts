import { API_BASE_URL } from "~shared/config";

export interface APIErrorPayload {
  error?: {
    code?: string;
    message?: string;
    currentRevision?: number;
    conflictNodeId?: string;
  };
}

export class APIError extends Error {
  readonly status: number;
  readonly code: string;
  readonly currentRevision?: number;
  readonly conflictNodeId?: string;

  constructor(status: number, payload: APIErrorPayload) {
    super(payload.error?.message ?? "Не удалось выполнить действие");
    this.name = "APIError";
    this.status = status;
    this.code = payload.error?.code ?? "unknown_error";
    this.currentRevision = payload.error?.currentRevision;
    this.conflictNodeId = payload.error?.conflictNodeId;
  }
}

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (
    init.body &&
    !(init.body instanceof Blob) &&
    !(init.body instanceof FormData) &&
    !headers.has("Content-Type")
  ) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    credentials: "include",
    headers,
  });
  if (!response.ok) {
    let payload: APIErrorPayload = {};
    try {
      payload = (await response.json()) as APIErrorPayload;
    } catch {
      payload = {};
    }
    throw new APIError(response.status, payload);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

export function webSocketURL(path: string): string {
  const base = API_BASE_URL
    ? new URL(API_BASE_URL, window.location.origin)
    : new URL(window.location.origin);
  base.protocol = base.protocol === "https:" ? "wss:" : "ws:";
  base.pathname = path;
  base.search = "";
  base.hash = "";
  return base.toString();
}
