export default function WarningBox({title, children}: {title: string; children: string}) {
  return (
    <div className="rounded-2xl border border-amber-200 bg-amber-50/80 px-4 py-3 text-left text-sm text-amber-900">
      <div className="text-xs font-semibold uppercase tracking-widest text-amber-700">
        {title}
      </div>
      <p className="mt-2 leading-relaxed">{children}</p>
    </div>
  )
}
