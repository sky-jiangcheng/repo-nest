import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Line } from 'react-chartjs-2'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Title,
} from 'chart.js'
import { cssVar } from '../utils/theme'
import { useTheme } from '../hooks/useTheme'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Title)

export interface TrendDataset {
  label: string
  data: number[]
  /** Token name (e.g. '--success') resolved against the active theme. */
  color: string
}

interface Props {
  labels: string[]
  datasets: TrendDataset[]
}

function TrendChart({ labels, datasets }: Props) {
  const { t } = useTranslation()
  // Canvas paints with resolved values, not CSS — so a theme flip needs a
  // re-render to repaint the axes, grid and tooltip in the new palette.
  useTheme()

  const { data, options } = useMemo(() => {
    const border = cssVar('--border-subtle')
    const text = cssVar('--text-tertiary')
    const textStrong = cssVar('--text-secondary')
    const grid = cssVar('--border-subtle')
    const tooltipBg = cssVar('--bg-secondary')

    const resolved = datasets.map((ds) => ({
      ...ds,
      // The token resolves to the light or dark value; fall back to the raw
      // value if a caller passes a literal colour instead of a token.
      borderColor: ds.color.startsWith('--') ? cssVar(ds.color) || '#4a7d4a' : ds.color,
    }))

    return {
      data: {
        labels,
        datasets: resolved.map((ds) => ({
          ...ds,
          backgroundColor: ds.borderColor + '20',
          tension: 0.2,
          pointRadius: 2,
          pointHoverRadius: 4,
        })),
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: {
            position: 'top' as const,
            align: 'end' as const,
            labels: {
              color: textStrong,
              // Chart.js's default 16px legend type is heavy at this size and
              // the colour swatch plus label already carry the meaning.
              boxWidth: 8,
              boxHeight: 8,
              usePointStyle: true,
              pointStyle: 'circle' as const,
              font: { size: 12 },
            },
          },
          tooltip: {
            backgroundColor: tooltipBg,
            titleColor: textStrong,
            bodyColor: textStrong,
            borderColor: border,
            borderWidth: 1,
            cornerRadius: 6,
            padding: 10,
            // Match the app's surface elevation so the tooltip doesn't read
            // as a foreign layer pasted over the chart.
            boxPadding: 4,
          },
        },
        scales: {
          y: {
            beginAtZero: true,
            // Horizontal-only grid: vertical lines add noise without carrying
            // a value, and this is an editorial layout, not a dense plot.
            grid: { color: grid, drawTicks: false },
            border: { display: false },
            ticks: {
              color: text,
              padding: 8,
              font: { size: 11 },
              maxTicksLimit: 5,
            },
            title: {
              display: true,
              text: t('trend.count', { defaultValue: 'Count' }),
              color: text,
              font: { size: 11 },
            },
          },
          x: {
            grid: { display: false },
            border: { color: border },
            ticks: {
              color: text,
              padding: 8,
              font: { size: 11 },
              maxTicksLimit: 8,
              maxRotation: 0,
              autoSkipPadding: 12,
            },
            title: {
              display: true,
              text: t('summaryBar.date', { defaultValue: 'Date' }),
              color: text,
              font: { size: 11 },
            },
          },
        },
      },
    }
  }, [labels, datasets, t])

  return (
    <div className="chart-container" style={{ height: 220 }}>
      <Line data={data} options={options} className="chart-simple" />
    </div>
  )
}

export default TrendChart
