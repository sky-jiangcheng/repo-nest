import { useTranslation } from 'react-i18next'
import type { ProjectOverview } from '../../api/client'
import { renderMarkdown } from '../../utils/markdown'
import s from './ProjectOverviewSection.module.css'

// 预留钩子类：`overview-empty` —— 目前全站没有对应样式定义（P35 复核确认），
// 保留在 markup 里作为将来挂样式的锚点，删留都不影响行为。

// tech.category 是自由字符串，只有 framework / tool 有专属配色，其余落回 techChip 基态。
const TECH_CLASS: Record<string, string | undefined> = {
  framework: s.techFramework,
  tool: s.techTool,
}

interface Props {
  overview: ProjectOverview
}

/**
 * The mined-knowledge panel of the project detail page: tech stack, language
 * breakdown, README excerpt, dependencies, contributors, activity and the
 * recent commit feed. Hidden entirely when nothing was mined yet.
 */
export default function ProjectOverviewSection({ overview }: Props) {
  const { t } = useTranslation()

  // Activity metrics and the recent-commit feed live in the Commits tab —
  // rendering them here too made Overview and Commits largely the same page.
  const hasContent =
    overview.readme_excerpt ||
    (overview.tech_stack?.length ?? 0) > 0 ||
    (overview.dependencies?.length ?? 0) > 0 ||
    (overview.top_contributors?.length ?? 0) > 0
  if (!hasContent) {
    return (
      <div className="detail-section overview-empty">
        <div className="section-header">
          <h2>{t('project.overview')}</h2>
          <span className={s.cacheHint}>{overview.cached ? t('project.fromCache') : t('project.realtimeMining')}</span>
        </div>
        <p className="empty-hint">{t('project.overviewEmpty')}：{t('project.overviewEmptyHint')}</p>
      </div>
    )
  }

  return (
      <div className={`detail-section ${s.section}`}>
      <div className="section-header">
        <h2>{t('project.overview')}</h2>
        <span className={s.cacheHint}>{overview.cached ? t('project.fromCache') : t('project.realtimeMining')}</span>
      </div>

      {(overview.tech_stack?.length ?? 0) > 0 && (
        <div className={s.tech}>
          {overview.tech_stack!.map(tech => (
            <span key={tech.name} className={`${s.techChip} ${TECH_CLASS[tech.category] ?? ''}`}>{tech.name}</span>
          ))}
        </div>
      )}

      {(overview.languages?.length ?? 0) > 0 && (
        <div className={s.langs}>
          {overview.languages!.map(l => {
            const max = overview.languages![0]?.count || 1
            return (
              <div key={l.language} className={s.langRow}>
                <span className={s.langName}>{l.language}</span>
                <div className={s.langBar}><div className={s.langFill} style={{ width: `${(l.count / max) * 100}%` }} /></div>
                <span className={s.langCount}>{l.count}</span>
              </div>
            )
          })}
        </div>
      )}

      {overview.readme_excerpt && (
        <div className={`${s.readme} markdown-body`} dangerouslySetInnerHTML={{ __html: renderMarkdown(overview.readme_excerpt) }} />
      )}

      {(overview.dependencies?.length ?? 0) > 0 && (
        <div className={s.dependencies}>
          <h4 className={s.subTitle}>{t('project.dependencies')}</h4>
          <div className={s.depsList}>
            {overview.dependencies!.slice(0, 20).map(d => (
              <span key={d.name} className={s.depChip}>
                <span className={s.depName}>{d.name}</span>
                <span className={s.depVersion}>{d.version}</span>
                <span className={s.depSource}>{d.source}</span>
              </span>
            ))}
          </div>
        </div>
      )}

      {(overview.top_contributors?.length ?? 0) > 0 && (
        <div className={s.contributors}>
          <h4 className={s.subTitle}>{t('project.topContributors')}</h4>
          <div className={s.contribList}>
            {overview.top_contributors!.map((c, i) => (
              <div key={i} className={s.contribItem}>
                <span className={s.contribRank}>#{i + 1}</span>
                <span className={s.contribName}>{c.author}</span>
                <span className={s.contribCount}>{c.count} {t('project.commitsUnit')}</span>
              </div>
            ))}
          </div>
        </div>
      )}

    </div>
  )
}
