// Schedule is the scheduler's: it runs action at each time of rule while enabled.
export type Schedule = {
  id: string
  // rule is a recurrence set, as RFC 5545 writes it: a DTSTART line, then RRULE, RDATE, EXDATE or EXRULE lines.
  rule: string
  action: string
  options: Options
  enabled: boolean
}

// Options tunes a schedule's alarm, durations being Go's, such as "1m30s", and holds its action's params.
export type Options = {
  max_delay?: string
  coalesce?: string
  spin?: string
  poll?: string
  params?: unknown
}

// Occurrence holds an enabled schedule's times in a range, merged into spans.
export type Occurrence = {
  id: string
  spans: Span[]
  // next is its first time after now, if any.
  next?: string
  // truncated is whether it has too many times to walk: its last span then lasts until the range's end.
  truncated?: boolean
}

// Span holds times of a schedule close to one another: a single time if count is 1.
export type Span = {
  start: string
  end: string
  count: number
}
