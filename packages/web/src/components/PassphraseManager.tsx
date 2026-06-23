import { useState, useEffect, useCallback } from "react";
import Button from "./Button";

interface PassphraseInfo {
    hasPassphrase: boolean;
    passphrase?: string;
}

interface Props {
    tenantName: string;
    onApiError: (status: number, data: { error?: string }) => boolean;
}

export default function PassphraseManagerView({ tenantName, onApiError }: Props) {
    const [info, setInfo] = useState<PassphraseInfo | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [success, setSuccess] = useState<string | null>(null);
    const [showPassphrase, setShowPassphrase] = useState(false);
    const [newPassphrase, setNewPassphrase] = useState("");
    const [saving, setSaving] = useState(false);
    const [generating, setGenerating] = useState(false);
    const [confirmDelete, setConfirmDelete] = useState(false);
    const [deleting, setDeleting] = useState(false);

    const fetchInfo = useCallback(async () => {
        setLoading(true);
        setError(null);
        try {
            const resp = await fetch(
                `/api/v1/portal/${encodeURIComponent(tenantName)}/admin/passphrase`,
                { credentials: "same-origin" },
            );
            if (!resp.ok) {
                const data = await resp.json().catch(() => ({}));
                if (onApiError(resp.status, data)) return;
                throw new Error(data.error || "読み込みに失敗しました");
            }
            const data = await resp.json();
            setInfo(data);
        } catch (err) {
            setError(err instanceof Error ? err.message : "読み込みに失敗しました");
        } finally {
            setLoading(false);
        }
    }, [tenantName, onApiError]);

    useEffect(() => {
        fetchInfo();
    }, [fetchInfo]);

    const handleSet = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!newPassphrase || newPassphrase.length < 8) {
            setError("パスフレーズは8文字以上にしてください");
            return;
        }
        setSaving(true);
        setError(null);
        setSuccess(null);
        try {
            const resp = await fetch(
                `/api/v1/portal/${encodeURIComponent(tenantName)}/admin/passphrase`,
                {
                    method: "PUT",
                    credentials: "same-origin",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ passphrase: newPassphrase }),
                },
            );
            if (!resp.ok) {
                const data = await resp.json().catch(() => ({}));
                if (onApiError(resp.status, data)) return;
                throw new Error(data.error === "passphrase_too_short" ? "パスフレーズは8文字以上にしてください" : data.error || "設定に失敗しました");
            }
            setSuccess("パスフレーズを設定しました");
            setNewPassphrase("");
            await fetchInfo();
        } catch (err) {
            setError(err instanceof Error ? err.message : "設定に失敗しました");
        } finally {
            setSaving(false);
        }
    };

    const handleGenerate = async () => {
        setGenerating(true);
        setError(null);
        setSuccess(null);
        try {
            const resp = await fetch(
                `/api/v1/portal/${encodeURIComponent(tenantName)}/admin/passphrase/generate`,
                { method: "POST", credentials: "same-origin" },
            );
            if (!resp.ok) {
                const data = await resp.json().catch(() => ({}));
                if (onApiError(resp.status, data)) return;
                throw new Error(data.error || "生成に失敗しました");
            }
            const data = await resp.json();
            setSuccess(`パスフレーズを自動生成しました: ${data.passphrase}`);
            await fetchInfo();
        } catch (err) {
            setError(err instanceof Error ? err.message : "生成に失敗しました");
        } finally {
            setGenerating(false);
        }
    };

    const handleDelete = async () => {
        setDeleting(true);
        setError(null);
        setSuccess(null);
        try {
            const resp = await fetch(
                `/api/v1/portal/${encodeURIComponent(tenantName)}/admin/passphrase`,
                { method: "DELETE", credentials: "same-origin" },
            );
            if (!resp.ok) {
                const data = await resp.json().catch(() => ({}));
                if (onApiError(resp.status, data)) return;
                throw new Error(data.error || "削除に失敗しました");
            }
            setSuccess("パスフレーズを削除しました");
            setConfirmDelete(false);
            await fetchInfo();
        } catch (err) {
            setError(err instanceof Error ? err.message : "削除に失敗しました");
        } finally {
            setDeleting(false);
        }
    };

    if (loading) {
        return <p className="py-4 text-center text-sm text-ink/50">読み込み中...</p>;
    }

    return (
        <div className="space-y-5">
            {error && (
                <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
                    {error}
                </div>
            )}
            {success && (
                <div className="rounded-2xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">
                    {success}
                </div>
            )}

            {/* Current status */}
            <div className="rounded-2xl border border-outline/60 bg-white/50 p-4">
                <h3 className="mb-2 text-sm font-medium text-ink">現在の状態</h3>
                {info?.hasPassphrase ? (
                    <div className="space-y-2">
                        <p className="text-sm text-emerald-700">パスフレーズ: 設定済み</p>
                        {info.passphrase && (
                            <div className="flex items-center gap-2">
                                <code className="flex-1 rounded-lg bg-ink/5 px-3 py-2 text-sm break-all">
                                    {showPassphrase ? info.passphrase : "••••••••••••••••"}
                                </code>
                                <button
                                    type="button"
                                    className="shrink-0 rounded-lg border border-outline/60 bg-white px-3 py-2 text-xs font-medium text-ink/70 hover:bg-ink/5"
                                    onClick={() => setShowPassphrase(!showPassphrase)}
                                >
                                    {showPassphrase ? "隠す" : "表示"}
                                </button>
                                {showPassphrase && info.passphrase && (
                                    <button
                                        type="button"
                                        className="shrink-0 rounded-lg border border-outline/60 bg-white px-3 py-2 text-xs font-medium text-ink/70 hover:bg-ink/5"
                                        onClick={() => navigator.clipboard.writeText(info.passphrase!)}
                                    >
                                        Copy
                                    </button>
                                )}
                            </div>
                        )}
                    </div>
                ) : (
                    <p className="text-sm text-ink/50">パスフレーズ: 未設定</p>
                )}
            </div>

            {/* Set passphrase form */}
            <div className="rounded-2xl border border-outline/60 bg-white/50 p-4">
                <h3 className="mb-3 text-sm font-medium text-ink">パスフレーズの設定・変更</h3>
                <form onSubmit={handleSet} className="space-y-3">
                    <input
                        type="text"
                        value={newPassphrase}
                        onChange={(e) => setNewPassphrase(e.target.value)}
                        placeholder="新しいパスフレーズ（8文字以上）"
                        className="w-full rounded-xl border border-outline/60 bg-white px-3 py-2 text-sm focus:border-brand focus:outline-none focus:ring-2 focus:ring-brand/20"
                    />
                    <div className="flex items-center gap-3">
                        <Button type="submit" disabled={saving || !newPassphrase || newPassphrase.length < 8}>
                            {saving ? "設定中..." : "設定"}
                        </Button>
                        <Button variant="secondary" onClick={handleGenerate} disabled={generating}>
                            {generating ? "生成中..." : "自動生成"}
                        </Button>
                    </div>
                </form>
            </div>

            {/* Delete */}
            {info?.hasPassphrase && (
                <div className="rounded-2xl border border-rose-200 bg-rose-50/50 p-4">
                    <h3 className="mb-2 text-sm font-medium text-rose-700">パスフレーズの削除</h3>
                    <p className="mb-3 text-xs text-rose-600">
                        パスフレーズを削除すると、パスフレーズ認証によるポータルアクセスが無効になります。
                    </p>
                    {!confirmDelete ? (
                        <button
                            type="button"
                            className="rounded-full border border-rose-300 bg-white px-4 py-2 text-sm font-medium text-rose-600 hover:bg-rose-50"
                            onClick={() => setConfirmDelete(true)}
                        >
                            削除
                        </button>
                    ) : (
                        <div className="flex items-center gap-3">
                            <button
                                type="button"
                                className="rounded-full border border-rose-500 bg-rose-500 px-4 py-2 text-sm font-medium text-white hover:bg-rose-600"
                                onClick={handleDelete}
                                disabled={deleting}
                            >
                                {deleting ? "削除中..." : "本当に削除する"}
                            </button>
                            <button
                                type="button"
                                className="rounded-full border border-outline/60 bg-white px-4 py-2 text-sm font-medium text-ink/70 hover:bg-ink/5"
                                onClick={() => setConfirmDelete(false)}
                            >
                                キャンセル
                            </button>
                        </div>
                    )}
                </div>
            )}
        </div>
    );
}
