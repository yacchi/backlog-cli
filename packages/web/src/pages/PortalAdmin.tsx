import { useState, useEffect, useCallback } from "react";
import { useParams, useNavigate } from "react-router-dom";
import AuditLogViewer from "../components/AuditLogViewer";
import PassphraseManagerView from "../components/PassphraseManager";

type AdminTab = "audit" | "passphrase";

interface SessionInfo {
    authenticated: boolean;
    user?: { id: string; name: string; email: string };
    tenant?: string;
    role?: number;
    auth_time?: number;
}

export default function PortalAdmin() {
    const { name } = useParams<{ name: string }>();
    const navigate = useNavigate();
    const [session, setSession] = useState<SessionInfo | null>(null);
    const [loading, setLoading] = useState(true);
    const [tab, setTab] = useState<AdminTab>("audit");
    const [authError, setAuthError] = useState<string | null>(null);

    useEffect(() => {
        fetch("/api/v1/portal/session")
            .then((r) => r.json())
            .then((data: SessionInfo) => {
                setSession(data);
                if (!data.authenticated || data.role !== 1) {
                    navigate(`/portal/${encodeURIComponent(name ?? "")}`, { replace: true });
                }
            })
            .catch(() => {
                navigate(`/portal/${encodeURIComponent(name ?? "")}`, { replace: true });
            })
            .finally(() => setLoading(false));
    }, [name, navigate]);

    const handleReauth = useCallback(() => {
        const portalUrl = `/portal/${encodeURIComponent(name ?? "")}`;
        fetch(`/api/v1/portal/${encodeURIComponent(name ?? "")}/info`)
            .then((r) => r.json())
            .then((info: { default_space?: string }) => {
                if (info.default_space) {
                    window.location.href = `${portalUrl}/auth/start?space=${encodeURIComponent(info.default_space)}`;
                } else {
                    navigate(portalUrl, { replace: true });
                }
            })
            .catch(() => navigate(portalUrl, { replace: true }));
    }, [name, navigate]);

    const handleApiError = useCallback((status: number, data: { error?: string }) => {
        if (status === 401 && data.error === "reauth_required") {
            setAuthError("セッションの有効期限が切れました。再認証が必要です。");
            return true;
        }
        if (status === 403) {
            navigate(`/portal/${encodeURIComponent(name ?? "")}`, { replace: true });
            return true;
        }
        return false;
    }, [name, navigate]);

    if (loading) {
        return (
            <main className="min-h-screen px-6 py-10">
                <div className="mx-auto max-w-6xl">
                    <div className="glass-card rounded-3xl border border-white/70 bg-white/85 p-8 shadow-glow backdrop-blur-xl">
                        <header className="space-y-3 text-center">
                            <p className="text-xs font-semibold uppercase tracking-[0.3em] text-ink/60">
                                Backlog CLI
                            </p>
                            <h1 className="text-3xl font-semibold text-ink">管理画面</h1>
                            <p className="text-sm text-ink/70">読み込み中...</p>
                        </header>
                    </div>
                </div>
            </main>
        );
    }

    return (
        <main className="min-h-screen px-6 py-10">
            <div className="mx-auto w-full max-w-6xl">
                <div className="glass-card w-full rounded-3xl border border-white/70 bg-white/85 p-8 shadow-glow backdrop-blur-xl">
                    <div className="flex flex-col gap-6">
                        <header className="space-y-3 text-center">
                            <p className="text-xs font-semibold uppercase tracking-[0.3em] text-ink/60">
                                Backlog CLI
                            </p>
                            <h1 className="text-3xl font-semibold text-ink">管理画面</h1>
                            <div className="flex items-center justify-center gap-3">
                                <span className="text-sm text-ink/70">
                                    {session?.user?.name}
                                </span>
                                <a
                                    href={`/portal/${encodeURIComponent(name ?? "")}`}
                                    className="text-xs font-medium text-brand hover:text-brand-strong"
                                >
                                    ポータルに戻る
                                </a>
                            </div>
                        </header>

                        {authError && (
                            <div className="space-y-3">
                                <div className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700">
                                    {authError}
                                </div>
                                <div className="flex justify-center">
                                    <button
                                        type="button"
                                        className="inline-flex items-center gap-2 rounded-full border border-brand bg-brand px-6 py-3 text-sm font-semibold text-white shadow-sm transition-all hover:-translate-y-0.5 hover:shadow-md focus:outline-none focus:ring-2 focus:ring-brand/50 focus:ring-offset-2"
                                        onClick={handleReauth}
                                    >
                                        再認証
                                    </button>
                                </div>
                            </div>
                        )}

                        {!authError && (
                            <>
                                <div className="flex rounded-2xl border border-outline/60 bg-white/50 p-1">
                                    <button
                                        type="button"
                                        className={`flex-1 rounded-xl px-3 py-2 text-sm font-medium transition-colors ${
                                            tab === "audit"
                                                ? "bg-white text-ink shadow-sm"
                                                : "text-ink/60 hover:text-ink/80"
                                        }`}
                                        onClick={() => setTab("audit")}
                                    >
                                        監査ログ
                                    </button>
                                    <button
                                        type="button"
                                        className={`flex-1 rounded-xl px-3 py-2 text-sm font-medium transition-colors ${
                                            tab === "passphrase"
                                                ? "bg-white text-ink shadow-sm"
                                                : "text-ink/60 hover:text-ink/80"
                                        }`}
                                        onClick={() => setTab("passphrase")}
                                    >
                                        パスフレーズ管理
                                    </button>
                                </div>

                                {tab === "audit" ? (
                                    <AuditLogViewer
                                        tenantName={name ?? ""}
                                        onApiError={handleApiError}
                                    />
                                ) : (
                                    <PassphraseManagerView
                                        tenantName={name ?? ""}
                                        onApiError={handleApiError}
                                    />
                                )}
                            </>
                        )}
                    </div>
                </div>
            </div>
        </main>
    );
}
