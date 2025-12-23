import React, {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react";
import { create } from "@bufbuild/protobuf";
import { authClient } from "../lib/connect-client";
import { GetConfigRequestSchema } from "../gen/auth/v1/auth_pb";

export type AuthContextData = {
  space: string;
  domain: string;
  relayServer: string;
  spaceHost: string;
  configured: boolean;
  currentAuthType: string;
};

type AuthContextValue = {
  loading: boolean;
  error: string | null;
  data: AuthContextData | null;
  refresh: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

async function fetchAuthContext(): Promise<AuthContextData> {
  const response = await authClient.getConfig(
    create(GetConfigRequestSchema, {}),
  );

  return {
    space: response.space,
    domain: response.domain,
    relayServer: response.relayServer,
    spaceHost: response.spaceHost,
    configured: response.configured,
    currentAuthType: response.currentAuthType,
  };
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<AuthContextData | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const payload = await fetchAuthContext();
      setData(payload);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "不明なエラーが発生しました",
      );
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void refresh();
  }, [refresh]);

  const value = useMemo(
    () => ({
      loading,
      error,
      data,
      refresh,
    }),
    [loading, error, data, refresh],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuthContext() {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("AuthContext is not available");
  }
  return ctx;
}
