import { useMemo } from 'react'
import { cssVar } from '../utils/theme'
import { useTheme } from '../hooks/useTheme'
import s from './GoalRing.module.css'

interface Props {
  value: number
  goal: number
  size?: number
  stroke?: number
  label?: string
  sublabel?: string
}

/**
 * Goal progress ring. The track and the progress arc both read their colour
 * from design tokens rather than literal hex, so the dark theme is painted
 * with its own palette instead of a light-mode ring on a dark surface.
 *
 * The arc's weight steps down as progress drops — full attention at the goal,
 * a quiet hint when barely started — which encodes the value before the number
 * is read.
 */
export default function GoalRing({ value, goal, size = 80, stroke = 7, label, sublabel }: Props) {
  useTheme()

  const { radius, circumference, offset, pct, arcColor, trackColor } = useMemo(() => {
    const v = value || 0
    const g = goal || 0
    const r = (size - stroke) / 2
    const c = 2 * Math.PI * r
    const ratio = g > 0 ? Math.min(v / g, 1) : 0
    const reached = v >= g && g > 0

    return {
      radius: r,
      circumference: c,
      offset: c * (1 - ratio),
      pct: Math.round(ratio * 100),
      // Reaching the goal is the only state that earns the strongest ink;
      // below it the arc recedes so the number carries the message.
      arcColor: reached
        ? cssVar('--accent')
        : ratio >= 0.5
          ? cssVar('--text-secondary')
          : cssVar('--text-tertiary'),
      trackColor: cssVar('--bg-tertiary'),
    }
  }, [value, goal, size, stroke])

  return (
    <div className={s.ring} style={{ width: size, height: size }}>
      <svg width={size} height={size} className={s.svg} aria-hidden="true">
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke={trackColor}
          strokeWidth={stroke}
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke={arcColor}
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          transform={`rotate(-90 ${size / 2} ${size / 2})`}
          className={s.progress}
        />
      </svg>
      <div className={s.center}>
        <span className={s.value} style={{ fontSize: Math.round(size * 0.26) }}>{pct}%</span>
        {label && <span className={s.label}>{label}</span>}
      </div>
      {sublabel && <span className={s.sublabel}>{sublabel}</span>}
    </div>
  )
}
