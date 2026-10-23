import logo from '../assets/logo-128.png'

/** The Bordo app icon (same artwork as the iOS app). */
export function Logo({ size = 20 }: { size?: number }) {
  return (
    <img
      className="logo"
      src={logo}
      width={size}
      height={size}
      alt=""
      aria-hidden="true"
      style={{ borderRadius: Math.round(size * 0.224) }}
    />
  )
}
