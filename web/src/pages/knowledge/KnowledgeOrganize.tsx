import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  listOrganizeGroups, reassignLandingNotes, deleteLandingProject, getProjects,
  type OrganizeGroup, type Project,
} from '../../api/client'
import { useConfirmClick } from '../../hooks/useConfirmClick'
import ErrorBanner from '../../components/ErrorBanner'
import Icon from '../../components/Icon'
import s from './KnowledgeOrganize.module.css'

// The knowledge-base organize surface (ADR-0023). Import placeholders and the
// default "Uncategorized" bucket collect notes that arrived from external
// knowledge without a matching project; this view lets a person re-file them
// into a real project (or the default) and remove an emptied placeholder — the
// whole point being that nothing is silently lost anymore, but filing stays a
// human act. Renders nothing when there is nothing to organize.
function KnowledgeOrganize() {
  const { t } = useTranslation()
  const [groups, setGroups] = useState<OrganizeGroup[]>([])
  const [realProjects, setRealProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState<number | null>(null)
  // Per-group chosen reassign target; 0 = none selected.
  const [targets, setTargets] = useState<Record<number, number>>({})

  const load = useCallback(() => {
    // State changes happen only in the promise callbacks, never synchronously in
    // the effect that calls this (react-hooks/set-state-in-effect) — the same
    // shape as NoteSection's fetchNotes.
    Promise.all([listOrganizeGroups(), getProjects(undefined, false)])
      .then(([gs, ps]) => {
        setGroups(gs)
        setRealProjects(ps.filter(p => p.project_kind === 'scanned'))
        setError('')
      })
      .catch(e => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  // Nothing awaiting the user: no landing buckets at all, or only an empty
  // default bucket (the default is never shown by itself — it is the catch-all).
  const worthShowing = groups.some(
    g => g.kind === 'import_placeholder' || (g.kind === 'default' && g.note_count > 0),
  )
  if (!loading && !worthShowing) {
    return null
  }

  const defaultGroup = groups.find(g => g.kind === 'default')

  // Reassign targets for a source bucket: the scanner-backed projects, plus the
  // default bucket for an import placeholder (un-filing into "Uncategorized").
  // The source itself and any other placeholder are never offered.
  const optionsFor = (g: OrganizeGroup): Array<{ id: number; name: string }> => {
    const opts = realProjects.map(p => ({ id: p.id, name: p.name }))
    if (g.kind !== 'default' && defaultGroup && defaultGroup.project_id !== g.project_id) {
      opts.push({ id: defaultGroup.project_id, name: t('project.kindDefault', { defaultValue: '未分类' }) })
    }
    return opts.filter(o => o.id !== g.project_id)
  }

  const doReassign = async (g: OrganizeGroup) => {
    const target = targets[g.project_id]
    if (!target) return
    setBusy(g.project_id)
    try {
      await reassignLandingNotes(g.project_id, target)
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(null)
    }
  }

  const doDelete = async (g: OrganizeGroup) => {
    setBusy(g.project_id)
    try {
      // deleteNotes=true only when the bucket still holds notes (confirmed path);
      // an emptied placeholder is dropped without the flag.
      await deleteLandingProject(g.project_id, g.note_count > 0)
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(null)
    }
  }

  if (loading && groups.length === 0) {
    return <div className="skeleton skeleton-text" style={{ height: 60 }} />
  }

  return (
    <div className={`knowledge-section ${s.section}`}>
      <div className="section-header">
        <h2><Icon name="check" size={15} /> {t('knowledge.organizeTitle', { defaultValue: '知识库整理' })}</h2>
      </div>
      <p className={s.intro}>
        {t('knowledge.organizeIntro', { defaultValue: '这些笔记导入时没有匹配到项目，先落在占位/未分类里。归口到真实项目，或删除空的占位。' })}
      </p>
      {error && <ErrorBanner message={error} onRetry={load} />}

      {groups.length === 0 ? (
        <p className="empty-hint">{t('knowledge.organizeEmpty', { defaultValue: '暂无待整理的笔记。' })}</p>
      ) : (
        <div className={s.list}>
          {groups.map(g => (
            <OrganizeRow
              key={g.project_id}
              group={g}
              options={optionsFor(g)}
              target={targets[g.project_id] ?? 0}
              busy={busy === g.project_id}
              onPickTarget={id => setTargets(prev => ({ ...prev, [g.project_id]: id }))}
              onReassign={() => void doReassign(g)}
              onDelete={() => void doDelete(g)}
              t={t}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function OrganizeRow({
  group, options, target, busy, onPickTarget, onReassign, onDelete, t,
}: {
  group: OrganizeGroup
  options: Array<{ id: number; name: string }>
  target: number
  busy: boolean
  onPickTarget: (id: number) => void
  onReassign: () => void
  onDelete: () => void
  t: (key: string, opts?: Record<string, unknown>) => string
}) {
  const isDefault = group.kind === 'default'
  const sourceGone = !isDefault && !group.repo_bound
  // Confirm-before-delete is deliberate (the user's standing rule): the armed
  // click turns into a second tap that actually removes the bucket + its notes.
  const { armed, click } = useConfirmClick(onDelete)

  return (
    <div className={s.row}>
      <div className={s.rowHead}>
        <span className={s.rowName} title={group.root_path}>{group.name}</span>
        <span className={`badge ${isDefault ? s.badgeDefault : s.badgePlaceholder}`}>
          {isDefault ? t('project.kindDefault', { defaultValue: '未分类' }) : t('project.kindPlaceholder', { defaultValue: '未归类仓库' })}
        </span>
        {sourceGone && (
          <span className={`badge ${s.badgeGone}`} title={t('knowledge.organizeSourceGoneTip', { defaultValue: '该仓库当前不在扫描结果里（可能已删除或尚未扫描）' })}>
            {t('knowledge.organizeSourceGone', { defaultValue: '源仓库不在' })}
          </span>
        )}
        <span className={s.rowCount}>
          {t('knowledge.organizeNoteCount', { n: group.note_count, defaultValue: '{{n}} 条笔记' })}
        </span>
      </div>

      <div className={s.rowActions}>
        {group.note_count > 0 && (
          <>
            <select
              className={`form-input ${s.select}`}
              value={target}
              disabled={busy || options.length === 0}
              onChange={e => onPickTarget(Number(e.target.value))}
            >
              <option value={0}>{t('knowledge.organizePickTarget', { defaultValue: '归口到…' })}</option>
              {options.map(o => <option key={o.id} value={o.id}>{o.name}</option>)}
            </select>
            <button
              className="btn btn-sm btn-primary"
              disabled={busy || !target}
              onClick={onReassign}
            >
              {t('knowledge.organizeReassign', { defaultValue: '归口' })}
            </button>
          </>
        )}
        {/* The default bucket can never be deleted; only placeholders (emptied or
            with notes, the latter confirmed) are removable. */}
        {!isDefault && (
          <button
            className={`btn btn-sm ${armed ? 'btn-delete-confirm' : 'btn-danger'}`}
            disabled={busy}
            onClick={click}
            title={group.note_count > 0
              ? t('knowledge.organizeDeleteWithNotes', { defaultValue: '连同其笔记一并删除' })
              : t('knowledge.organizeDeleteEmpty', { defaultValue: '删除空占位' })}
          >
            {armed ? t('project.confirmDelete') : t('common.delete')}
          </button>
        )}
      </div>
    </div>
  )
}

export default KnowledgeOrganize
