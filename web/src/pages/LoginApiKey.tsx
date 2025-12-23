import { type FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { create } from "@bufbuild/protobuf";
import Button from "../components/Button";
import Container from "../components/Container";
import InfoBox from "../components/InfoBox";
import Input from "../components/Input";
import StatusIndicator from "../components/StatusIndicator";
import WarningBox from "../components/WarningBox";
import { useAuthContext } from "../context/AuthContext";
import { useStreamingContext } from "../context/StreamingContext";
import { authClient } from "../lib/connect-client";
import { AuthenticateWithApiKeyRequestSchema } from "../gen/auth/v1/auth_pb";

export default function LoginApiKey() {
  const navigate = useNavigate();
  const { loading, error, data, refresh } = useAuthContext();
  const { status, error: streamError } = useStreamingContext();
  const [spaceHost, setSpaceHost] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [spaceHostDirty, setSpaceHostDirty] = useState(false);
  const [showSpaceInput, setShowSpaceInput] = useState(false);

  // 既存のスペース設定があるかどうか
  const hasExistingSpace = Boolean(data?.spaceHost);
  const existingSpaceHost = data?.spaceHost || "";

  useEffect(() => {
    if (!data) return;
    // 既存の設定がない場合はスペース入力を表示
    if (!data.spaceHost) {
      setShowSpaceInput(true);
    }
  }, [data]);

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

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    setFormError(null);

    try {
      // スペースが変更された場合のみ spaceHost を送信
      // 既存のスペースを使う場合は空文字を送信（サーバー側で既存値を使用）
      const sendSpaceHost = showSpaceInput && spaceHostDirty ? spaceHost : "";

      const response = await authClient.authenticateWithApiKey(
        create(AuthenticateWithApiKeyRequestSchema, {
          spaceHost: sendSpaceHost,
          apiKey,
        }),
      );

      if (!response.success || response.error) {
        setFormError(response.error || "認証に失敗しました");
        setSubmitting(false);
        return;
      }

      // 認証成功 - ストリーミングで状態変更が通知されるので待つ
      await refresh();
      // status が success になるので ResultView が表示される
    } catch (err) {
      setFormError(
        err instanceof Error ? err.message : "不明なエラーが発生しました",
      );
      setSubmitting(false);
    }
  };

  // API Key設定ページへのリンクを生成
  const displaySpaceHost = showSpaceInput ? spaceHost : existingSpaceHost;
  const apiKeySettingsUrl = displaySpaceHost
    ? `https://${displaySpaceHost}/EditApiSettings.action`
    : null;

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-10">
      <Container>
        <div className="flex flex-col gap-6">
          <header className="space-y-3 text-center">
            <p className="text-xs font-semibold uppercase tracking-[0.3em] text-ink/60">
              Backlog CLI
            </p>
            <h1 className="text-3xl font-semibold text-ink">API Key 認証</h1>
            <p className="text-sm text-ink/70">
              Backlog の API Key を使用して認証します。
            </p>
          </header>

          {error ? (
            <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
              {error}
            </div>
          ) : null}

          {formError ? (
            <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
              {formError}
            </div>
          ) : null}

          <form className="space-y-5" onSubmit={handleSubmit}>
            {/* 既存のスペースがある場合は表示、なければ入力欄 */}
            {hasExistingSpace && !showSpaceInput ? (
              <div className="space-y-3">
                <InfoBox label="スペース" value={existingSpaceHost} />
                <button
                  type="button"
                  onClick={() => {
                    setShowSpaceInput(true);
                    setSpaceHost(existingSpaceHost);
                  }}
                  className="text-sm text-violet-600 hover:underline"
                >
                  別のスペースを使用する
                </button>
              </div>
            ) : (
              <div className="space-y-3">
                <Input
                  label="スペース"
                  placeholder="yourspace.backlog.jp"
                  value={spaceHost}
                  onChange={(event) => {
                    setSpaceHost(event.target.value);
                    setSpaceHostDirty(true);
                  }}
                  required
                  disabled={loading || submitting}
                />
                {hasExistingSpace && (
                  <button
                    type="button"
                    onClick={() => {
                      setShowSpaceInput(false);
                      setSpaceHost("");
                      setSpaceHostDirty(false);
                    }}
                    className="text-sm text-violet-600 hover:underline"
                  >
                    既存のスペース ({existingSpaceHost}) を使用する
                  </button>
                )}
              </div>
            )}

            <Input
              label="API Key"
              type="password"
              placeholder="API Key を入力"
              value={apiKey}
              onChange={(event) => {
                setApiKey(event.target.value);
              }}
              helper={
                apiKeySettingsUrl ? (
                  <>
                    API Key は{" "}
                    <a
                      href={apiKeySettingsUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-violet-600 hover:underline"
                    >
                      個人設定 &gt; API
                    </a>{" "}
                    から取得できます
                  </>
                ) : (
                  "API Key は Backlog の個人設定から取得できます"
                )
              }
              required
              disabled={loading || submitting}
            />

            <WarningBox title="セキュリティに関する注意">
              API Key は Backlog
              アカウントへのフルアクセス権限を持ちます。第三者に共有しないでください。
            </WarningBox>

            <div className="flex flex-wrap items-center justify-center gap-3 pt-2">
              <Button
                type="button"
                variant="secondary"
                onClick={() => navigate("/auth/method")}
                disabled={submitting}
              >
                戻る
              </Button>
              <Button type="submit" disabled={loading || submitting}>
                認証する
              </Button>
            </div>
          </form>

          {submitting ? <StatusIndicator message="認証しています..." /> : null}
        </div>
      </Container>
    </main>
  );
}
