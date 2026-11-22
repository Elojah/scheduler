import { defineStore } from 'pinia'
import { ref } from 'vue'
import { get, post } from '@/utils/api'
import type { Occurrence, Schedule } from '@/types/schedule'

export const useScheduleStore = defineStore('schedule', () => {
  const schedules = ref<Schedule[]>([])
  // next maps each enabled schedule's ID to its next time, or to none once its rule ended.
  const next = ref<Record<string, string | undefined>>({})
  // version counts the changes made here, for the calendar to refetch their times.
  const version = ref(0)
  // editor is the dialog's state: open, on the schedule it edits, or on none to create one.
  const editor = ref<{ open: boolean; schedule?: Schedule }>({ open: false })

  // occurrences returns the times of each enabled schedule from from to to, excluded, those at most merge apart, a Go
  // duration, merged into spans.
  const occurrences = async (from: Date, to: Date, merge: string): Promise<Occurrence[]> => {
    const q = new URLSearchParams({ from: from.toISOString(), to: to.toISOString(), merge })
    const res = await get<{ occurrences: Occurrence[] }>(`/api/occurrences?${q}`)
    return res.occurrences
  }

  // loadNext loads the next times: those of a second from now, which come with them.
  const loadNext = async (): Promise<void> => {
    const now = new Date()
    const list = await occurrences(now, new Date(now.getTime() + 1000), '0s')
    next.value = Object.fromEntries(list.map((o) => [o.id, o.next]))
  }

  // changed counts a change, and reloads the next times, which the change shows in any case.
  const changed = () => {
    version.value++
    loadNext().catch(() => undefined)
  }

  // list loads every schedule, in the order they were created, and their next times.
  const list = async (): Promise<Schedule[]> => {
    const [res] = await Promise.all([post<{ schedules: Schedule[] }>('/api/schedules/list', {}), loadNext()])
    schedules.value = res.schedules
    return res.schedules
  }

  // refresh loads the schedules, and has the calendar refetch their times.
  const refresh = async (): Promise<void> => {
    await list()
    version.value++
  }

  const create = async (s: Omit<Schedule, 'id'>): Promise<Schedule> => {
    const res = await post<{ schedule: Schedule }>('/api/schedules/create', s)
    schedules.value.push(res.schedule)
    changed()
    return res.schedule
  }

  const update = async (s: Schedule): Promise<Schedule> => {
    const res = await post<{ schedule: Schedule }>('/api/schedules/update', s)
    schedules.value = schedules.value.map((old) => (old.id === s.id ? res.schedule : old))
    changed()
    return res.schedule
  }

  const remove = async (id: string): Promise<void> => {
    await post('/api/schedules/delete', { id })
    schedules.value = schedules.value.filter((s) => s.id !== id)
    changed()
  }

  // preview returns the next times of rule, or throws why the scheduler would refuse it.
  const preview = async (rule: string): Promise<string[]> => {
    const res = await post<{ times: string[] }>('/api/preview', { rule })
    return res.times
  }

  // edit opens the dialog on s, or on a new schedule.
  const edit = (s?: Schedule) => {
    editor.value = { open: true, schedule: s }
  }

  return {
    schedules,
    next,
    version,
    editor,
    occurrences,
    loadNext,
    list,
    refresh,
    create,
    update,
    remove,
    preview,
    edit
  }
})
