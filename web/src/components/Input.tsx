import React from "react";

type InputProps = React.InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  helper?: string;
};

export default function Input({
  label,
  helper,
  className,
  ...props
}: InputProps) {
  return (
    <label className="flex flex-col gap-2 text-sm text-ink">
      <span className="text-sm font-semibold uppercase tracking-widest text-ink/70">
        {label}
      </span>
      <input
        className={[
          "rounded-2xl border border-outline bg-white/80 px-4 py-3 text-base text-ink shadow-sm outline-none transition focus:border-brand focus:ring-2 focus:ring-brand/30",
          className,
        ]
          .filter(Boolean)
          .join(" ")}
        {...props}
      />
      {helper ? <span className="text-xs text-ink/60">{helper}</span> : null}
    </label>
  );
}
