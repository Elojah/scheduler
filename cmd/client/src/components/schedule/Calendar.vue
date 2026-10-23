<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import FullCalendar from '@fullcalendar/vue3'
import dayGridPlugin from '@fullcalendar/daygrid'
import timeGridPlugin from '@fullcalendar/timegrid'
import type { CalendarOptions, EventInput, EventSourceFuncArg } from '@fullcalendar/core'
import { useScheduleStore } from '@/stores/schedule'
import { useFail } from '@/composables/useFail'
import { describe } from '@/utils/rule'
import { formatTime } from '@/utils/date'
import type { Occurrence } from '@/types/schedule'

// week bounds the range of a day's or a week's view, a week and an hour when clocks go back: a month's is longer, and
// merges times up to 2h apart, so that an hourly rule is a bar rather than 24 times a day.
const week = 8 * 24 * 3600 * 1000

// colors are the schedules', in turn, by their rank in the list.
const colors = ['#0ea5e9', '#f59e0b', '#8b5cf6', '#10b981', '#ef4444', '#ec4899', '#14b8a6', '#6366f1']

const store = useScheduleStore()
const fail = useFail()

const calendar = ref<InstanceType<typeof FullCalendar>>()
const occurrences = ref<Occurrence[]>([])

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

// events returns the times of the range shown: a span of a single time is an event at it, a span of more an event
// from its first to its last.
const events = async ({ start, end }: EventSourceFuncArg): Promise<EventInput[]> => {
  try {
    // The list too, so that a schedule changed elsewhere is labelled.
    const merge = end.getTime() - start.getTime() > week ? '2h' : '15m'
    const [list] = await Promise.all([store.occurrences(start, end, merge), store.list()])
    occurrences.value = list
  } catch (e) {
    fail(e)
    return []
  }
  const now = Date.now()
  return occurrences.value.flatMap((o) =>
    o.spans.map((span, i) => {
      const more = o.truncated && i === o.spans.length - 1 ? '+' : ''
      return {
        title: span.count === 1 ? label(o.id) : `${label(o.id)} ×${span.count}${more}`,
        start: span.start,
        end: span.count === 1 ? undefined : span.end,
        color: color(o.id),
        // Past: whether it ran depends on whether it was enabled, and the scheduler up, then.
        classNames: new Date(span.end).getTime() < now ? ['opacity-50'] : []
      }
    })
  )
}

const options: CalendarOptions = {
  plugins: [dayGridPlugin, timeGridPlugin],
  initialView: 'timeGridWeek',
  headerToolbar: { left: 'prev,next today', center: 'title', right: 'timeGridDay,timeGridWeek,dayGridMonth' },
  buttonText: { today: 'Today', day: 'Day', week: 'Week', month: 'Month' },
  firstDay: 1,
  nowIndicator: true,
  allDaySlot: false,
  height: '72vh',
  scrollTime: `${String(Math.max(0, new Date().getHours() - 1)).padStart(2, '0')}:00:00`,
  slotLabelFormat: { hour: '2-digit', minute: '2-digit', hour12: false },
  eventTimeFormat: { hour: '2-digit', minute: '2-digit', hour12: false },
  // A single time is shown this long.
  defaultTimedEventDuration: '00:15',
  dayMaxEvents: 4,
  events
}

// A change of the schedules changes their times.
watch(
  () => store.version,
  () => calendar.value?.getApi().refetchEvents()
)
</script>

<template>
  <FullCalendar ref="calendar" :options="options" />

  <h3 class="text-base font-semibold mt-6 mb-2">Running</h3>
  <p v-if="!occurrences.length" class="text-surface-500">No schedule is enabled.</p>
  <ul class="flex flex-col gap-2 text-sm">
    <li v-for="o in occurrences" :key="o.id" class="flex flex-wrap items-center gap-2">
      <span class="w-3 h-3 rounded-full shrink-0" :style="{ background: color(o.id) }" />
      <span class="font-medium">{{ label(o.id) }}</span>
      <span class="text-surface-500">next {{ o.next ? formatTime(o.next) : 'none: the rule has ended' }}</span>
      <span v-if="o.truncated" class="text-surface-500">(too frequent to show each time: shown as blocks)</span>
    </li>
  </ul>
</template>
