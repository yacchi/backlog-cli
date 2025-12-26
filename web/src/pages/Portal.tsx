import { useState } from "react";
import { useParams } from "react-router-dom";
import Button from "../components/Button";
import Container from "../components/Container";
import Input from "../components/Input";
import InfoBox from "../components/InfoBox";

interface TenantInfo {
  domain: string;
  relay_url: string;
  space: string;
  backlog_domain: string;
}

const errorMessages: Record<string, string> = {
  invalid_passphrase: "パスフレーズが正しくありません",
  portal_not_enabled: "このテナントではポータルが有効化されていません",
  "tenant not found": "テナントが見つかりません",
  "failed to create bundle": "バンドルの作成に失敗しました",
};

function translateError(error: string): string {
  return errorMessages[error] || error;
}

export default function Portal() {
  const { domain } = useParams<{ domain: string }>();
  const [passphrase, setPassphrase] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [tenantInfo, setTenantInfo] = useState<TenantInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [downloading, setDownloading] = useState(false);

  const handleVerify = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    try {
      const response = await fetch("/api/v1/portal/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ domain, passphrase }),
      });
      const data = await response.json();

      if (data.success) {
        setTenantInfo(data);
      } else {
        setError(translateError(data.error) || "認証に失敗しました");
      }
    } catch {
      setError("ネットワークエラーが発生しました");
    } finally {
      setLoading(false);
    }
  };

  const handleDownload = async () => {
    if (!domain) return;
    setDownloading(true);
    setError(null);

    try {
      const url = `/api/v1/portal/${encodeURIComponent(domain)}/bundle?passphrase=${encodeURIComponent(passphrase)}`;
      const response = await fetch(url);

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(
          translateError(data.error) || "ダウンロードに失敗しました",
        );
      }

      const blob = await response.blob();
      const downloadUrl = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = downloadUrl;
      a.download = `${domain}.backlog-cli.zip`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(downloadUrl);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "ダウンロードに失敗しました",
      );
    } finally {
      setDownloading(false);
    }
  };

  const handleReset = () => {
    setTenantInfo(null);
    setPassphrase("");
    setError(null);
  };

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-10">
      <Container>
        <div className="flex flex-col gap-6">
          <header className="space-y-3 text-center">
            <p className="text-xs font-semibold uppercase tracking-[0.3em] text-ink/60">
              Backlog CLI
            </p>
            <h1 className="text-3xl font-semibold text-ink">設定ポータル</h1>
            <p className="text-sm text-ink/70">
              {domain
                ? `${domain} の設定バンドルをダウンロード`
                : "設定バンドルのダウンロード"}
            </p>
          </header>

          {error && (
            <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
              {error}
            </div>
          )}

          {!tenantInfo ? (
            <form onSubmit={handleVerify} className="space-y-5">
              <Input
                label="パスフレーズ"
                type="password"
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
                placeholder="パスフレーズを入力"
                helper="管理者から提供されたパスフレーズを入力してください"
                required
                disabled={loading}
                autoFocus
              />
              <div className="flex justify-center pt-2">
                <Button type="submit" disabled={loading || !passphrase}>
                  {loading ? "確認中..." : "確認"}
                </Button>
              </div>
            </form>
          ) : (
            <div className="space-y-5">
              <div className="grid gap-3">
                <InfoBox label="スペース" value={tenantInfo.space} />
                <InfoBox label="ドメイン" value={tenantInfo.backlog_domain} />
                <InfoBox label="リレーサーバー" value={tenantInfo.relay_url} />
              </div>
              <div className="flex justify-center gap-3 pt-2">
                <Button
                  variant="secondary"
                  onClick={handleReset}
                  disabled={downloading}
                >
                  戻る
                </Button>
                <Button onClick={handleDownload} disabled={downloading}>
                  {downloading ? "ダウンロード中..." : "バンドルをダウンロード"}
                </Button>
              </div>
              <p className="text-center text-xs text-ink/50">
                ダウンロード後、以下のコマンドでインポートしてください
              </p>
              <p className="text-center">
                <code className="rounded bg-ink/10 px-2 py-1 text-sm">
                  backlog config import {domain}.backlog-cli.zip
                </code>
              </p>
            </div>
          )}
        </div>
      </Container>
    </main>
  );
}
