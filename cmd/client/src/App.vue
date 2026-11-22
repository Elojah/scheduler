<script setup lang="ts">
import { defineAsyncComponent, onMounted } from 'vue'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'
import Button from 'primevue/button'
import Toast from 'primevue/toast'
import ConfirmDialog from 'primevue/confirmdialog'
import ScheduleTable from '@/components/schedule/Table.vue'
import ScheduleDialog from '@/components/schedule/Dialog.vue'
import { useScheduleStore } from '@/stores/schedule'
import { useFail } from '@/composables/useFail'

// Lazy, as trax's views: FullCalendar is most of the bundle.
const ScheduleCalendar = defineAsyncComponent(() => import('@/components/schedule/Calendar.vue'))

const store = useScheduleStore()
const fail = useFail()

onMounted(() => store.list().catch(fail))
</script>

<template>
  <!-- surface-900 is Aura's dark content background, so that the tabs and the table make no box on the page. -->
  <div class="min-h-screen bg-surface-0 dark:bg-surface-900 text-color">
    <header class="border-b border-surface">
      <div class="max-w-6xl mx-auto flex items-center gap-3 px-4 sm:px-6 h-16">
        <img src="/logo.svg" alt="" class="size-8" />
        <h1 class="text-lg font-semibold">scheduler</h1>
        <div class="ml-auto flex items-center gap-2">
          <Button v-tooltip.bottom="'Refresh'" icon="pi pi-refresh" text rounded severity="secondary" aria-label="Refresh" @click="store.refresh().catch(fail)" />
          <Button icon="pi pi-plus" label="New schedule" @click="store.edit()" />
        </div>
      </div>
    </header>
    <main class="max-w-6xl mx-auto px-4 sm:px-6 py-4">
      <!-- lazy: the calendar mounts once shown, fetching the times of now. -->
      <Tabs value="schedules" lazy>
        <TabList>
          <Tab value="schedules">Schedules</Tab>
          <Tab value="calendar">Calendar</Tab>
        </TabList>
        <TabPanels class="px-0">
          <TabPanel value="schedules">
            <ScheduleTable />
          </TabPanel>
          <TabPanel value="calendar">
            <ScheduleCalendar />
          </TabPanel>
        </TabPanels>
      </Tabs>
    </main>
    <ScheduleDialog />
    <Toast />
    <ConfirmDialog />
  </div>
</template>
