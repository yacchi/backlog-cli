import { type FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import Button from "../components/Button";
import Container from "../components/Container";
import Input from "../components/Input";
import ResultView from "../components/ResultView";
import StatusIndicator from "../components/StatusIndicator";
import WarningBox from "../components/WarningBox";
import { useAuthContext } from "../context/AuthContext";
import { useWebSocketContext } from "../context/WebSocketContext";

export default function LoginSetup() {
  const navigate = useNavigate();
  const { loading, error, data, refresh } = useAuthContext();
  const { status, error: wsError } = useWebSocketContext();
  const [spaceHost, setSpaceHost] = useState("");
  const [relayServer, setRelayServer] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (!data || dirty) return;
    setSpaceHost(data.spaceHost || "");
    setRelayServer(data.relayServer || "");
  }, [data, dirty]);

  if (status === "success") {
    return (
      <main className="flex min-h-screen items-center justify-center px-4 py-10">
        <Container>
          <ResultView type="success" />
        </Container>
      </main>
    );
  }

  if (status === "error") {
    return (
      <main className="flex min-h-screen items-center justify-center px-4 py-10">
        <Container>
          <ResultView type="error" message={wsError || undefined} />
        </Container>
      </main>
    );
  }

  if (status === "closed") {
    return (
      <main className="flex min-h-screen items-center justify-center px-4 py-10">
        <Container>
          <ResultView type="closed" />
        </Container>
      </main>
    );
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    setFormError(null);

    const formData = new FormData();
    formData.append("space_host", spaceHost);
    formData.append("relay_server", relayServer);

    try {
      const response = await fetch("/auth/configure", {
        method: "POST",
        headers: {
          Accept: "application/json",
        },
        body: formData,
        credentials: "same-origin",
      });

      const payload = (await response.json()) as { error?: string };
      if (!response.ok || payload.error) {
        setFormError(payload.error || "設定の保存に失敗しました");
        setSubmitting(false);
        return;
      }

      await refresh();
      navigate("/auth/start");
    } catch (err) {
      setFormError(
        err instanceof Error ? err.message : "不明なエラーが発生しました",
      );
      setSubmitting(false);
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
            <h1 className="text-3xl font-semibold text-ink">ログイン設定</h1>
            <p className="text-sm text-ink/70">
              ブラウザから認証を行うために、Backlog のスペース情報を登録します。
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
            <Input
              label="スペース"
              placeholder="yourspace.backlog.jp"
              value={spaceHost}
              onChange={(event) => {
                setSpaceHost(event.target.value);
                setDirty(true);
              }}
              required
              disabled={loading}
            />
            <Input
              label="リレーサーバーURL"
              type="url"
              placeholder="https://relay.example.com"
              value={relayServer}
              onChange={(event) => {
                setRelayServer(event.target.value);
                setDirty(true);
              }}
              helper="OAuth 認証を中継するサーバーの URL"
              required
              disabled={loading}
            />

            <WarningBox title="セキュリティに関する注意">
              リレーサーバーは OAuth
              認証を中継し、アクセストークンを取り扱います。信頼できる
              サーバーのみを指定してください。不明な場合は、組織の管理者にご確認ください。
            </WarningBox>

            <div className="flex flex-wrap items-center justify-center gap-3 pt-2">
              <Button
                type="button"
                variant="secondary"
                onClick={() => navigate("/auth/start")}
                disabled={submitting}
              >
                キャンセル
              </Button>
              <Button type="submit" disabled={loading || submitting}>
                登録して続行
              </Button>
            </div>
          </form>

          {submitting ? (
            <StatusIndicator message="設定を保存しています..." />
          ) : null}
        </div>
      </Container>
    </main>
  );
}
