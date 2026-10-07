// api/types.ts — Payload types shared between the Wails bindings and the
// (standalone-dev) HTTP fallback. Mirrors the Go types in internal/domain and
// internal/service.

export interface Project {
  id: number
  name: string
  root_path: string
  level_override: number
  is_auto_grouped: boolean
  is_starred: boolean
  created_at: string
  repo_count: number
  total_added: number
  total_deleted: number
  my_added: number
  my_deleted: number
  my_files: number
  is_workday: boolean
  below_standard: boolean
}

export interface ProjectDetail {
  id: number
  name: string
  root_path: string
  level_override: number
  is_auto_grouped: boolean
  repos: RepoInfo[]
}

export interface RepoInfo {
  id: number
  path: string
  project_id: number
  last_scanned_at: string
  stats: DailyStat[]
  /** Browsable commit log for the repo's origin remote; '' when it has none. */
  web_url?: string
}

export interface DailyStat {
  id: number
  repository_id: number
  stat_date: string
  author: string
  files_changed: number
  lines_added: number
  lines_deleted: number
}

export interface Summary {
  date: string
  repo_count: number
  total_files: number
  total_added: number
  total_deleted: number
  my_added: number
  my_deleted: number
  my_files: number
  is_workday: boolean
}

export interface AppConfig {
  config: Record<string, string>
  scan_roots: string[]
}

export interface Todo {
  id: number
  project_id: number
  title: string
  completed: boolean
  priority: number
  sort_order: number
  created_at: string
  updated_at: string
}

export type NoteKind = 'knowledge' | 'log' | 'idea' | 'other'

export interface Note {
  id: number
  project_id: number
  title: string
  content: string
  tags: string
  kind: string
  pinned: boolean
  source: string
  sort_order: number
  created_at: string
  updated_at: string
}

// A note joined with its parent project, for the global knowledge hub.
export interface NoteWithProject extends Note {
  project_name: string
  root_path: string
}

export interface NoteVersion {
  id: number
  note_id: number
  title: string
  content: string
  tags: string
  kind: string
  created_at: string
}

export interface Tech {
  name: string
  category: string
}

export interface LanguageStat {
  language: string
  count: number
}

export interface Dependency {
  name: string
  version: string
  source: string
}

export interface TopContributor {
  author: string
  count: number
}

export interface ActivityStat {
  total_commits: number
  active_days: number
  last_commit_date: string
  commit_rate_30d: number
  active_months: number
}

export interface RecentCommit {
  time: string
  message: string
  author: string
  repo: string
  branch: string
}

export interface ProjectOverview {
  readme_excerpt: string
  tech_stack: Tech[]
  languages: LanguageStat[]
  dependencies: Dependency[]
  top_contributors: TopContributor[]
  activity: ActivityStat | null
  recent_commits: RecentCommit[]
  cached: boolean
}

export interface ImportResult {
  synced: number
  updated: number
  skipped: number
}

export interface SearchHit {
  type: 'note' | 'todo'
  id: number
  project_id: number
  project_name: string
  title: string
  snippet: string
  tags?: string
  updated_at: string
}

export interface TodoCount {
  project_id: number
  count: number
  total: number
}

export interface NoteCount {
  project_id: number
  count: number
}

export interface HeatmapDay {
  date: string
  lines_added: number
  lines_deleted: number
  commits: number
}

export interface HeatmapResponse {
  days: HeatmapDay[]
}

export interface StatusBarData {
  current_time: string
  last_commit_time: string
  last_commit_repo: string
  last_commit_branch: string
  last_commit_msg: string
}

export interface ScanStatus {
  running: boolean
  backfilling: boolean
  message: string
  progress: number
  total: number
}

export interface PluginStatus {
  name: string
  path: string
  loaded: boolean
  error?: string
}

export interface SourceStatus {
  name: string
  plugin: string
  enabled: boolean
}

export interface ImportRun {
  created: number
  updated: number
  skipped: number
}

// HandoffResult is what a session capture persists (matches Go service.HandoffResult).
export interface HandoffResult {
  note_id: number
  title: string
  tags: string
}
export interface ImportCompletedEvent {
  source: string
  created: number
  updated: number
  skipped: number
  error?: string
}
