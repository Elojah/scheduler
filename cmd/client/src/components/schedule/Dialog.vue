<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import Menu from 'primevue/menu'
import { useScheduleStore } from '@/stores/schedule'
import { useFail } from '@/composables/useFail'
import { describe, presets } from '@/utils/rule'
import { formatTime } from '@/utils/date'
import type { Options } from '@/types/schedule'

const store = useScheduleStore()
const fail = useFail()

// durations are the options the alarm takes, as Go durations, unset by default but for poll.
const durations = [
  { key: 'max_delay', label: 'Max delay', placeholder: 'none', help: 'Skips a time late by more than this.' },
  { key: 'coalesce', label: 'Coalesce', placeholder: 'none', help: 'Of times late by more than this, runs the latest.' },
  { key: 'spin', label: 'Spin', placeholder: 'none', help: 'Busy-waits this long before each time.' },
  { key: 'poll', label: 'Poll', placeholder: '1m', help: 'Checks the clock this often while waiting.' }
] as const

type Durations = Record<(typeof durations)[number]['key'], string>

const schedule = computed(() => store.editor.schedule)
const rule = ref('')
const operation = ref('')
const enabled = ref(true)
const options = ref<Durations>({ max_delay: '', coalesce: '', spin: '', poll: '' })
const params = ref('')
const paramsError = ref('')
const advanced = ref(false)
const saving = ref(false)

const presetMenu = ref<InstanceType<typeof Menu>>()
const presetItems = presets().map((p) => ({ label: p.label, command: () => (rule.value = p.rule) }))

// The form starts from the schedule edited, or from a daily rule, its options shown if it has some.
watch(
  () => store.editor.open,
  (open) => {
    if (!open) {
      return
    }
    const s = schedule.value
    rule.value = s?.rule ?? presets()[2].rule
    operation.value = s?.operation ?? 'noop'
    enabled.value = s?.enabled ?? true
    const o = s?.options ?? {}
    options.value = { max_delay: o.max_delay ?? '', coalesce: o.coalesce ?? '', spin: o.spin ?? '', poll: o.poll ?? '' }
    params.value = o.params === undefined ? '' : JSON.stringify(o.params, null, 2)
    paramsError.value = ''
    advanced.value = Object.values(options.value).some((v) => v) || !!params.value
  }
)

// The next times of the rule, previewed as it is typed, or why the scheduler would refuse it.
const times = ref<string[]>([])
const ruleError = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
// previewed numbers the previews, so that a late one does not replace a newer one.
let previewed = 0
watch(rule, (r) => {
  clearTimeout(timer)
  timer = setTimeout(async () => {
    const n = ++previewed
    try {
      const next = await store.preview(r)
      if (n === previewed) {
        times.value = next
        ruleError.value = ''
      }
    } catch (e) {
      if (n === previewed) {
        times.value = []
        ruleError.value = e instanceof Error ? e.message : String(e)
      }
    }
  }, 300)
})

const close = () => (store.editor.open = false)

const submit = async () => {
  const o: Options = {}
  for (const { key } of durations) {
    if (options.value[key].trim()) {
      o[key] = options.value[key].trim()
    }
  }
  if (params.value.trim()) {
    try {
      o.params = JSON.parse(params.value)
    } catch (e) {
      paramsError.value = `Not JSON: ${e instanceof Error ? e.message : e}`
      advanced.value = true
      return
    }
  }
  paramsError.value = ''

  const s = { rule: rule.value.trim(), operation: operation.value.trim(), options: o, enabled: enabled.value }
  saving.value = true
  try {
    await (schedule.value ? store.update({ ...s, id: schedule.value.id }) : store.create(s))
    close()
  } catch (e) {
    fail(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog
    :visible="store.editor.open"
    modal
    :draggable="false"
    :header="schedule ? 'Edit schedule' : 'New schedule'"
    class="w-[40rem] max-w-[calc(100vw-2rem)]"
    @update:visible="store.editor.open = $event"
  >
    <form id="schedule" class="flex flex-col gap-6" @submit.prevent="submit">
      <div class="flex items-end gap-6">
        <div class="flex flex-col gap-2 grow">
          <label for="operation" class="font-medium">Operation</label>
          <InputText id="operation" v-model="operation" fluid />
        </div>
        <div class="flex items-center gap-2 h-10">
          <ToggleSwitch v-model="enabled" inputId="enabled" />
          <label for="enabled">Enabled</label>
        </div>
      </div>

      <div class="flex flex-col gap-2">
        <div class="flex items-center justify-between">
          <label for="rule" class="font-medium">Rule</label>
          <Button type="button" label="Presets" icon="pi pi-angle-down" iconPos="right" size="small" text @click="presetMenu?.toggle($event)" />
          <Menu ref="presetMenu" :model="presetItems" popup />
        </div>
        <Textarea id="rule" v-model="rule" rows="2" autoResize spellcheck="false" class="font-mono text-sm" fluid :invalid="!!ruleError" />
        <small v-if="ruleError" class="text-red-500 break-words">{{ ruleError }}</small>
        <small v-else class="text-muted-color">
          {{ describe(rule) }}.
          <template v-if="times.length">Next: {{ times.slice(0, 3).map(formatTime).join(' · ') }}</template>
          <template v-else>No more times.</template>
        </small>
      </div>

      <div class="flex flex-col gap-4">
        <Button
          type="button"
          :label="advanced ? 'Hide advanced options' : 'Advanced options'"
          :icon="advanced ? 'pi pi-angle-up' : 'pi pi-angle-down'"
          iconPos="right"
          size="small"
          text
          class="self-start -ml-3"
          @click="advanced = !advanced"
        />
        <div v-show="advanced" class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <small class="sm:col-span-2 text-muted-color">Durations are Go's, such as 250ms or 1m30s.</small>
          <div v-for="d in durations" :key="d.key" class="flex flex-col gap-2">
            <label :for="d.key" class="font-medium">{{ d.label }}</label>
            <InputText :id="d.key" v-model="options[d.key]" :placeholder="d.placeholder" fluid />
            <small class="text-muted-color">{{ d.help }}</small>
          </div>
          <div class="flex flex-col gap-2 sm:col-span-2">
            <label for="params" class="font-medium">Params</label>
            <Textarea id="params" v-model="params" rows="2" autoResize spellcheck="false" class="font-mono text-sm" fluid :invalid="!!paramsError" />
            <small v-if="paramsError" class="text-red-500">{{ paramsError }}</small>
            <small v-else class="text-muted-color">JSON, handed to the operation as is.</small>
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <Button label="Cancel" severity="secondary" text @click="close" />
      <Button type="submit" form="schedule" :label="schedule ? 'Save' : 'Create'" :loading="saving" />
    </template>
  </Dialog>
</template>
