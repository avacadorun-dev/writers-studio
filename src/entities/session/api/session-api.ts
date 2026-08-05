import { apiRequest } from "~shared/api";

import type { User } from "../model/types";

export interface AuthInput {
  email: string;
  password: string;
}

interface SessionResponse {
  user: User;
}

export function getCurrentUser(): Promise<SessionResponse> {
  return apiRequest<SessionResponse>("/api/auth/me");
}

export function login(input: AuthInput): Promise<SessionResponse> {
  return apiRequest<SessionResponse>("/api/auth/login", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function register(input: AuthInput): Promise<SessionResponse> {
  return apiRequest<SessionResponse>("/api/auth/register", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function logout(): Promise<void> {
  return apiRequest<void>("/api/auth/logout", { method: "POST" });
}
