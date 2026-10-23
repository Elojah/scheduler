// units are the plural of each FREQ's period.
const units: Record<string, string> = {
  SECONDLY: 'seconds',
  MINUTELY: 'minutes',
  HOURLY: 'hours',
  DAILY: 'days',
  WEEKLY: 'weeks',
  MONTHLY: 'months',
  YEARLY: 'years'
}

// every is each FREQ, with an INTERVAL of 1.
const every: Record<string, string> = {
  SECONDLY: 'every second',
  MINUTELY: 'every minute',
  HOURLY: 'hourly',
  DAILY: 'daily',
  WEEKLY: 'weekly',
  MONTHLY: 'monthly',
  YEARLY: 'yearly'
}

// parts returns the parts of the rule's first RRULE line, such as { FREQ: 'DAILY' }, or undefined if it has none.
const parts = (rule: string): Record<string, string> | undefined => {
  const line = rule.split(/\r?\n/).find((l) => l.toUpperCase().startsWith('RRULE:'))
  if (!line) {
    return undefined
  }
  return Object.fromEntries(
    line
      .slice('RRULE:'.length)
      .split(';')
      .map((p) => {
        const [k, v = ''] = p.split('=')
        return [k.toUpperCase(), v.toUpperCase()]
      })
  )
}

// xfreq returns an X-FREQ, an ISO 8601 duration of hours, minutes and seconds such as PT0.5S, as Go writes it: 0.5s.
const xfreq = (v: string): string => {
  const m = /^PT(?:([\d.]+)H)?(?:([\d.]+)M)?(?:([\d.]+)S)?$/.exec(v)
  if (!m) {
    return v
  }
  return [
    [m[1], 'h'],
    [m[2], 'm'],
    [m[3], 's']
  ]
    .filter(([n]) => n)
    .map(([n, unit]) => `${Number(n)}${unit}`)
    .join('')
}

// describe returns how often a rule repeats, such as "every 15 minutes" or "weekly on MO, WE".
export const describe = (rule: string): string => {
  const p = parts(rule)
  if (!p) {
    return /^RDATE/m.test(rule) ? 'on dates' : 'once'
  }
  let s: string
  if (p['X-FREQ']) {
    s = `every ${xfreq(p['X-FREQ'])}`
  } else {
    const n = Number(p.INTERVAL || 1)
    s = (n === 1 ? every[p.FREQ] : `every ${n} ${units[p.FREQ]}`) ?? p.FREQ
  }
  if (p.BYDAY) {
    s += ` on ${p.BYDAY.split(',').join(', ')}`
  }
  if (p.COUNT) {
    s += `, ${p.COUNT} times`
  }
  if (p.UNTIL) {
    s += `, until ${p.UNTIL.slice(0, 4)}-${p.UNTIL.slice(4, 6)}-${p.UNTIL.slice(6, 8)}`
  }
  return s
}

const pad = (n: number) => String(n).padStart(2, '0')

// presets returns rules to start from, in the browser's time zone, from today.
export const presets = (): { label: string; rule: string }[] => {
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone
  const d = new Date()
  const today = `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}`
  const start = (time: string) => (tz ? `DTSTART;TZID=${tz}:${today}T${time}` : `DTSTART:${today}T${time}Z`)
  return [
    { label: 'Every minute', rule: `${start('000000')}\nRRULE:FREQ=MINUTELY` },
    { label: 'Hourly', rule: `${start('000000')}\nRRULE:FREQ=HOURLY` },
    { label: 'Daily 09:00', rule: `${start('090000')}\nRRULE:FREQ=DAILY` },
    { label: 'Weekdays 09:00', rule: `${start('090000')}\nRRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR` }
  ]
}
