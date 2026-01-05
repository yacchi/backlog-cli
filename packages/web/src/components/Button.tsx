import React from "react";

type Variant = "primary" | "secondary";

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
};

export default function Button({
  variant = "primary",
  className,
  ...props
}: ButtonProps) {
  const base =
    "inline-flex items-center justify-center rounded-full px-6 py-3 text-sm font-semibold tracking-wide transition duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2";
  const variants: Record<Variant, string> = {
    primary:
      "bg-brand text-white shadow-lg shadow-emerald-500/30 hover:-translate-y-0.5 hover:bg-brand-strong focus-visible:ring-emerald-300",
    secondary:
      "border border-outline bg-white text-ink hover:-translate-y-0.5 hover:border-brand hover:text-brand focus-visible:ring-brand",
  };

  return (
    <button
      className={[base, variants[variant], className].filter(Boolean).join(" ")}
      {...props}
    />
  );
}
