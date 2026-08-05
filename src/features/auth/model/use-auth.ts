import { useMutation, useQueryClient } from "@tanstack/react-query";

import { type AuthInput, login, logout, register, sessionQueryKey } from "~entities/session";

export function useLogin() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: AuthInput) => login(input),
    onSuccess: (data) => {
      queryClient.setQueryData(sessionQueryKey, data);
    },
  });
}

export function useRegister() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: AuthInput) => register(input),
    onSuccess: (data) => {
      queryClient.setQueryData(sessionQueryKey, data);
    },
  });
}

export function useLogout() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: logout,
    onSuccess: () => {
      queryClient.clear();
    },
  });
}
