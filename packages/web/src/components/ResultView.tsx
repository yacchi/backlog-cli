import { useEffect, useMemo, useState } from "react";

export type ResultType = "success" | "error" | "closed";

type ResultViewProps = {
  type: ResultType;
  message?: string;
};

const resultCopy: Record<
  ResultType,
  { title: string; body: string; accent: string }
> = {
  success: {
    title: "認証が完了しました",
    body: "ターミナルに戻って操作を続けてください。",
    accent: "text-emerald-600",
  },
  error: {
    title: "認証に失敗しました",
    body: "認証が完了できませんでした。内容をご確認ください。",
    accent: "text-rose-600",
  },
  closed: {
    title: "CLIが終了しました",
    body: "ターミナルで再度 backlog auth login を実行してください。",
    accent: "text-amber-600",
  },
};

function canAutoClose(): boolean {
  return (
    typeof (window as { forceCloseTab?: () => void }).forceCloseTab ===
    "function"
  );
}

export default function ResultView({ type, message }: ResultViewProps) {
  const [countdown, setCountdown] = useState(5);
  const [autoCloseEnabled, setAutoCloseEnabled] = useState(false);
  const copy = useMemo(() => resultCopy[type], [type]);

  useEffect(() => {
    if (type !== "success") return;
    if (!canAutoClose()) return;

    setAutoCloseEnabled(true);
    setCountdown(5);

    const timer = window.setInterval(() => {
      setCountdown((prev) => {
        if (prev <= 1) {
          const closer = (window as { forceCloseTab?: () => void })
            .forceCloseTab;
          closer?.();
          window.clearInterval(timer);
          return 0;
        }
        return prev - 1;
      });
    }, 1000);

    return () => window.clearInterval(timer);
  }, [type]);

  return (
    <div className="flex flex-col items-center gap-4 text-center">
      <div
        className={`flex h-16 w-16 items-center justify-center rounded-full border-2 ${copy.accent}`}
      >
        <span className="text-2xl">
          {type === "success" ? "✓" : type === "error" ? "✕" : "!"}
        </span>
      </div>
      <div>
        <h1 className="text-2xl font-semibold text-ink">{copy.title}</h1>
        <p className="mt-2 text-sm text-ink/70">{message || copy.body}</p>
      </div>
      {type === "success" ? (
        <div className="text-xs text-ink/60">
          {autoCloseEnabled ? (
            <p>このタブは {countdown} 秒後に自動で閉じられます。</p>
          ) : (
            <p>このタブは閉じて構いません。</p>
          )}
        </div>
      ) : null}
    </div>
  );
}
