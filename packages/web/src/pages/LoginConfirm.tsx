import { useEffect, useRef, useState } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import Button from "../components/Button";
import Container from "../components/Container";
import InfoBox from "../components/InfoBox";
import type { ResultType } from "../components/ResultView";
import StatusIndicator from "../components/StatusIndicator";
import { useAuthContext } from "../context/AuthContext";
import { useStreamingContext } from "../context/StreamingContext";
import { openPopupCentered } from "../utils/popup";

export default function LoginConfirm() {
  const navigate = useNavigate();
  const { loading, error, data } = useAuthContext();
  const { status, error: streamError, disconnect } = useStreamingContext();
  const [isLoggingIn, setIsLoggingIn] = useState(false);
  const [popupMessage, setPopupMessage] = useState<string | null>(null);
  const [forcedResult, setForcedResult] = useState<ResultType | null>(null);
  const popupCheckRef = useRef<number | null>(null);
  // statusをrefで追跡（interval内でのクロージャ問題を回避）
  const statusRef = useRef(status);
  statusRef.current = status;

  useEffect(() => {
    if (
      status === "success" ||
      status === "error" ||
      status === "closed" ||
      forcedResult
    ) {
      if (popupCheckRef.current) {
        window.clearInterval(popupCheckRef.current);
        popupCheckRef.current = null;
      }
    }
  }, [status, forcedResult]);

  // Navigate to complete page when auth finishes
  useEffect(() => {
    if (forcedResult) {
      const params = new URLSearchParams({ type: forcedResult });
      if (popupMessage) params.set("message", popupMessage);
      navigate(`/auth/complete?${params.toString()}`, { replace: true });
    } else if (status === "success") {
      navigate("/auth/complete?type=success", { replace: true });
    } else if (status === "error") {
      const params = new URLSearchParams({ type: "error" });
      if (streamError) params.set("message", streamError);
      navigate(`/auth/complete?${params.toString()}`, { replace: true });
    } else if (status === "closed") {
      navigate("/auth/complete?type=closed", { replace: true });
    }
  }, [status, streamError, forcedResult, popupMessage, navigate]);

  if (!loading && data && !data.configured) {
    return <Navigate to="/auth/setup" replace />;
  }

  const handleLogin = () => {
    setPopupMessage(null);
    setIsLoggingIn(true);

    const popup = openPopupCentered("/auth/popup", "backlog_auth", 600, 700);

    if (!popup || popup.closed || typeof popup.closed === "undefined") {
      setPopupMessage(
        "ポップアップがブロックされました。ポップアップを許可してください。",
      );
      setIsLoggingIn(false);
      return;
    }

    setPopupMessage("ポップアップで認証を進めてください...");

    popupCheckRef.current = window.setInterval(() => {
      const currentStatus = statusRef.current;

      // 認証が完了した場合はチェック終了
      if (
        currentStatus === "success" ||
        currentStatus === "error" ||
        currentStatus === "closed"
      ) {
        if (popupCheckRef.current) {
          window.clearInterval(popupCheckRef.current);
          popupCheckRef.current = null;
        }
        return;
      }

      // ポップアップが閉じられた場合（認証完了以外で）
      if (popup.closed) {
        if (popupCheckRef.current) {
          window.clearInterval(popupCheckRef.current);
          popupCheckRef.current = null;
        }
        // React状態更新の反映を待つために少し遅延させる
        // これにより、認証成功直後にポップアップが閉じられた場合の
        // タイミング問題を回避する
        setTimeout(() => {
          const finalStatus = statusRef.current;
          // 認証が完了していない場合のみエラーとする
          if (finalStatus === "connecting" || finalStatus === "connected") {
            disconnect();
            setForcedResult("error");
            setPopupMessage(
              "認証がキャンセルされました。ポップアップが閉じられました。",
            );
          }
        }, 500);
      }
    }, 1000);
  };

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-10">
      <Container>
        <div className="flex flex-col gap-6">
          <header className="space-y-3 text-center">
            <p className="text-xs font-semibold uppercase tracking-[0.3em] text-ink/60">
              Backlog CLI
            </p>
            <h1 className="text-3xl font-semibold text-ink">ログイン</h1>
            <p className="text-sm text-ink/70">
              Backlog CLI がターミナルからの操作で Backlog API
              にアクセスするための認証を行います。
            </p>
          </header>

          {error ? (
            <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
              {error}
            </div>
          ) : null}

          {popupMessage && !isLoggingIn ? (
            <div className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700">
              {popupMessage}
            </div>
          ) : null}

          <div className="space-y-3">
            <InfoBox
              label="スペース"
              value={data ? `${data.space}.${data.domain}` : "読み込み中..."}
            />
            <InfoBox
              label="リレーサーバー"
              value={data ? data.relayServer : "読み込み中..."}
            />
          </div>

          <div className="flex flex-wrap items-center justify-center gap-3">
            <Button
              variant="secondary"
              type="button"
              onClick={() => {
                navigate("/auth/method");
              }}
            >
              戻る
            </Button>
            <Button
              variant="secondary"
              type="button"
              onClick={() => {
                navigate("/auth/setup");
              }}
            >
              設定を変更
            </Button>
            <Button
              type="button"
              onClick={handleLogin}
              disabled={isLoggingIn || loading}
            >
              ログインする
            </Button>
          </div>

          {isLoggingIn ? (
            <StatusIndicator
              message={popupMessage || "ログイン画面を開いています..."}
            />
          ) : null}
        </div>
      </Container>
    </main>
  );
}
