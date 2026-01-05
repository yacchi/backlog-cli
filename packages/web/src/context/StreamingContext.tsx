import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { create } from "@bufbuild/protobuf";
import { authClient } from "../lib/connect-client";
import {
  AuthStatus,
  SubscribeAuthEventsRequestSchema,
} from "../gen/auth/v1/auth_pb";

// Status型をexport（他コンポーネントで再利用可能）
export type StreamStatus =
  | "connecting"
  | "connected"
  | "success"
  | "error"
  | "closed";

type StreamingContextValue = {
  status: StreamStatus;
  error: string | null;
  disconnect: () => void;
};

const StreamingContext = createContext<StreamingContextValue | null>(null);

// 再接続設定
const RETRY_DELAYS = [1000, 2000, 5000]; // 1s -> 2s -> 5s
const MAX_RETRIES = 3;

export function StreamingProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<StreamStatus>("connecting");
  const [error, setError] = useState<string | null>(null);
  const activeRef = useRef(true);
  const retryCountRef = useRef(0);
  const abortControllerRef = useRef<AbortController | null>(null);

  const startStreaming = useCallback(async () => {
    if (!activeRef.current) return;

    // 新しい AbortController を作成
    abortControllerRef.current = new AbortController();

    // 認証完了フラグ（ローカル変数で追跡）
    let authCompleted = false;

    try {
      setStatus("connecting");

      const stream = authClient.subscribeAuthEvents(
        create(SubscribeAuthEventsRequestSchema, {}),
        { signal: abortControllerRef.current.signal },
      );

      for await (const event of stream) {
        if (!activeRef.current) break;

        // 接続成功、リトライカウントをリセット
        retryCountRef.current = 0;

        switch (event.status) {
          case AuthStatus.PENDING:
            setStatus("connected");
            break;
          case AuthStatus.SUCCESS:
            authCompleted = true;
            activeRef.current = false;
            setStatus("success");
            return;
          case AuthStatus.ERROR:
            authCompleted = true;
            activeRef.current = false;
            setStatus("error");
            setError(event.error || "認証に失敗しました");
            return;
        }
      }

      // ストリームが正常終了した場合
      if (activeRef.current && !authCompleted) {
        // 予期せぬ終了、再接続を試みる
        await handleReconnect();
      }
    } catch (err) {
      if (!activeRef.current) return;

      // AbortError は無視（意図的な切断）
      if (err instanceof Error && err.name === "AbortError") {
        return;
      }

      console.error("Streaming error:", err);

      // 再接続を試みる
      await handleReconnect();
    }
  }, []);

  const handleReconnect = useCallback(async () => {
    if (!activeRef.current) return;

    if (retryCountRef.current >= MAX_RETRIES) {
      setStatus("closed");
      return;
    }

    const delay =
      RETRY_DELAYS[retryCountRef.current] ||
      RETRY_DELAYS[RETRY_DELAYS.length - 1];
    retryCountRef.current++;

    console.log(
      `Reconnecting in ${delay}ms (attempt ${retryCountRef.current}/${MAX_RETRIES})`,
    );

    await new Promise((resolve) => setTimeout(resolve, delay));

    if (activeRef.current) {
      await startStreaming();
    }
  }, [startStreaming]);

  useEffect(() => {
    activeRef.current = true;
    void startStreaming();

    return () => {
      activeRef.current = false;
      abortControllerRef.current?.abort();
    };
  }, [startStreaming]);

  const disconnect = useCallback(() => {
    activeRef.current = false;
    abortControllerRef.current?.abort();
    setStatus("closed");
  }, []);

  const value = useMemo(
    () => ({ status, error, disconnect }),
    [status, error, disconnect],
  );

  return (
    <StreamingContext.Provider value={value}>
      {children}
    </StreamingContext.Provider>
  );
}

export function useStreamingContext() {
  const ctx = useContext(StreamingContext);
  if (!ctx) {
    throw new Error("StreamingContext is not available");
  }
  return ctx;
}
