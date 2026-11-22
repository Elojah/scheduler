<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import FullCalendar, { useCalendarController } from '@fullcalendar/vue3'
import type { CalendarOptions, EventClickInfo, EventInput, EventSourceFuncInfo } from '@fullcalendar/vue3'
import classicTheme from '@fullcalendar/vue3/themes/classic'
import dayGridPlugin from '@fullcalendar/vue3/daygrid'
import timeGridPlugin from '@fullcalendar/vue3/timegrid'
import '@fullcalendar/vue3/skeleton.css'
import '@fullcalendar/vue3/themes/classic/theme.css'
import Button from 'primevue/button'
import SelectButton from 'primevue/selectbutton'
import { useScheduleStore } from '@/stores/schedule'
import { useFail } from '@/composables/useFail'
import { describe } from '@/utils/rule'
import type { Span } from '@/types/schedule'

// week bounds the range of a day's or a week's view, a week and an hour when clocks go back: a month's is longer, and
// merges times up to 2h apart, so that an hourly rule is a bar rather than 24 times a day.
const week = 8 * 24 * 3600 * 1000

// colors are the schedules', in turn, by their rank in the list.
const colors = ['#0ea5e9', '#f59e0b', '#8b5cf6', '#10b981', '#ef4444', '#ec4899', '#14b8a6', '#6366f1']

const views = [
  { label: 'Day', value: 'timeGridDay' },
  { label: 'Week', value: 'timeGridWeek' },
  { label: 'Month', value: 'dayGridMonth' }
]

const store = useScheduleStore()
const fail = useFail()

// controller drives the calendar from the toolbar, and is reactive: its view changes as the calendar's does.
const controller = useCalendarController()
const view = computed({
  get: () => controller.view?.type,
  set: (v) => v && controller.changeView(v)
})

const calendar = ref<InstanceType<typeof FullCalendar>>()

const byID = computed(() => new Map(store.schedules.map((s) => [s.id, s])))
const color = (id: string) =>
  colors[
    Math.max(
      0,
      store.schedules.findIndex((s) => s.id === id)
    ) % colors.length
  ]
const label = (id: string) => {
  const s = byID.value.get(id)
  return s ? `${s.operation} · ${describe(s.rule)}` : id
}

// byDay merges the spans starting the same day into one, so that a month lists each schedule once a day.
const byDay = (spans: Span[]): Span[] =>
  spans.reduce<Span[]>((merged, span) => {
    const last = merged.at(-1)
    if (last && new Date(last.start).toDateString() === new Date(span.start).toDateString()) {
      merged[merged.length - 1] = { start: last.start, end: span.end, count: last.count + span.count }
    } else {
      merged.push(span)
    }
    return merged
  }, [])

// events returns the times of the range shown: a span of a single time is an event at it, a span of more an event
// from its first to its last.
const events = async ({ start, end }: EventSourceFuncInfo): Promise<EventInput[]> => {
  const month = end.getTime() - start.getTime() > week
  try {
    // The list too, so that a schedule changed elsewhere is labelled.
    const [occurrences] = await Promise.all([store.occurrences(start, end, month ? '2h' : '15m'), store.list()])
    return occurrences.flatMap((o) => {
      const spans = month ? byDay(o.spans) : o.spans
      return spans.map((span, i) => {
        const more = o.truncated && i === spans.length - 1 ? '+' : ''
        return {
          title: span.count === 1 ? label(o.id) : `${label(o.id)} ×${span.count}${more}`,
          start: span.start,
          end: span.count === 1 ? undefined : span.end,
          color: color(o.id),
          extendedProps: { schedule: o.id }
        }
      })
    })
  } catch (e) {
    fail(e)
    return []
  }
}

const options: CalendarOptions = {
  plugins: [classicTheme, dayGridPlugin, timeGridPlugin],
  controller,
  // A week is too narrow on a phone.
  initialView: window.matchMedia('(max-width: 640px)').matches ? 'timeGridDay' : 'timeGridWeek',
  firstDay: 1,
  nowIndicator: true,
  allDaySlot: false,
  height: '70vh',
  scrollTime: `${String(Math.max(0, new Date().getHours() - 1)).padStart(2, '0')}:00:00`,
  slotHeaderFormat: { hour: '2-digit', minute: '2-digit', hour12: false },
  eventTimeFormat: { hour: '2-digit', minute: '2-digit', hour12: false },
  // A single time is shown this long.
  defaultTimedEventDuration: '00:15',
  dayMaxEvents: true,
  events,
  // A view's spans are merged for its range: a day's are not a month's.
  lazyFetching: false,
  // Past: whether it ran depends on whether it was enabled, and the scheduler up, then.
  eventClass: ({ isPast }) => (isPast ? 'cursor-pointer opacity-50' : 'cursor-pointer'),
  // The title in full, which a short event cuts.
  eventDidMount: ({ el, event }) => el.setAttribute('title', event.title),
  eventClick: ({ event }: EventClickInfo) => {
    const s = byID.value.get(event.extendedProps.schedule)
    if (s) {
      store.edit(s)
    }
  }
}

// A change of the schedules changes their times.
watch(
  () => store.version,
  () => calendar.value?.getApi().refetchEvents()
)
</script>

<template>
  <div class="flex flex-wrap items-center gap-2 mb-4">
    <Button label="Today" severity="secondary" outlined size="small" @click="controller.today()" />
    <div class="flex">
      <Button icon="pi pi-chevron-left" text rounded severity="secondary" aria-label="Previous" @click="controller.prev()" />
      <Button icon="pi pi-chevron-right" text rounded severity="secondary" aria-label="Next" @click="controller.next()" />
    </div>
    <h2 class="text-lg font-semibold">{{ controller.view?.title }}</h2>
    <SelectButton v-model="view" :options="views" optionLabel="label" optionValue="value" :allowEmpty="false" size="small" class="ml-auto" />
  </div>
  <FullCalendar ref="calendar" :options="options" />
</template>
