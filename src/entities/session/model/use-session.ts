import { useQuery } from "@tanstack/react-query";

import { getCurrentUser } from "../api/session-api";

export const sessionQueryKey = ["session"] as const;

export function useSession() {
  return useQuery({
    queryKey: sessionQueryKey,
    queryFn: getCurrentUser,
    retry: false,
  });
}
