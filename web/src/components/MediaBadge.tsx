import type { MediaType } from '../api/client'

export default function MediaBadge({ type }: { type?: MediaType }) {
  const audiobook = type === 'audiobook'
  const both = type === 'both'
  const label = both ? 'Both' : audiobook ? 'Audiobook' : 'Ebook'
  const icon = both ? '📖🎧' : audiobook ? '🎧' : '📖'
  const cls = both
    ? 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300'
    : audiobook
      ? 'bg-indigo-100 text-indigo-800 dark:bg-indigo-950 dark:text-indigo-300'
      : 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
  return (
    <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium ${cls}`}>
      <span aria-hidden>{icon}</span>
      {label}
    </span>
  )
}
