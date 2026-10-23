<script setup lang="ts">
import { ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import ScheduleDialog from '@/components/schedule/Dialog.vue'
import { useScheduleStore } from '@/stores/schedule'
import { useFail } from '@/composables/useFail'
import { describe } from '@/utils/rule'
import type { Schedule } from '@/types/schedule'

const store = useScheduleStore()
const confirm = useConfirm()
const fail = useFail()

const visible = ref(false)
// editing is the schedule the dialog edits, none to create one.
const editing = ref<Schedule>()

const open = (s?: Schedule) => {
  editing.value = s
  visible.value = true
}

const toggle = (s: Schedule, enabled: boolean) => store.update({ ...s, enabled }).catch(fail)

const remove = (s: Schedule) =>
  confirm.require({
    header: 'Delete schedule',
    message: `Delete ${s.action}, ${describe(s.rule)}? Its action under way finishes.`,
    icon: 'pi pi-trash',
    acceptProps: { label: 'Delete', icon: 'pi pi-trash', severity: 'danger' },
    rejectProps: { label: 'Cancel', severity: 'secondary', outlined: true },
    accept: () => store.remove(s.id).catch(fail)
  })
</script>

<template>
  <div class="flex items-center justify-between gap-2 mb-4">
    <h2 class="text-lg font-semibold">Schedules</h2>
    <div class="flex gap-2">
      <Button icon="pi pi-refresh" severity="secondary" outlined aria-label="Refresh" @click="store.list().catch(fail)" />
      <Button icon="pi pi-plus" label="Create" severity="success" @click="open()" />
    </div>
  </div>

  <DataTable :value="store.schedules" dataKey="id" size="small">
    <template #empty>
      <div class="py-6 text-center text-surface-500">No schedule yet: create one.</div>
    </template>
    <Column header="Enabled" class="w-24">
      <template #body="{ data }">
        <ToggleSwitch :modelValue="data.enabled" :aria-label="`Enable ${data.action}`" @update:modelValue="(v: boolean) => toggle(data, v)" />
      </template>
    </Column>
    <Column header="Action">
      <template #body="{ data }">
        <Tag :value="data.action" severity="secondary" />
      </template>
    </Column>
    <Column header="Frequency">
      <template #body="{ data }">{{ describe(data.rule) }}</template>
    </Column>
    <Column header="Rule">
      <template #body="{ data }">
        <pre class="font-mono text-xs whitespace-pre-wrap break-all text-surface-600 dark:text-surface-300">{{ data.rule }}</pre>
      </template>
    </Column>
    <Column class="w-0">
      <template #body="{ data }">
        <div class="flex gap-2 justify-end">
          <Button icon="pi pi-cog" label="Edit" size="small" outlined class="btn-action" @click="open(data)" />
          <Button icon="pi pi-trash" label="Delete" size="small" severity="danger" outlined class="btn-action" @click="remove(data)" />
        </div>
      </template>
    </Column>
  </DataTable>

  <ScheduleDialog v-model:visible="visible" :schedule="editing" />
</template>
