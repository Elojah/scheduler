// formatTime returns an instant in the browser's time zone, such as "Fri 23 Oct, 09:00:00", with its milliseconds if
// it has some.
export const formatTime = (iso: string): string => {
  const d = new Date(iso)
  return d.toLocaleString(undefined, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    ...(d.getMilliseconds() ? { fractionalSecondDigits: 3 } : {})
  })
}
