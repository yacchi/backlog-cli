export default function StatusIndicator({message}: {message: string}) {
  return (
    <div className="mt-6 flex items-center justify-center gap-3 text-sm text-ink/70">
      <span className="h-4 w-4 animate-spin rounded-full border-2 border-brand border-t-transparent" />
      <span>{message}</span>
    </div>
  )
}
