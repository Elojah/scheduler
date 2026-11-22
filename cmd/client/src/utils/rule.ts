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
  SECONDLY: 'Every second',
  MINUTELY: 'Every minute',
  HOURLY: 'Hourly',
  DAILY: 'Daily',
  WEEKLY: 'Weekly',
  MONTHLY: 'Monthly',
  YEARLY: 'Yearly'
}

// days are BYDAY's, in the week's order.
const days: Record<string, string> = { MO: 'Mon', TU: 'Tue', WE: 'Wed', TH: 'Thu', FR: 'Fri', SA: 'Sat', SU: 'Sun' }

// named are the sets of days with a name.
const named: Record<string, string> = {
  'MO,TU,WE,TH,FR': 'weekdays',
  'SA,SU': 'weekends',
  'MO,TU,WE,TH,FR,SA,SU': 'every day'
}

// line returns what follows the name of the rule's first line named name, such as DTSTART, or undefined.
const line = (rule: string, name: string): string | undefined =>
  rule
    .split(/\r?\n/)
    .find((l) => new RegExp(`^${name}[:;]`, 'i').test(l))
    ?.slice(name.length)

// parts returns the parts of the rule's first RRULE line, such as { FREQ: 'DAILY' }, or undefined if it has none.
const parts = (rule: string): Record<string, string> | undefined => {
  const v = line(rule, 'RRULE')
  if (v === undefined) {
    return undefined
  }
  return Object.fromEntries(
    v
      .slice(1)
      .split(';')
      .map((p) => {
        const [k = '', x = ''] = p.split('=')
        return [k.toUpperCase(), x.toUpperCase()]
      })
  )
}

// Start is a DTSTART: its date, such as 20261023, its time, such as 090000, if any, and its zone if not the
// browser's: UTC or a TZID.
type Start = { date: string; time?: string; zone?: string }

const start = (rule: string): Start | undefined => {
  const v = line(rule, 'DTSTART')
  const m = v && /:(\d{8})(?:T(\d{6})(Z)?)?$/i.exec(v)
  if (!v || !m) {
    return undefined
  }
  const tzid = /;TZID=([^:;]+)/i.exec(v)?.[1]
  const zone = m[3] ? 'UTC' : tzid !== Intl.DateTimeFormat().resolvedOptions().timeZone ? tzid : undefined
  return { date: m[1], time: m[2], zone }
}

// weekday returns the day of the week of a date such as 20261023, as days writes it.
const weekday = (date: string): string => {
  const d = new Date(Date.UTC(Number(date.slice(0, 4)), Number(date.slice(4, 6)) - 1, Number(date.slice(6, 8))))
  return Object.values(days)[(d.getUTCDay() + 6) % 7]
}

// weekdays returns BYDAY in words, such as "weekdays" or "Mon, Wed", or as written if it has ordinal days, such as
// 1MO for a month's first Monday.
const weekdays = (byday: string): string => {
  const set = byday.split(',')
  const known = Object.keys(days).filter((d) => set.includes(d))
  if (known.length !== set.length) {
    return byday
  }
  return named[known.join(',')] ?? known.map((d) => days[d]).join(', ')
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

const pad = (n: number | string) => String(n).padStart(2, '0')

// clock returns the times of day of a daily or longer rule, from its BYHOUR and BYMINUTE, or its DTSTART's, such as
// " at 06:00, 18:00", or ", 4 times a day" past 3.
const clock = (p: Record<string, string>, s?: Start): string => {
  const hours = p.BYHOUR?.split(',') ?? (s?.time ? [s.time.slice(0, 2)] : [])
  const minutes = p.BYMINUTE?.split(',') ?? [s?.time?.slice(2, 4) ?? '0']
  const times = hours.flatMap((h) => minutes.map((m) => `${pad(Number(h))}:${pad(Number(m))}`)).sort()
  if (!times.length) {
    return ''
  }
  const zone = s?.zone ? ` ${s.zone}` : ''
  return times.length > 3 ? `, ${times.length} times a day${zone}` : ` at ${times.join(', ')}${zone}`
}

// describe returns how often a rule repeats, in words, such as "Every 15 minutes" or "Weekdays at 09:00".
export const describe = (rule: string): string => {
  const p = parts(rule)
  if (!p) {
    return /^RDATE/im.test(rule) ? 'On dates' : 'Once'
  }
  const s = start(rule)
  const n = Number(p.INTERVAL || 1)
  const period = (n === 1 ? every[p.FREQ] : units[p.FREQ] && `Every ${n} ${units[p.FREQ]}`) ?? p.FREQ
  let d: string
  if (p['X-FREQ']) {
    d = `Every ${xfreq(p['X-FREQ'])}`
  } else if (['SECONDLY', 'MINUTELY', 'HOURLY'].includes(p.FREQ)) {
    const w = p.BYDAY && weekdays(p.BYDAY)
    d = period + (w && w !== 'every day' ? ` on ${w}` : '')
  } else if (p.BYDAY && n === 1 && ['DAILY', 'WEEKLY'].includes(p.FREQ)) {
    const w = weekdays(p.BYDAY)
    const lead = w === 'every day' ? 'Daily' : Object.values(named).includes(w) ? w : `Weekly on ${w}`
    d = lead[0].toUpperCase() + lead.slice(1) + clock(p, s)
  } else if (p.FREQ === 'WEEKLY') {
    const w = p.BYDAY ? weekdays(p.BYDAY) : s && weekday(s.date)
    d = period + (w ? ` on ${w}` : '') + clock(p, s)
  } else if (p.FREQ === 'MONTHLY') {
    const day = p.BYMONTHDAY ?? (s && String(Number(s.date.slice(6, 8))))
    const on = p.BYDAY ? ` on ${weekdays(p.BYDAY)}` : day ? ` on day ${day}` : ''
    d = period + on + clock(p, s)
  } else {
    d = period + clock(p, s)
  }
  if (p.COUNT) {
    d += `, ${p.COUNT} times`
  }
  if (p.UNTIL) {
    d += `, until ${p.UNTIL.slice(0, 4)}-${p.UNTIL.slice(4, 6)}-${p.UNTIL.slice(6, 8)}`
  }
  return d
}

// presets returns rules to start from, in the browser's time zone, from today.
export const presets = (): { label: string; rule: string }[] => {
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone
  const d = new Date()
  const today = `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}`
  const at = (time: string) => (tz ? `DTSTART;TZID=${tz}:${today}T${time}` : `DTSTART:${today}T${time}Z`)
  return [
    { label: 'Every minute', rule: `${at('000000')}\nRRULE:FREQ=MINUTELY` },
    { label: 'Hourly', rule: `${at('000000')}\nRRULE:FREQ=HOURLY` },
    { label: 'Daily at 09:00', rule: `${at('090000')}\nRRULE:FREQ=DAILY` },
    { label: 'Weekdays at 09:00', rule: `${at('090000')}\nRRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR` }
  ]
}
