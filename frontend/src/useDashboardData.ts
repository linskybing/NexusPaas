import { useEffect, useMemo, useState } from "react";

import { createAPIClient } from "./api";
import type { DashboardData } from "./types";

export type DashboardState = {
  data?: DashboardData;
  loading: boolean;
  error?: string;
};

export function useDashboardData(baseURL: string, apiKey: string, refreshKey: number, cookieAuth = false): DashboardState {
  const client = useMemo(() => createAPIClient({ baseURL, apiKey }), [baseURL, apiKey]);
  const [state, setState] = useState<DashboardState>({ loading: false });
  const enabled = cookieAuth || Boolean(apiKey);

  useEffect(() => {
    if (!enabled) {
      setState({ loading: false });
      return;
    }

    const controller = new AbortController();
    setState((current) => ({ data: current.data, loading: true }));

    client
      .dashboard(controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) {
          setState({ data, loading: false });
        }
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setState({ loading: false, error: error instanceof Error ? error.message : "Request failed" });
        }
      });

    return () => controller.abort();
  }, [baseURL, client, enabled, refreshKey]);

  return state;
}
