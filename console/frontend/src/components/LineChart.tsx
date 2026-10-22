import { useMemo, useState } from 'react'

export interface Series {
  label: string
  /** [unix seconds, value] */
  points: Array<[number, number]>
}

interface Props {
  series: Series[]
  format?: (v: number) => string
  height?: number
}

const COLORS = ['#0a0a0a', '#2563eb', '#16a34a', '#d97706', '#9333ea', '#dc2626']
const W = 640
const PAD = { l: 52, r: 12, t: 10, b: 22 }

export function LineChart({ series, format = (v) => String(Math.round(v * 100) / 100), height = 180 }: Props) {
  const [hover, setHover] = useState<number | null>(null)

  const { x0, x1, y1, ticks } = useMemo(() => {
    let lo = Infinity, hi = -Infinity, top = 0
    for (const s of series) for (const [t, v] of s.points) {
      lo = Math.min(lo, t); hi = Math.max(hi, t); top = Math.max(top, v)
    }
    const y = top > 0 ? top * 1.1 : 1
    return { x0: lo, x1: hi === lo ? lo + 1 : hi, y1: y, ticks: [0, 0.25, 0.5, 0.75, 1].map((f) => f * y) }
  }, [series])

  const all = series.flatMap((s) => s.points)
  if (all.length === 0) return <div className="chart-empty">No data in this range</div>

  const iw = W - PAD.l - PAD.r
  const ih = height - PAD.t - PAD.b
  const X = (t: number) => PAD.l + ((t - x0) / (x1 - x0)) * iw
  const Y = (v: number) => PAD.t + ih - (v / y1) * ih
  const clock = (t: number) => new Date(t * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

  // Nearest timestamp across all series for the crosshair.
  const hoverT = hover == null ? null : all.reduce((best, [t]) => (Math.abs(t - hover) < Math.abs(best - hover) ? t : best), all[0][0])
  const hoverVals = hoverT == null ? [] : series.map((s, i) => {
    const p = s.points.find(([t]) => t === hoverT)
    return p ? { label: s.label, v: p[1], color: COLORS[i % COLORS.length] } : null
  }).filter((p): p is { label: string; v: number; color: string } => p !== null)

  return (
    <div className="chart">
      <svg
        viewBox={`0 0 ${W} ${height}`}
        width="100%"
        role="img"
        onMouseMove={(e) => {
          const r = e.currentTarget.getBoundingClientRect()
          const px = ((e.clientX - r.left) / r.width) * W
          setHover(x0 + ((px - PAD.l) / iw) * (x1 - x0))
        }}
        onMouseLeave={() => setHover(null)}
      >
        {ticks.map((v, i) => (
          <g key={i}>
            <line x1={PAD.l} x2={W - PAD.r} y1={Y(v)} y2={Y(v)} className="chart-grid" />
            <text x={PAD.l - 6} y={Y(v) + 3} textAnchor="end" className="chart-label">{format(v)}</text>
          </g>
        ))}
        <text x={PAD.l} y={height - 6} className="chart-label">{clock(x0)}</text>
        <text x={W - PAD.r} y={height - 6} textAnchor="end" className="chart-label">{clock(x1)}</text>
        {series.map((s, i) => (
          <polyline
            key={s.label}
            fill="none"
            stroke={COLORS[i % COLORS.length]}
            strokeWidth="1.5"
            strokeLinejoin="round"
            points={s.points.map(([t, v]) => `${X(t)},${Y(v)}`).join(' ')}
          />
        ))}
        {hoverT != null && <line x1={X(hoverT)} x2={X(hoverT)} y1={PAD.t} y2={PAD.t + ih} className="chart-cursor" />}
      </svg>
      <div className="chart-legend">
        {hoverT != null && <span className="chart-hover-time">{clock(hoverT)}</span>}
        {(hoverT != null ? hoverVals : series.map((s, i) => ({ label: s.label, v: NaN, color: COLORS[i % COLORS.length] }))).map((p) => (
          <span key={p.label} className="chart-legend-item">
            <i style={{ background: p.color }} />
            {p.label}{Number.isNaN(p.v) ? '' : `: ${format(p.v)}`}
          </span>
        ))}
      </div>
    </div>
  )
}
