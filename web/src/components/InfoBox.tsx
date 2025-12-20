export default function InfoBox({label, value}: {label: string; value: string}) {
  return (
    <div className="rounded-2xl border border-outline/60 bg-white/70 px-4 py-3 text-left shadow-sm">
      <div className="text-xs font-semibold uppercase tracking-widest text-ink/60">
        {label}
      </div>
      <div className="mt-1 text-base font-medium text-ink break-all">{value}</div>
    </div>
  )
}
