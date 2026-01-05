import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import Container from "../components/Container";
import { useAuthContext } from "../context/AuthContext";
import { useStreamingContext } from "../context/StreamingContext";

type AuthMethod = "oauth" | "apikey";

interface MethodCardProps {
  method: AuthMethod;
  isActive: boolean;
  onClick: () => void;
}

function MethodCard({ method, isActive, onClick }: MethodCardProps) {
  const isOAuth = method === "oauth";

  return (
    <button
      type="button"
      onClick={onClick}
      className="w-full rounded-2xl border border-ink/10 bg-white px-5 py-4 text-left transition hover:border-ink/20 hover:bg-ink/5"
    >
      <div className="flex items-center gap-4">
        <div
          className={`flex h-12 w-12 items-center justify-center rounded-full text-xl text-white ${
            isOAuth
              ? "bg-gradient-to-br from-violet-500 to-purple-600"
              : "bg-gradient-to-br from-amber-500 to-orange-600"
          }`}
        >
          {isOAuth ? (
            <svg
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
              className="h-6 w-6"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M15.75 5.25a3 3 0 0 1 3 3m3 0a6 6 0 0 1-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1 1 21.75 8.25Z"
              />
            </svg>
          ) : (
            <svg
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
              className="h-6 w-6"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M7.864 4.243A7.5 7.5 0 0 1 19.5 10.5c0 2.92-.556 5.709-1.568 8.268M5.742 6.364A7.465 7.465 0 0 0 4.5 10.5a7.464 7.464 0 0 1-1.15 3.993m1.989 3.559A11.209 11.209 0 0 0 8.25 10.5a3.75 3.75 0 1 1 7.5 0c0 .527-.021 1.049-.064 1.565M12 10.5a14.94 14.94 0 0 1-3.6 9.75m6.633-4.596a18.666 18.666 0 0 1-2.485 5.33"
              />
            </svg>
          )}
        </div>
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <h3 className="font-semibold text-ink">
              {isOAuth ? "OAuth 2.0" : "API Key"}
            </h3>
            {isActive && (
              <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700">
                利用中
              </span>
            )}
          </div>
          <p className="text-sm text-ink/60">
            {isOAuth
              ? "リレーサーバー経由でセキュアに認証"
              : "Backlog の API Key を使用して認証"}
          </p>
        </div>
        <svg
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={2}
          stroke="currentColor"
          className="h-5 w-5 text-ink/40"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="m8.25 4.5 7.5 7.5-7.5 7.5"
          />
        </svg>
      </div>
    </button>
  );
}

export default function LoginMethodSelect() {
  const navigate = useNavigate();
  const { data } = useAuthContext();
  const { status, error: streamError } = useStreamingContext();

  const currentAuthType = data?.currentAuthType as AuthMethod | undefined;

  useEffect(() => {
    if (status === "success") {
      navigate("/auth/complete?type=success", { replace: true });
    } else if (status === "error") {
      const params = new URLSearchParams({ type: "error" });
      if (streamError) params.set("message", streamError);
      navigate(`/auth/complete?${params.toString()}`, { replace: true });
    } else if (status === "closed") {
      navigate("/auth/complete?type=closed", { replace: true });
    }
  }, [status, streamError, navigate]);

  // 常にOAuthを上に表示
  const methods: AuthMethod[] = ["oauth", "apikey"];

  const handleMethodClick = (method: AuthMethod) => {
    if (method === "oauth") {
      navigate("/auth/start");
    } else {
      navigate("/auth/apikey");
    }
  };

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-10">
      <Container>
        <div className="flex flex-col gap-6">
          <header className="space-y-3 text-center">
            <p className="text-xs font-semibold uppercase tracking-[0.3em] text-ink/60">
              Backlog CLI
            </p>
            <h1 className="text-3xl font-semibold text-ink">ログイン方法</h1>
            <p className="text-sm text-ink/70">
              Backlog CLI の認証方法を選択してください。
            </p>
          </header>

          <div className="space-y-4">
            {methods.map((method) => (
              <MethodCard
                key={method}
                method={method}
                isActive={currentAuthType === method}
                onClick={() => handleMethodClick(method)}
              />
            ))}
          </div>
        </div>
      </Container>
    </main>
  );
}
