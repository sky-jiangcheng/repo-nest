import { useMemo } from 'react'
import s from './DiffViewer.module.css'

/**
 * Colour one unified-diff text: '+' → green, '-' → red, anything else → context.
 *
 * Content-agnostic on purpose — it only knows the +/- line convention, not what
 * a version is. Two screens share it: the note editor's VersionHistoryPanel and
 * the review page's wiki page history (ADR-0021), because the backend deliberately
 * returns the same `diff.Lines` text from DiffNoteVersions and DiffWikiPageVersion.
 * One renderer is what keeps those two from drifting apart on the +/- direction.
 */
export default function DiffViewer({ text }: { text: string }) {
  const lines = useMemo(() => text.split('\n'), [text])

  return (
    <div className={s.diffPanel}>
      <pre className={s.diffPre}>
        {lines.map((line, i) => {
          if (line.startsWith('+')) {
            return <code key={i} className={s.diffLineAdd}>{line}</code>
          }
          if (line.startsWith('-')) {
            return <code key={i} className={s.diffLineDel}>{line}</code>
          }
          return <code key={i}>{line}</code>
        })}
      </pre>
    </div>
  )
}
