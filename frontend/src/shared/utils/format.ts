// Formats a byte count for humans. Uses decimal-style thresholds with one
// decimal place, trimmed when whole ("50 MB", "1.5 GB").
export function formatBytes(bytes: number): string {
  if (bytes < 1000) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = -1
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000
    unit += 1
  }
  const rounded = Math.round(value * 10) / 10
  return `${rounded} ${units[unit]}`
}

// Formats an ISO timestamp as a relative age like "3 days ago". Falls back
// to a locale date string for anything a month old or older.
export function formatRelativeDate(iso: string, now: Date = new Date()): string {
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return 'unknown'
  const seconds = Math.round((now.getTime() - then) / 1000)
  if (seconds < 0) return 'unknown'
  if (seconds < 60) return 'just now'
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? '' : 's'} ago`
  const hours = Math.round(minutes / 60)
  if (hours < 24) return `${hours} hour${hours === 1 ? '' : 's'} ago`
  const days = Math.round(hours / 24)
  if (days < 30) return `${days} day${days === 1 ? '' : 's'} ago`
  return new Date(iso).toLocaleDateString()
}

// Short display form of an image id: the hex digest after "sha256:", first
// 12 characters, matching podman's short id convention.
export function shortImageId(id: string): string {
  const digest = id.startsWith('sha256:') ? id.slice('sha256:'.length) : id
  return digest.slice(0, 12)
}
