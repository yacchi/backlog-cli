import React, {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react";

export type AuthContextData = {
  space: string;
  domain: string;
  relayServer: string;
  spaceHost: string;
  configured: boolean;
};

type AuthContextValue = {
  loading: boolean;
  error: string | null;
  data: AuthContextData | null;
  refresh: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

async function fetchAuthContext(): Promise<AuthContextData> {
  const response = await fetch("/auth/config", {
    method: "GET",
    headers: {
      Accept: "application/json",
    },
    credentials: "same-origin",
  });

  if (!response.ok) {
    throw new Error("認証情報の取得に失敗しました");
  }

  return response.json();
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
