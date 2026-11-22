<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useScheduleStore } from '@/stores/schedule'
import { useFail } from '@/composables/useFail'
import { describe } from '@/utils/rule'
import { formatTime } from '@/utils/date'
import type { Schedule } from '@/types/schedule'

const store = useScheduleStore()
const confirm = useConfirm()
const fail = useFail()

// The next times pass: reloaded every 30s while shown.
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  timer = setInterval(() => store.loadNext().catch(() => undefined), 30_000)
})
onUnmounted(() => clearInterval(timer))

const toggle = (s: Schedule, enabled: boolean) => store.update({ ...s, enabled }).catch(fail)

const remove = (s: Schedule) =>
  confirm.require({
    header: 'Delete schedule',
    message: `Delete ${s.operation}, ${describe(s.rule).toLowerCase()}? A run under way finishes.`,
    icon: 'pi pi-exclamation-triangle',
    acceptProps: { label: 'Delete', severity: 'danger' },
    rejectProps: { label: 'Cancel', severity: 'secondary', text: true },
    accept: () => store.remove(s.id).catch(fail)
  })
</script>

<template>
  <DataTable :value="store.schedules" dataKey="id" rowHover>
    <template #empty>
      <div class="flex flex-col items-center gap-3 py-12 text-muted-color">
        <i class="pi pi-calendar-clock text-3xl" />
        <span>No schedules yet.</span>
        <Button icon="pi pi-plus" label="New schedule" outlined @click="store.edit()" />
      </div>
    </template>
    <Column header="Operation">
      <template #body="{ data }">
        <div class="font-medium">{{ data.operation }}</div>
        <!-- A phone has no room for the schedule's column: it goes under. -->
        <div class="sm:hidden text-sm text-muted-color">{{ describe(data.rule) }}</div>
      </template>
    </Column>
    <Column header="Schedule" headerClass="hidden sm:table-cell" bodyClass="hidden sm:table-cell">
      <template #body="{ data }">
        <span v-tooltip.top="{ value: data.rule, class: 'font-mono text-xs max-w-none' }">{{ describe(data.rule) }}</span>
      </template>
    </Column>
    <Column header="Next run" headerClass="hidden sm:table-cell" bodyClass="hidden sm:table-cell">
      <template #body="{ data }">
        <span v-if="data.enabled && store.next[data.id]">{{ formatTime(store.next[data.id]!) }}</span>
        <span v-else-if="data.enabled && data.id in store.next" class="text-muted-color">Ended</span>
        <span v-else class="text-muted-color">—</span>
      </template>
    </Column>
    <Column header="Enabled" class="w-0">
      <template #body="{ data }">
        <ToggleSwitch :modelValue="data.enabled" :aria-label="`Enable ${data.operation}`" @update:modelValue="(v: boolean) => toggle(data, v)" />
      </template>
    </Column>
    <Column class="w-0">
      <template #body="{ data }">
        <div class="flex justify-end">
          <Button v-tooltip.top="'Edit'" icon="pi pi-pencil" text rounded severity="secondary" aria-label="Edit" @click="store.edit(data)" />
          <Button v-tooltip.top="'Delete'" icon="pi pi-trash" text rounded severity="danger" aria-label="Delete" @click="remove(data)" />
        </div>
      </template>
    </Column>
  </DataTable>
</template>
