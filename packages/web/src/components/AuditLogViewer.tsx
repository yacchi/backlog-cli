import { useState, useCallback } from "react";
import Button from "./Button";

interface AuditLogEntry {
    timestamp: string;
    action: string;
    result: string;
    userId?: string;
    userName?: string;
    userEmail?: string;
    clientIp?: string;
    userAgent?: string;
    space?: string;
    error?: string;
    requestId?: string;
    durationMs?: number;
}

interface Props {
    tenantName: string;
    onApiError: (status: number, data: { error?: string }) => boolean;
}

const ACTION_OPTIONS = [
    { value: "", label: "すべて" },
    { value: "portal_verify", label: "パスフレーズ検証" },
    { value: "portal_download", label: "バンドルダウンロード" },
    { value: "portal_provision", label: "プロビジョニング" },
    { value: "portal_oauth_start", label: "OAuth開始" },
    { value: "portal_oauth_login", label: "OAuthログイン" },
    { value: "portal_logout", label: "ログアウト" },
    { value: "auth_start", label: "CLI認証開始" },
    { value: "auth_callback", label: "CLI認証コールバック" },
    { value: "token_exchange", label: "トークン交換" },
    { value: "token_refresh", label: "トークン更新" },
    { value: "admin_passphrase_view", label: "パスフレーズ閲覧" },
    { value: "admin_passphrase_set", label: "パスフレーズ設定" },
    { value: "admin_passphrase_generate", label: "パスフレーズ生成" },
    { value: "admin_passphrase_clear", label: "パスフレーズ削除" },
    { value: "admin_audit_query", label: "監査ログ照会" },
];

const RESULT_OPTIONS = [
    { value: "", label: "すべて" },
    { value: "success", label: "成功" },
    { value: "error", label: "エラー" },
];

function formatTimestamp(ts: string): string {
    try {
        const d = new Date(ts);
        if (isNaN(d.getTime())) return ts;
        return d.toLocaleString();
    } catch {
        return ts;
    }
}

function formatAction(action: string): string {
    const found = ACTION_OPTIONS.find((o) => o.value === action);
    return found ? found.label : action;
}

export default function AuditLogViewer({ tenantName, onApiError }: Props) {
    const [entries, setEntries] = useState<AuditLogEntry[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [queried, setQueried] = useState(false);

    // Filter state
    const [hours, setHours] = useState("24");
    const [action, setAction] = useState("");
    const [userEmail, setUserEmail] = useState("");
    const [resultFilter, setResultFilter] = useState("");

    const handleQuery = useCallback(async () => {
        setLoading(true);
        setError(null);

        const now = new Date();
        const startTime = new Date(now.getTime() - parseInt(hours) * 3600 * 1000);
        const params = new URLSearchParams();
        params.set("start_time", startTime.toISOString());
        params.set("end_time", now.toISOString());
        if (action) params.set("action", action);
        if (userEmail.trim()) params.set("user_email", userEmail.trim());
        if (resultFilter) params.set("result", resultFilter);

        try {
            const resp = await fetch(
                `/api/v1/portal/${encodeURIComponent(tenantName)}/admin/audit?${params}`,
                { credentials: "same-origin" },
            );

            if (!resp.ok) {
                const data = await resp.json().catch(() => ({}));
                if (onApiError(resp.status, data)) return;
                throw new Error(data.error || "照会に失敗しました");
            }

            const data = await resp.json();
            setEntries(data.entries ?? []);
            setQueried(true);
        } catch (err) {
            setError(err instanceof Error ? err.message : "照会に失敗しました");
        } finally {
            setLoading(false);
        }
    }, [tenantName, hours, action, userEmail, resultFilter, onApiError]);

    return (
        <div className="space-y-4">
            {/* Filters */}
            <div className="grid grid-cols-2 gap-3">
                <div>
                    <label className="mb-1 block text-xs font-medium text-ink/60">期間</label>
                    <select
                        value={hours}
                        onChange={(e) => setHours(e.target.value)}
                        className="w-full rounded-xl border border-outline/60 bg-white px-3 py-2 text-sm focus:border-brand focus:outline-none focus:ring-2 focus:ring-brand/20"
                    >
                        <option value="1">過去1時間</option>
                        <option value="6">過去6時間</option>
                        <option value="24">過去24時間</option>
                        <option value="72">過去3日間</option>
                        <option value="168">過去7日間</option>
                    </select>
                </div>
                <div>
                    <label className="mb-1 block text-xs font-medium text-ink/60">アクション</label>
                    <select
                        value={action}
                        onChange={(e) => setAction(e.target.value)}
                        className="w-full rounded-xl border border-outline/60 bg-white px-3 py-2 text-sm focus:border-brand focus:outline-none focus:ring-2 focus:ring-brand/20"
                    >
                        {ACTION_OPTIONS.map((o) => (
                            <option key={o.value} value={o.value}>{o.label}</option>
                        ))}
                    </select>
                </div>
                <div>
                    <label className="mb-1 block text-xs font-medium text-ink/60">ユーザーメール</label>
                    <input
                        type="text"
                        value={userEmail}
                        onChange={(e) => setUserEmail(e.target.value)}
                        placeholder="user@example.com"
                        className="w-full rounded-xl border border-outline/60 bg-white px-3 py-2 text-sm focus:border-brand focus:outline-none focus:ring-2 focus:ring-brand/20"
                    />
                </div>
                <div>
                    <label className="mb-1 block text-xs font-medium text-ink/60">結果</label>
                    <select
                        value={resultFilter}
                        onChange={(e) => setResultFilter(e.target.value)}
                        className="w-full rounded-xl border border-outline/60 bg-white px-3 py-2 text-sm focus:border-brand focus:outline-none focus:ring-2 focus:ring-brand/20"
                    >
                        {RESULT_OPTIONS.map((o) => (
                            <option key={o.value} value={o.value}>{o.label}</option>
                        ))}
                    </select>
                </div>
            </div>

            <div className="flex justify-center">
                <Button onClick={handleQuery} disabled={loading}>
                    {loading ? "照会中..." : "照会"}
                </Button>
            </div>

            {error && (
                <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
                    {error}
                </div>
            )}

            {/* Results */}
            {queried && (
                <div className="space-y-2">
                    <p className="text-xs text-ink/50">{entries.length} 件</p>
                    {entries.length === 0 ? (
                        <p className="py-4 text-center text-sm text-ink/50">該当するログがありません</p>
                    ) : (
                        <div className="overflow-x-auto rounded-2xl border border-outline/60">
                            <table className="w-full text-left text-sm">
                                <thead>
                                    <tr className="border-b border-outline/40 bg-ink/5">
                                        <th className="whitespace-nowrap px-3 py-2 font-medium text-ink/70">日時</th>
                                        <th className="whitespace-nowrap px-3 py-2 font-medium text-ink/70">アクション</th>
                                        <th className="whitespace-nowrap px-3 py-2 font-medium text-ink/70">結果</th>
                                        <th className="whitespace-nowrap px-3 py-2 font-medium text-ink/70">ユーザー</th>
                                        <th className="whitespace-nowrap px-3 py-2 font-medium text-ink/70">IP</th>
                                        <th className="whitespace-nowrap px-3 py-2 font-medium text-ink/70">詳細</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {entries.map((entry, i) => (
                                        <tr key={i} className="border-b border-outline/20 last:border-0">
                                            <td className="whitespace-nowrap px-3 py-2 text-xs text-ink/70">{formatTimestamp(entry.timestamp)}</td>
                                            <td className="whitespace-nowrap px-3 py-2">{formatAction(entry.action)}</td>
                                            <td className="whitespace-nowrap px-3 py-2">
                                                <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${
                                                    entry.result === "success"
                                                        ? "bg-emerald-100 text-emerald-700"
                                                        : "bg-rose-100 text-rose-700"
                                                }`}>
                                                    {entry.result === "success" ? "成功" : "エラー"}
                                                </span>
                                            </td>
                                            <td className="whitespace-nowrap px-3 py-2 text-xs text-ink/70">{entry.userEmail || entry.userName || "-"}</td>
                                            <td className="whitespace-nowrap px-3 py-2 text-xs text-ink/70">{entry.clientIp || "-"}</td>
                                            <td className={`max-w-[200px] truncate px-3 py-2 text-xs ${entry.result === "error" ? "text-rose-600" : "text-ink/40"}`}>{entry.error || ""}</td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </div>
            )}
        </div>
    );
}
