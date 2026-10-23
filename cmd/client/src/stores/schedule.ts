import { defineStore } from 'pinia'
import { ref } from 'vue'
import { get, post } from '@/utils/api'
import type { Occurrence, Schedule } from '@/types/schedule'

export const useScheduleStore = defineStore('schedule', () => {
  const schedules = ref<Schedule[]>([])
  // version counts the changes made here, for the calendar to refetch their times.
  const version = ref(0)

  // list loads every schedule, in the order they were created.
  const list = async (): Promise<Schedule[]> => {
    const res = await post<{ schedules: Schedule[] }>('/api/schedules/list', {})
    schedules.value = res.schedules
    return res.schedules
  }

  const create = async (s: Omit<Schedule, 'id'>): Promise<Schedule> => {
    const res = await post<{ schedule: Schedule }>('/api/schedules/create', s)
    schedules.value.push(res.schedule)
    version.value++
    return res.schedule
  }

  const update = async (s: Schedule): Promise<Schedule> => {
    const res = await post<{ schedule: Schedule }>('/api/schedules/update', s)
    schedules.value = schedules.value.map((old) => (old.id === s.id ? res.schedule : old))
    version.value++
    return res.schedule
  }

  const remove = async (id: string): Promise<void> => {
    await post('/api/schedules/delete', { id })
    schedules.value = schedules.value.filter((s) => s.id !== id)
    version.value++
  }

  // occurrences returns the times of each enabled schedule from from to to, excluded, those at most merge apart, a Go
  // duration, merged into spans.
  const occurrences = async (from: Date, to: Date, merge: string): Promise<Occurrence[]> => {
    const q = new URLSearchParams({ from: from.toISOString(), to: to.toISOString(), merge })
    const res = await get<{ occurrences: Occurrence[] }>(`/api/occurrences?${q}`)
    return res.occurrences
  }

  // preview returns the next times of rule, or throws why the scheduler would refuse it.
  const preview = async (rule: string): Promise<string[]> => {
    const res = await post<{ times: string[] }>('/api/preview', { rule })
    return res.times
  }

  return { schedules, version, list, create, update, remove, occurrences, preview }
})
