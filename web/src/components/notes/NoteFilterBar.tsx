import type { KindFilter } from '../../types/kind'

interface NoteFilterBarProps {
  filter: KindFilter
  setFilter: (f: KindFilter) => void
  notesCount: number
  t: (key: string, params?: Record<string, unknown>) => string
}

/** Category chips — one per note kind. Hidden when there are no notes. */
export default function NoteFilterBar({ filter, setFilter, notesCount, t }: NoteFilterBarProps) {
  if (notesCount === 0) return null
  const kinds: KindFilter[] = ['all', 'knowledge', 'log', 'idea', 'other']
  return (
    <div className="note-filters">
      {kinds.map(k => (
        <button
          key={k}
          className={`filter-btn ${filter === k ? 'active' : ''}`}
          onClick={() => setFilter(k)}
        >
          {k === 'all' ? t('project.filterAll') : t(`project.kinds.${k}`)}
        </button>
      ))}
    </div>
  )
}
