import { useState, useEffect } from "react";
import { useParams } from "react-router-dom";
import Button from "../components/Button";
import Container from "../components/Container";
import Input from "../components/Input";
import InfoBox from "../components/InfoBox";

interface TenantInfo {
  name: string;
  relay_url: string;
}

interface SessionInfo {
  authenticated: boolean;
  user?: { id: string; name: string; email: string };
  tenant?: string;
  role?: number;
  auth_time?: number;
}

interface PortalInfo {
  name: string;
  has_passphrase: boolean;
  oauth_enabled: boolean;
  default_space?: string;
}

type SetupMethod = "quickstart" | "provision" | "bundle";
type LoginMethod = "oauth" | "passphrase";

const errorMessages: Record<string, string> = {
  invalid_passphrase: "パスフレーズが正しくありません",
  portal_not_enabled: "このテナントではポータルが有効化されていません",
  "tenant not found": "テナントが見つかりません",
  "failed to create bundle": "バンドルの作成に失敗しました",
  "failed to generate provisioning key":
    "プロビジョニングキーの生成に失敗しました",
  authentication_required: "認証が必要です",
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
  const [copiedQuickstart, setCopiedQuickstart] = useState<
    "curl" | "cli" | null
  >(null);

  // OAuth-related state
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [portalInfo, setPortalInfo] = useState<PortalInfo | null>(null);
  const [loginMethod, setLoginMethod] = useState<LoginMethod>("oauth");
  const [oauthSpace, setOauthSpace] = useState("");
  const [initialLoading, setInitialLoading] = useState(true);

  // Check session and fetch portal info on mount
  useEffect(() => {
    if (!name) return;

    Promise.all([
      fetch("/api/v1/portal/session").then((r) => r.json()),
      fetch(`/api/v1/portal/${encodeURIComponent(name)}/info`).then((r) =>
        r.ok ? r.json() : null,
      ),
    ])
      .then(([sessionData, infoData]) => {
        setSession(sessionData as SessionInfo);
        if (infoData) {
          setPortalInfo(infoData as PortalInfo);
          if ((infoData as PortalInfo).default_space) {
            setOauthSpace((infoData as PortalInfo).default_space!);
          }
          if (!(infoData as PortalInfo).oauth_enabled) {
            setLoginMethod("passphrase");
          }
        }

        // If authenticated via OAuth, set tenant info from session
        if ((sessionData as SessionInfo).authenticated && infoData) {
          setTenantInfo({
            name: (infoData as PortalInfo).name,
            relay_url: window.location.origin,
          });
        }
      })
      .catch(() => {
        setSession({ authenticated: false });
      })
      .finally(() => {
        setInitialLoading(false);
      });
  }, [name]);

  const isAuthenticated = session?.authenticated === true;

  const handleOAuthLogin = () => {
    if (!name) return;
    const space = oauthSpace.trim();
    if (!space) return;
    const url = `/portal/${encodeURIComponent(name)}/auth/start?space=${encodeURIComponent(space)}`;
    window.location.href = url;
  };

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
      const fetchOpts: RequestInit = {
        method: "POST",
        credentials: "same-origin",
      };

      if (!isAuthenticated) {
        fetchOpts.headers = { "Content-Type": "application/json" };
        fetchOpts.body = JSON.stringify({ passphrase });
      }

      const response = await fetch(url, fetchOpts);

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
      const fetchOpts: RequestInit = {
        method: "POST",
        credentials: "same-origin",
      };

      if (!isAuthenticated) {
        fetchOpts.headers = { "Content-Type": "application/json" };
        fetchOpts.body = JSON.stringify({ passphrase });
      }

      const response = await fetch(url, fetchOpts);
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

  const handleCopyQuickstart = async (type: "curl" | "cli") => {
    if (!tenantInfo || !name) return;
    const command =
      type === "curl"
        ? `curl -fsSL ${tenantInfo.relay_url}/install.sh | sh -s -- --name ${name}`
        : `backlog config setup --relay-url ${tenantInfo.relay_url} --name ${name}`;
    await navigator.clipboard.writeText(command);
    setCopiedQuickstart(type);
    setTimeout(() => setCopiedQuickstart(null), 2000);
  };

  const handleLogout = async () => {
    await fetch("/api/v1/portal/session", { method: "DELETE" });
    setSession({ authenticated: false });
    setTenantInfo(null);
    setPassphrase("");
    setError(null);
    setProvisioningKey(null);
    setCopied(false);
  };

  const handleReset = () => {
    if (isAuthenticated) {
      setProvisioningKey(null);
      setCopied(false);
      setError(null);
      return;
    }
    setTenantInfo(null);
    setPassphrase("");
    setError(null);
    setProvisioningKey(null);
    setCopied(false);
  };

  if (initialLoading) {
    return (
      <main className="flex min-h-screen items-center justify-center px-4 py-10">
        <Container>
          <div className="flex flex-col gap-6">
            <header className="space-y-3 text-center">
              <p className="text-xs font-semibold uppercase tracking-[0.3em] text-ink/60">
                Backlog CLI
              </p>
              <h1 className="text-3xl font-semibold text-ink">設定ポータル</h1>
              <p className="text-sm text-ink/70">読み込み中...</p>
            </header>
          </div>
        </Container>
      </main>
    );
  }

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

          {isAuthenticated && session?.user && (
            <div className="flex items-center justify-between rounded-2xl border border-emerald-200 bg-emerald-50 px-4 py-3">
              <span className="text-sm text-emerald-800">
                {session.user.name} としてログイン中
              </span>
              <div className="flex items-center gap-3">
                {session.role === 1 && (
                  <a
                    href={`/portal/${encodeURIComponent(name ?? "")}/admin`}
                    className="text-xs font-medium text-emerald-600 hover:text-emerald-800"
                  >
                    管理
                  </a>
                )}
                <button
                  type="button"
                  className="text-xs font-medium text-emerald-600 hover:text-emerald-800"
                  onClick={handleLogout}
                >
                  ログアウト
                </button>
              </div>
            </div>
          )}

          {error && (
            <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
              {error}
            </div>
          )}

          {!tenantInfo && !isAuthenticated ? (
            <div className="space-y-5">
              {/* Login method tabs */}
              {portalInfo?.oauth_enabled && portalInfo?.has_passphrase && (
                <div className="flex rounded-2xl border border-outline/60 bg-white/50 p-1">
                  <button
                    type="button"
                    className={`flex-1 rounded-xl px-3 py-2 text-sm font-medium transition-colors ${
                      loginMethod === "oauth"
                        ? "bg-white text-ink shadow-sm"
                        : "text-ink/60 hover:text-ink/80"
                    }`}
                    onClick={() => setLoginMethod("oauth")}
                  >
                    Backlog でログイン
                  </button>
                  <button
                    type="button"
                    className={`flex-1 rounded-xl px-3 py-2 text-sm font-medium transition-colors ${
                      loginMethod === "passphrase"
                        ? "bg-white text-ink shadow-sm"
                        : "text-ink/60 hover:text-ink/80"
                    }`}
                    onClick={() => setLoginMethod("passphrase")}
                  >
                    パスフレーズ
                  </button>
                </div>
              )}

              {loginMethod === "oauth" && portalInfo?.oauth_enabled ? (
                <div className="space-y-4">
                  {!portalInfo.default_space && (
                    <Input
                      label="Backlog スペース"
                      type="text"
                      value={oauthSpace}
                      onChange={(e) => setOauthSpace(e.target.value)}
                      placeholder="example.backlog.jp"
                      helper="ログインする Backlog スペースのホスト名を入力してください"
                      required
                      autoFocus
                    />
                  )}
                  <div className="flex justify-center pt-2">
                    <Button
                      onClick={handleOAuthLogin}
                      disabled={!oauthSpace.trim()}
                    >
                      Backlog でログイン
                    </Button>
                  </div>
                  <p className="text-center text-xs text-ink/50">
                    Backlog の認証画面に移動します
                  </p>
                </div>
              ) : (
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
              )}
            </div>
          ) : (
            <div className="space-y-5">
              {tenantInfo && (
                <div className="grid gap-3">
                  <InfoBox label="バンドル名" value={tenantInfo.name} />
                  <InfoBox label="リレーサーバー" value={tenantInfo.relay_url} />
                </div>
              )}

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
                    ターミナルで以下のコマンドを実行すると、CLIのインストールとセットアップが完了します。
                  </p>
                  <div className="relative">
                    <div className="rounded-2xl border border-outline/60 bg-ink/5 p-4 pr-20">
                      <code className="block break-all text-sm leading-relaxed text-ink">
                        curl -fsSL {tenantInfo?.relay_url ?? window.location.origin}/install.sh | sh
                        -s -- --name {name}
                      </code>
                    </div>
                    <button
                      type="button"
                      className="absolute right-3 top-3 rounded-lg border border-outline/60 bg-white px-3 py-1.5 text-xs font-medium text-ink/70 shadow-sm transition-colors hover:bg-ink/5"
                      onClick={() => handleCopyQuickstart("curl")}
                    >
                      {copiedQuickstart === "curl" ? "Copied!" : "Copy"}
                    </button>
                  </div>
                  <p className="text-center text-xs text-ink/50">
                    既にCLIがインストール済みの場合は直接セットアップできます:
                  </p>
                  <div className="relative">
                    <div className="rounded-2xl border border-outline/60 bg-ink/5 p-4 pr-20">
                      <code className="block break-all text-sm leading-relaxed text-ink">
                        backlog config setup --relay-url{" "}
                        {tenantInfo?.relay_url ?? window.location.origin} --name {name}
                      </code>
                    </div>
                    <button
                      type="button"
                      className="absolute right-3 top-3 rounded-lg border border-outline/60 bg-white px-3 py-1.5 text-xs font-medium text-ink/70 shadow-sm transition-colors hover:bg-ink/5"
                      onClick={() => handleCopyQuickstart("cli")}
                    >
                      {copiedQuickstart === "cli" ? "Copied!" : "Copy"}
                    </button>
                  </div>
                  <p className="text-center text-xs text-ink/50">
                    ブラウザが開き Backlog OAuth で認証を行います
                  </p>
                  <div className="flex justify-center gap-3 pt-1">
                    {!isAuthenticated && (
                      <Button variant="secondary" onClick={handleReset}>
                        戻る
                      </Button>
                    )}
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
                        {!isAuthenticated && (
                          <Button
                            variant="secondary"
                            onClick={handleReset}
                            disabled={generatingKey}
                          >
                            戻る
                          </Button>
                        )}
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
                          {isAuthenticated ? "キーを再生成" : "戻る"}
                        </Button>
                      </div>
                    </>
                  )}
                </div>
              ) : (
                <div className="space-y-4">
                  <div className="flex justify-center gap-3 pt-1">
                    {!isAuthenticated && (
                      <Button
                        variant="secondary"
                        onClick={handleReset}
                        disabled={downloading}
                      >
                        戻る
                      </Button>
                    )}
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
