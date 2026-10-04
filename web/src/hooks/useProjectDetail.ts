import { useState, useEffect, useMemo, useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { getProjectDetail, getProjectOverview, updateProjectLevel } from '../api/client'
import type { ProjectDetail, ProjectOverview } from '../api/client'
import type { TrendDataset } from '../components/TrendChart'

/** Aggregated daily stat for trend chart. */
interface DailyStat {
  added: number
  deleted: number
  files: number
  commits: number
}

export function useProjectDetail(id: string | undefined) {
  const { t } = useTranslation()

  const [project, setProject] = useState<ProjectDetail | null>(null)
  const [overview, setOverview] = useState<ProjectOverview | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [scope, setScope] = useState<'week' | 'month' | 'all'>('week')

  const load = useCallback(() => {
    if (!id) return
    // Sequence guard: /project/:id reuses this component instance across
    // navigation, so a slow response for the previous project id would
    // otherwise overwrite the new project's page with the old project's data.
    const seq = ++loadSeqRef.current
    const current = () => loadSeqRef.current === seq
    setLoading(true)
    setError('')
    getProjectDetail(Number(id))
      .then(p => {
        if (!current()) return
        setProject(p)
        // The overview is nested inside detail's chain: it needs the same
        // guard, or a late overview could still land after the next load.
        getProjectOverview(Number(id))
          .then(ov => { if (current()) setOverview(ov) })
          .catch(() => {})
      })
      .catch((e) => { if (current()) setError(e instanceof Error ? e.message : t('common.failed')) })
      .finally(() => { if (current()) setLoading(false) })
  }, [id, t])

  // Keeps the token for the most recent load; anything older is discarded.
  const loadSeqRef = useRef(0)

  // Initial + on-`id`-change data load. Synchronous setState (loading/error
  // reset) is intentional here; matches the established set-state-in-effect
  // disable pattern used across the codebase.
  useEffect(() => { load() }, [load]) // eslint-disable-line react-hooks/set-state-in-effect

  const handleLevelChange = async (direction: 'up' | 'down') => {
    if (!id) return
    try {
      await updateProjectLevel(Number(id), direction)
      const updated = await getProjectDetail(Number(id))
      setProject(updated)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('common.failed'))
    }
  }

  const stats = useMemo(() => {
    const map = new Map<string, DailyStat>()
    if (project?.repos) {
      project.repos.forEach((repo) => {
        repo.stats?.forEach((stat) => {
          const cur = map.get(stat.stat_date) || { added: 0, deleted: 0, files: 0, commits: 0 }
          cur.added += stat.lines_added
          cur.deleted += stat.lines_deleted
          cur.files += stat.files_changed
          cur.commits++
          map.set(stat.stat_date, cur)
        })
      })
    }
    return map
  }, [project])

  const trendData = useMemo(() => {
    let dates = Array.from(stats.keys()).sort()
    if (scope === 'week') {
      const weekDates = new Set(getLastDays(7))
      dates = dates.filter((d) => weekDates.has(d))
    } else if (scope === 'month') {
      const monthDates = new Set(getLastDays(30))
      dates = dates.filter((d) => monthDates.has(d))
    }

    return {
      labels: dates,
      datasets: [
        { label: t('dashboard.sortMyAdded', { defaultValue: 'Lines Added' }), data: dates.map((d) => stats.get(d)!.added), color: '--success' },
        { label: t('project.deletedLines'), data: dates.map((d) => stats.get(d)!.deleted), color: '--danger' },
        { label: t('dashboard.sortMyFiles', { defaultValue: 'Files Changed' }), data: dates.map((d) => stats.get(d)!.files), color: '--info' },
      ] as TrendDataset[],
    }
  }, [stats, scope, t])

  const totals = useMemo(() => {
    let added = 0, deleted = 0, files = 0, active = 0
    stats.forEach((v) => {
      added += v.added
      deleted += v.deleted
      files += v.files
      if (v.added + v.deleted > 0) active++
    })
    return { added, deleted, files, active, repos: project?.repos?.length || 0 }
  }, [stats, project])

  return {
    project,
    overview,
    loading,
    error,
    setError,
    scope,
    setScope,
    trendData,
    totals,
    retry: load,
    handleLevelChange,
  }
}

function getLastDays(n: number): string[] {
  const result: string[] = []
  const now = new Date()
  for (let i = n - 1; i >= 0; i--) {
    const d = new Date(now)
    d.setDate(d.getDate() - i)
    result.push(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`)
  }
  return result
}
