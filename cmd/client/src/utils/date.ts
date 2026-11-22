const relative = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

const midnight = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()

// formatTime returns an instant in the browser's time zone, such as "Today, 09:00" or "Fri, Oct 23, 09:00", with its
// seconds, and milliseconds, if it has some.
export const formatTime = (iso: string): string => {
  const d = new Date(iso)
  const time = d.toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
    ...(d.getSeconds() || d.getMilliseconds() ? { second: '2-digit' } : {}),
    ...(d.getMilliseconds() ? { fractionalSecondDigits: 3 } : {})
  })
  // Rounded: a day is 23 or 25 hours when clocks change.
  const days = Math.round((midnight(d) - midnight(new Date())) / 86_400_000)
  const day = Math.abs(days) <= 1 ? relative.format(days, 'day').replace(/^./, (c) => c.toUpperCase()) : d.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' })
  return `${day}, ${time}`
}
