import React from 'react'

export default function Container({children}: {children: React.ReactNode}) {
  return (
    <div className="glass-card w-full max-w-3xl rounded-3xl border border-white/70 bg-white/85 p-8 shadow-glow backdrop-blur-xl sm:p-10">
      {children}
    </div>
  )
}
