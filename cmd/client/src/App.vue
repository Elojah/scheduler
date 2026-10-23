<script setup lang="ts">
import { defineAsyncComponent, onMounted } from 'vue'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'
import Toast from 'primevue/toast'
import ConfirmDialog from 'primevue/confirmdialog'
import ScheduleTable from '@/components/schedule/Table.vue'
import { useScheduleStore } from '@/stores/schedule'
import { useFail } from '@/composables/useFail'

// Lazy, as trax's views: FullCalendar is most of the bundle.
const ScheduleCalendar = defineAsyncComponent(() => import('@/components/schedule/Calendar.vue'))

const store = useScheduleStore()
const fail = useFail()

onMounted(() => store.list().catch(fail))
</script>

<template>
  <div class="min-h-screen bg-surface-0 dark:bg-surface-950 text-surface-900 dark:text-surface-0">
    <header class="flex items-center gap-3 px-6 py-3 border-b border-surface-200 dark:border-surface-800">
      <img src="/logo.svg" alt="" class="w-8 h-8" />
      <h1 class="text-xl font-semibold">scheduler</h1>
    </header>
    <main class="px-6 py-4">
      <!-- lazy: the calendar mounts once shown, fetching the times of now. -->
      <Tabs value="schedules" lazy>
        <TabList>
          <Tab value="schedules"><i class="pi pi-list mr-2" />Schedules</Tab>
          <Tab value="calendar"><i class="pi pi-calendar mr-2" />Calendar</Tab>
        </TabList>
        <TabPanels>
          <TabPanel value="schedules">
            <ScheduleTable />
          </TabPanel>
          <TabPanel value="calendar">
            <ScheduleCalendar />
          </TabPanel>
        </TabPanels>
      </Tabs>
    </main>
    <Toast />
    <ConfirmDialog />
  </div>
</template>
