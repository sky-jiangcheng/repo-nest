// BrandMark — the RepoNest mark, drawn from the same construction as the
// brand SSOT (build/icon.svg) that generates the app icon, favicons and the
// VS Code extension icon.
//
// Why this exists: the navbar used a "▦" text glyph while every shipped
// artifact carried the navy-and-gold nest. Same product, two identities — and
// the glyph also inherited font metrics, so it shifted with the system font
// and broke at large sizes. One component, used everywhere in the app, keeps
// the UI on the same mark as the icon in the dock.
//
// The gold gradient is defined with a unique id per instance: duplicating an
// id across several inline SVGs makes browsers resolve `url(#id)` to the
// first match, so all instances would take instance #1's colours.

let uid = 0

interface Props {
  /** Rendered width/height in px. The mark is square. */
  size?: number
  className?: string
  /**
   * 'full'   — rounded navy tile with the gold mark (app icon, favicon)
   * 'mark'   — gold strokes only, no tile (needs a dark surface behind it)
   * Default 'full'.
   */
  variant?: 'full' | 'mark'
  /** Accessible name. Pass null when an adjacent text label already names the product. */
  title?: string
}

export default function BrandMark({ size = 22, className, variant = 'full', title }: Props) {
  const id = `rn-${++uid}`
  const gold = `url(#${id}-gold)`
  const bg = `url(#${id}-bg)`

  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 512 512"
      className={className}
      role={title ? 'img' : 'presentation'}
      aria-label={title ?? undefined}
      aria-hidden={title ? undefined : true}
      focusable="false"
    >
      {title && <title>{title}</title>}
      <defs>
        <linearGradient id={`${id}-bg`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#1F2C4D" />
          <stop offset="1" stopColor="#0B1526" />
        </linearGradient>
        <linearGradient id={`${id}-gold`} gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="0" y2="512">
          <stop offset="0" stopColor="#F7CE58" />
          <stop offset="1" stopColor="#E0A63C" />
        </linearGradient>
        {variant === 'full' && (
          <clipPath id={`${id}-clip`}>
            <rect x="0" y="0" width="512" height="512" rx="104" ry="104" />
          </clipPath>
        )}
      </defs>

      {variant === 'full' ? (
        <g clipPath={`url(#${id}-clip)`}>
          <rect x="0" y="0" width="512" height="512" fill={bg} />
          <g stroke={gold} fill="none" strokeLinecap="round">
            {/* Repository card cradled in the nest bowl — same geometry as
                build/icon.svg. Interior lines are dropped below ~20px, where
                they close up into a smudge at favicon sizes. */}
            <rect x="190" y="94" width="132" height="148" rx="30" strokeWidth="28" />
            {size >= 20 && (
              <>
                <path d="M222 148h68" strokeWidth="18" />
                <path d="M222 184h68" strokeWidth="18" />
              </>
            )}
            <path d="M84 250A172 172 0 0 0 428 250" strokeWidth="36" />
            <path d="M128 258A128 128 0 0 0 384 258" strokeWidth="28" />
          </g>
        </g>
      ) : (
        <g stroke={gold} fill="none" strokeLinecap="round">
          <rect x="190" y="94" width="132" height="148" rx="30" strokeWidth="28" />
          {size >= 20 && (
            <>
              <path d="M222 148h68" strokeWidth="18" />
              <path d="M222 184h68" strokeWidth="18" />
            </>
          )}
          <path d="M84 250A172 172 0 0 0 428 250" strokeWidth="36" />
          <path d="M128 258A128 128 0 0 0 384 258" strokeWidth="28" />
        </g>
      )}
    </svg>
  )
}
