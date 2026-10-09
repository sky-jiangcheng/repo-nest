import { useTranslation } from 'react-i18next'
import { useCallback, useEffect, useRef, useState } from 'react'
import Icon from './Icon'
import s from './BlockEditor.module.css'

// EDITOR-FROZEN: do not add new block types per ADR-0006 (scope freeze).
// Current block types: paragraph, heading, code, blockquote, callout, list, table, math, hr, other.
// This set is complete; any new structured blocks must go through an ADR first.
//
// BlockEditor is a lightweight block-based Markdown editor (gitbook-gap #19).
//
// - Content is split into blocks (paragraph/heading/code/callout/list/table/math…)
// - Each block is edited inline; blocks reorder via drag or arrows
// - Typing "/" opens a block palette to insert structured blocks
// - The stored format stays plain Markdown: joinBlocks() reassembles the text

type BlockType =
  | 'paragraph'
  | 'heading'
  | 'code'
  | 'blockquote'
  | 'callout'
  | 'list'
  | 'table'
  | 'math'
  | 'hr'
  | 'other'

interface Block {
  id: string
  type: BlockType
  text: string
}

interface BlockEditorProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
}

interface PaletteItem {
  label: string
  desc: string
  type: BlockType
  insert: () => string
}

// Built per-render from the active locale so labels, syntax hints and the
// inserted sample content all follow the UI language.
type TFunc = (key: string, opts?: Record<string, unknown>) => string

function buildPaletteItems(t: TFunc): PaletteItem[] {
  const s = (k: string) => t(`blockEditor.samples.${k}`)
  return [
    { label: t('blockEditor.paragraph'), desc: t('blockEditor.desc.plainText'), type: 'paragraph', insert: () => s('paragraph') },
    { label: t('blockEditor.heading'), desc: t('blockEditor.desc.heading'), type: 'heading', insert: () => s('heading') },
    { label: t('blockEditor.callout'), desc: '> [!TIP]', type: 'callout', insert: () => `> [!TIP]\n> ${s('tipBody')}` },
    { label: t('blockEditor.calloutWarn'), desc: '> [!WARNING]', type: 'callout', insert: () => `> [!WARNING]\n> ${s('warnBody')}` },
    { label: t('blockEditor.calloutNote'), desc: '> [!NOTE]', type: 'callout', insert: () => `> [!NOTE]\n> ${s('noteBody')}` },
    { label: t('blockEditor.code'), desc: t('blockEditor.desc.code'), type: 'code', insert: () => `\`\`\`js\n${s('codeComment')}\n\`\`\`` },
    { label: t('blockEditor.mermaid'), desc: t('blockEditor.desc.mermaidFlow'), type: 'code', insert: () => '```mermaid\nflowchart LR\n  A --> B\n```' },
    { label: t('blockEditor.math'), desc: '$$ E=mc^2 $$', type: 'math', insert: () => '$$\nE=mc^2\n$$' },
    { label: t('blockEditor.todo'), desc: t('blockEditor.desc.todoItem'), type: 'list', insert: () => `- [ ] ${s('todoItem')}` },
    { label: t('blockEditor.table'), desc: t('blockEditor.desc.tableCols'), type: 'table', insert: () => `| ${s('tableCell')} 1 | ${s('tableCell')} 2 |\n| --- | --- |\n| ${s('tableCell')} | ${s('tableCell')} |` },
    { label: t('blockEditor.details'), desc: t('blockEditor.desc.detailsFold'), type: 'other', insert: () => `<details>\n<summary>${s('detailsSummary')}</summary>\n\n${s('detailsBody')}\n\n</details>` },
    { label: t('blockEditor.tabs'), desc: t('blockEditor.desc.tabsMulti'), type: 'other', insert: () => `{% tabs %}\n{% tab title="${s('tabTitle')} 1" %}\n${s('tabBody')}\n{% endtab %}\n{% tab title="${s('tabTitle')} 2" %}\n${s('tabBody')}\n{% endtab %}\n{% endtabs %}` },
    { label: t('blockEditor.hr'), desc: t('blockEditor.desc.hrSplit'), type: 'hr', insert: () => '---' },
  ]
}

let blockSeq = 0
function newId(): string {
  blockSeq += 1
  return `blk-${Date.now().toString(36)}-${blockSeq}`
}

function detectType(text: string): BlockType {
  const firstLine = text.split('\n')[0].trim()
  if (/^> \[!\w+\]/.test(firstLine)) return 'callout'
  if (/^#{1,6}\s+/.test(firstLine)) return 'heading'
  if (/^```|^~~~/.test(firstLine)) return 'code'
  if (/^>\s?/.test(firstLine)) return 'blockquote'
  if (/^(\s*[-*+] |\s*\d+\. )/.test(firstLine)) return 'list'
  if (/^(\*\*\*|---|___)$/.test(firstLine)) return 'hr'
  if (/^\$\$/.test(firstLine)) return 'math'
  if (/^\|.*\|/.test(firstLine) || text.includes('\n---\n')) return 'table'
  return 'paragraph'
}

function splitBlocks(src: string): Block[] {
  const lines = src.split('\n')
  const blocks: Block[] = []
  let cur: string[] = []
  let i = 0

  const flush = () => {
    const text = cur.join('\n').trim()
    if (text) blocks.push({ id: newId(), type: detectType(text), text })
    cur = []
  }

  while (i < lines.length) {
    const line = lines[i]
    if (/^ {0,3}(```+|~~~+)/.test(line) && cur.length === 0) {
      const buf = [line]
      i += 1
      while (i < lines.length && !/^ {0,3}(```+|~~~+)\s*$/.test(lines[i])) {
        buf.push(lines[i])
        i += 1
      }
      if (i < lines.length) { buf.push(lines[i]); i += 1 }
      blocks.push({ id: newId(), type: 'code', text: buf.join('\n') })
      continue
    }
    if (line.trim() === '') {
      flush()
    } else {
      cur.push(line)
    }
    i += 1
  }
  flush()
  return blocks
}

function joinBlocks(blocks: Block[]): string {
  return blocks.map(b => b.text.trim()).filter(Boolean).join('\n\n')
}

function typeLabel(t: TFunc, type: BlockType): string {
  return t(`blockEditor.typeLabels.${type}`)
}

interface PaletteState {
  blockId: string
  slashIndex: number
  active: number
}

export default function BlockEditor({ value, onChange, placeholder }: BlockEditorProps) {
  const { t } = useTranslation()
  const [blocks, setBlocks] = useState<Block[]>(() => splitBlocks(value))
  const [palette, setPalette] = useState<PaletteState | null>(null)
  const [draggingIndex, setDraggingIndex] = useState<number | null>(null)
  const lastEmitted = useRef(value)
  const textareaRefs = useRef<Map<string, HTMLTextAreaElement>>(new Map())
  const paletteItems = buildPaletteItems(t)

  // Re-split when the external value changes (e.g. draft reset / note switch),
  // but never echo back our own emissions.
  useEffect(() => {
    if (value !== lastEmitted.current) {
      lastEmitted.current = value
      setBlocks(splitBlocks(value))
    }
  }, [value])

  const emit = useCallback((next: Block[]) => {
    lastEmitted.current = joinBlocks(next)
    onChange(lastEmitted.current)
  }, [onChange])

  const updateBlock = (id: string, text: string) => {
    setBlocks(prev => {
      const next = prev.map(b => b.id === id ? { ...b, text, type: detectType(text) } : b)
      emit(next)
      return next
    })
  }

  const insertTemplate = (item: PaletteItem) => {
    if (!palette) return
    const template = item.insert()
    setBlocks(prev => {
      const next = prev.map(b => {
        if (b.id !== palette.blockId) return b
        const caret = textareaRefs.current.get(b.id)?.selectionStart ?? palette.slashIndex + 1
        const before = b.text.slice(0, palette.slashIndex)
        const after = b.text.slice(caret)
        const text = (before + template + after).trim()
        return { ...b, text, type: detectType(text) }
      })
      emit(next)
      return next
    })
    const currentId = palette.blockId
    setPalette(null)
    // Place the caret at the end of the inserted template.
    requestAnimationFrame(() => {
      const el = textareaRefs.current.get(currentId)
      if (el) {
        el.focus()
        const pos = el.value.length
        el.setSelectionRange(pos, pos)
      }
    })
  }

  const moveBlock = (index: number, dir: -1 | 1) => {
    setBlocks(prev => {
      const target = index + dir
      if (target < 0 || target >= prev.length) return prev
      const next = [...prev]
      ;[next[index], next[target]] = [next[target], next[index]]
      emit(next)
      return next
    })
  }

  const deleteBlock = (index: number) => {
    setBlocks(prev => {
      if (prev.length <= 1) return prev
      const next = prev.filter((_, idx) => idx !== index)
      emit(next)
      return next
    })
  }

  const addBlockAfter = (index: number) => {
    setBlocks(prev => {
      const next = [...prev]
      next.splice(index + 1, 0, { id: newId(), type: 'paragraph', text: '' })
      emit(next)
      return next
    })
  }

  const handleDragStart = (index: number) => setDraggingIndex(index)
  const handleDragOver = (e: React.DragEvent) => e.preventDefault()
  const handleDrop = (index: number) => {
    if (draggingIndex === null || draggingIndex === index) return
    setBlocks(prev => {
      const next = [...prev]
      const [moved] = next.splice(draggingIndex, 1)
      next.splice(index, 0, moved)
      emit(next)
      return next
    })
    setDraggingIndex(null)
  }

  const handleTextareaKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>, id: string) => {
    if (palette && palette.blockId === id) {
      if (e.key === 'ArrowDown') { e.preventDefault(); setPalette(p => p ? { ...p, active: (p.active + 1) % paletteItems.length } : p) }
      else if (e.key === 'ArrowUp') { e.preventDefault(); setPalette(p => p ? { ...p, active: (p.active - 1 + paletteItems.length) % paletteItems.length } : p) }
      else if (e.key === 'Enter') { e.preventDefault(); insertTemplate(paletteItems[palette.active]) }
      else if (e.key === 'Escape') { e.preventDefault(); setPalette(null) }
      else { setPalette(null) }
      return
    }
    if (e.key === 'Escape') { setPalette(null); return }
    if (e.key === 'Tab' && e.target instanceof HTMLTextAreaElement) {
      e.preventDefault()
      const el = e.target
      const start = el.selectionStart
      const end = el.selectionEnd
      const newText = el.value.slice(0, start) + '  ' + el.value.slice(end)
      updateBlock(id, newText)
      requestAnimationFrame(() => { el.setSelectionRange(start + 2, start + 2) })
    }
  }

  const handleTextareaChange = (e: React.ChangeEvent<HTMLTextAreaElement>, id: string) => {
    const el = e.target
    const next = el.value
    const caret = el.selectionStart
    // Opening "/" triggers the block palette (a lone slash at line start or after a space).
    const charBefore = caret > 1 ? next[caret - 2] : ''
    if (caret > 0 && next[caret - 1] === '/' && (caret === 1 || charBefore === ' ' || charBefore === '\n')) {
      setPalette({ blockId: id, slashIndex: caret - 1, active: 0 })
    } else if (palette && palette.blockId === id) {
      setPalette(null)
    }
    updateBlock(id, next)
  }

  if (blocks.length === 0) {
    return (
      <div className={`${s.blockEditor} ${s.blockEditorEmpty}`}>
        <textarea
          value=""
          onChange={(e) => {
            if (e.target.value) {
              const b: Block = { id: newId(), type: 'paragraph', text: e.target.value }
              const next = [b]
              lastEmitted.current = e.target.value
              onChange(e.target.value)
              setBlocks(next)
              requestAnimationFrame(() => {
                textareaRefs.current.get(b.id)?.focus()
              })
            }
          }}
          placeholder={placeholder ?? t('project.contentBlockPlaceholder', { defaultValue: 'Type content or / for blocks' })}
          className={`form-input ${s.blockInput}`}
          rows={3}
        />
      </div>
    )
  }

  return (
    <div className={s.blockEditor}>
      {blocks.map((block, index) => (
        <div
          key={block.id}
          className={`${s.blockItem} ${draggingIndex === index ? s.blockItemDragging : ''}`}
          draggable={false}
        >
          <div className={s.blockGutter}>
            <span
              className={s.blockDragHandle}
              draggable
              title={t('blockEditor.dragSort')}
              onDragStart={() => handleDragStart(index)}
              onDragOver={handleDragOver}
              onDrop={() => handleDrop(index)}
              onDragEnd={() => setDraggingIndex(null)}
            >
              <Icon name="grip" size={14} />
            </span>
            <span className={s.blockType}>{typeLabel(t, block.type)}</span>
            <div className={s.blockActions}>
              <button type="button" className={s.blockBtn} title={t('blockEditor.moveUp')} aria-label={t('blockEditor.moveUp')} onClick={() => moveBlock(index, -1)} disabled={index === 0}><Icon name="arrow-up" size={14} /></button>
              <button type="button" className={s.blockBtn} title={t('blockEditor.moveDown')} aria-label={t('blockEditor.moveDown')} onClick={() => moveBlock(index, 1)} disabled={index === blocks.length - 1}><Icon name="arrow-down" size={14} /></button>
              <button type="button" className={s.blockBtn} title={t('blockEditor.insertAfter')} aria-label={t('blockEditor.insertAfter')} onClick={() => addBlockAfter(index)}><Icon name="plus" size={14} /></button>
              <button type="button" className={`${s.blockBtn} ${s.blockBtnDanger}`} title={t('blockEditor.deleteBlock')} aria-label={t('blockEditor.deleteBlock')} onClick={() => deleteBlock(index)}><Icon name="close" size={14} /></button>
            </div>
          </div>
          <textarea
            ref={(el) => { if (el) textareaRefs.current.set(block.id, el); else textareaRefs.current.delete(block.id) }}
            value={block.text}
            onChange={(e) => handleTextareaChange(e, block.id)}
            onKeyDown={(e) => handleTextareaKeyDown(e, block.id)}
            rows={Math.max(1, block.text.split('\n').length)}
            className={`form-input ${s.blockInput}`}
            placeholder={placeholder ?? t('project.contentBlockPlaceholder', { defaultValue: 'Type content or / for blocks' })}
          />
          {palette && palette.blockId === block.id && (
            <div className={s.blockPalette} role="listbox" aria-label={t('blockEditor.insert')}>
              <div className={s.blockPaletteHead}>{t('blockEditor.insert')}</div>
              {paletteItems.map((item, i) => (
                <button
                  key={item.label}
                  type="button"
                  role="option"
                  aria-selected={palette.active === i}
                  className={`${s.blockPaletteItem} ${palette.active === i ? 'active' : ''}`}
                  onMouseEnter={() => setPalette(p => p ? { ...p, active: i } : p)}
                  onClick={() => insertTemplate(item)}
                >
                  <span className={s.blockPaletteLabel}>{item.label}</span>
                  <span className={s.blockPaletteDesc}>{item.desc}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      ))}
      <button type="button" className={s.blockAdd} onClick={() => addBlockAfter(blocks.length - 1)}><Icon name="plus" size={14} /> {t('blockEditor.addBlock')}</button>
    </div>
  )
}
