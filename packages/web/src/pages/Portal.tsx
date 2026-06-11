import { useState } from "react";
import { useParams } from "react-router-dom";
import Button from "../components/Button";
import Container from "../components/Container";
import Input from "../components/Input";
import InfoBox from "../components/InfoBox";

interface TenantInfo {
  name: string;
  relay_url: string;
}

type SetupMethod = "quickstart" | "provision" | "bundle";

const errorMessages: Record<string, string> = {
  invalid_passphrase: "パスフレーズが正しくありません",
  portal_not_enabled: "このテナントではポータルが有効化されていません",
  "tenant not found": "テナントが見つかりません",
  "failed to create bundle": "バンドルの作成に失敗しました",
  "failed to generate provisioning key":
    "プロビジョニングキーの生成に失敗しました",
};

function translateError(error: string): string {
  return errorMessages[error] || error;
}

export default function Portal() {
  const { name } = useParams<{ name: string }>();
  const [passphrase, setPassphrase] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [tenantInfo, setTenantInfo] = useState<TenantInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [setupMethod, setSetupMethod] = useState<SetupMethod>("quickstart");
  const [provisioningKey, setProvisioningKey] = useState<string | null>(null);
  const [generatingKey, setGeneratingKey] = useState(false);
  const [copied, setCopied] = useState(false);

  const handleVerify = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    try {
      const response = await fetch("/api/v1/portal/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, passphrase }),
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
    if (!name) return;
    setDownloading(true);
    setError(null);

    try {
      const url = `/api/v1/portal/${encodeURIComponent(name)}/bundle`;
      const response = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ passphrase }),
      });

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
      a.download = `${name}.backlog-cli.zip`;
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

  const handleGenerateKey = async () => {
    if (!name) return;
    setGeneratingKey(true);
    setError(null);

    try {
      const url = `/api/v1/portal/${encodeURIComponent(name)}/provision`;
      const response = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ passphrase }),
      });

      const data = await response.json();
      if (data.success) {
        setProvisioningKey(data.provisioning_key);
      } else {
        throw new Error(
          translateError(data.error) || "キーの生成に失敗しました",
        );
      }
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "キーの生成に失敗しました",
      );
    } finally {
      setGeneratingKey(false);
    }
  };

  const handleCopyCommand = async () => {
    if (!provisioningKey) return;
    const command = `backlog config setup ${provisioningKey}`;
    await navigator.clipboard.writeText(command);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleReset = () => {
    setTenantInfo(null);
    setPassphrase("");
    setError(null);
    setProvisioningKey(null);
    setCopied(false);
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
              {name
                ? `${name} の設定バンドルをダウンロード`
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
                <InfoBox label="バンドル名" value={tenantInfo.name} />
                <InfoBox label="リレーサーバー" value={tenantInfo.relay_url} />
              </div>

              {/* Method tabs */}
              <div className="flex rounded-2xl border border-outline/60 bg-white/50 p-1">
                <button
                  type="button"
                  className={`flex-1 rounded-xl px-3 py-2 text-sm font-medium transition-colors ${
                    setupMethod === "quickstart"
                      ? "bg-white text-ink shadow-sm"
                      : "text-ink/60 hover:text-ink/80"
                  }`}
                  onClick={() => setSetupMethod("quickstart")}
                >
                  クイックスタート
                </button>
                <button
                  type="button"
                  className={`flex-1 rounded-xl px-3 py-2 text-sm font-medium transition-colors ${
                    setupMethod === "provision"
                      ? "bg-white text-ink shadow-sm"
                      : "text-ink/60 hover:text-ink/80"
                  }`}
                  onClick={() => setSetupMethod("provision")}
                >
                  CLI セットアップ
                </button>
                <button
                  type="button"
                  className={`flex-1 rounded-xl px-3 py-2 text-sm font-medium transition-colors ${
                    setupMethod === "bundle"
                      ? "bg-white text-ink shadow-sm"
                      : "text-ink/60 hover:text-ink/80"
                  }`}
                  onClick={() => setSetupMethod("bundle")}
                >
                  バンドルダウンロード
                </button>
              </div>

              {setupMethod === "quickstart" ? (
                <div className="space-y-4">
                  <p className="text-center text-sm text-ink/70">
                    ターミナルで以下のコマンドを実行すると、CLIのインストールとセットアップが完了します
                  </p>
                  <div className="relative">
                    <div className="rounded-2xl border border-outline/60 bg-ink/5 p-4">
                      <code className="block break-all text-sm leading-relaxed text-ink">
                        curl -fsSL {tenantInfo.relay_url}/install.sh | sh -s -- --name {name} --passphrase '...'
                      </code>
                    </div>
                  </div>
                  <p className="text-center text-xs text-ink/50">
                    パスフレーズには先ほど入力した値を指定してください
                  </p>
                  <p className="text-center text-xs text-ink/50">
                    既にCLIがインストール済みの場合は直接セットアップできます:
                  </p>
                  <div className="relative">
                    <div className="rounded-2xl border border-outline/60 bg-ink/5 p-4">
                      <code className="block break-all text-sm leading-relaxed text-ink">
                        backlog config setup --relay-url {tenantInfo.relay_url} --name {name} --passphrase '...'
                      </code>
                    </div>
                  </div>
                  <div className="flex justify-center gap-3 pt-1">
                    <Button variant="secondary" onClick={handleReset}>
                      戻る
                    </Button>
                  </div>
                </div>
              ) : setupMethod === "provision" ? (
                <div className="space-y-4">
                  {!provisioningKey ? (
                    <>
                      <p className="text-center text-sm text-ink/70">
                        プロビジョニングキーを発行して、CLIに貼り付けるだけでセットアップが完了します
                      </p>
                      <div className="flex justify-center gap-3 pt-1">
                        <Button
                          variant="secondary"
                          onClick={handleReset}
                          disabled={generatingKey}
                        >
                          戻る
                        </Button>
                        <Button
                          onClick={handleGenerateKey}
                          disabled={generatingKey}
                        >
                          {generatingKey
                            ? "生成中..."
                            : "プロビジョニングキーを発行"}
                        </Button>
                      </div>
                    </>
                  ) : (
                    <>
                      <p className="text-center text-sm text-ink/70">
                        以下のコマンドをターミナルで実行してください
                      </p>
                      <div className="relative">
                        <div className="rounded-2xl border border-outline/60 bg-ink/5 p-4">
                          <code className="block break-all text-sm leading-relaxed text-ink">
                            backlog config setup {provisioningKey}
                          </code>
                        </div>
                        <button
                          type="button"
                          className="absolute right-3 top-3 rounded-lg border border-outline/60 bg-white px-3 py-1.5 text-xs font-medium text-ink/70 shadow-sm transition-colors hover:bg-ink/5"
                          onClick={handleCopyCommand}
                        >
                          {copied ? "Copied!" : "Copy"}
                        </button>
                      </div>
                      <p className="text-center text-xs text-ink/50">
                        キーの有効期限は15分です
                      </p>
                      <div className="flex justify-center gap-3 pt-1">
                        <Button variant="secondary" onClick={handleReset}>
                          戻る
                        </Button>
                      </div>
                    </>
                  )}
                </div>
              ) : (
                <div className="space-y-4">
                  <div className="flex justify-center gap-3 pt-1">
                    <Button
                      variant="secondary"
                      onClick={handleReset}
                      disabled={downloading}
                    >
                      戻る
                    </Button>
                    <Button onClick={handleDownload} disabled={downloading}>
                      {downloading
                        ? "ダウンロード中..."
                        : "バンドルをダウンロード"}
                    </Button>
                  </div>
                  <p className="text-center text-xs text-ink/50">
                    ダウンロード後、以下のコマンドでインポートしてください
                  </p>
                  <p className="text-center">
                    <code className="rounded bg-ink/10 px-2 py-1 text-sm">
                      backlog config import {name}.backlog-cli.zip
                    </code>
                  </p>
                </div>
              )}
            </div>
          )}
        </div>
      </Container>
    </main>
  );
}
