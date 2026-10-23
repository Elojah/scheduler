<script setup lang="ts">
import { ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import Fieldset from 'primevue/fieldset'
import Tag from 'primevue/tag'
import { useScheduleStore } from '@/stores/schedule'
import { useFail } from '@/composables/useFail'
import { presets } from '@/utils/rule'
import { formatTime } from '@/utils/date'
import type { Options, Schedule } from '@/types/schedule'

const props = defineProps<{ visible: boolean; schedule?: Schedule }>()
const emit = defineEmits<{ 'update:visible': [visible: boolean] }>()

const store = useScheduleStore()
const fail = useFail()

// durations are the options the alarm takes, as Go durations.
const durations = [
  { key: 'max_delay', label: 'Max delay', help: 'drops a time passed by more than it' },
  { key: 'coalesce', label: 'Coalesce', help: 'of a run of late times, runs only the latest' },
  { key: 'spin', label: 'Spin', help: 'busy-waits the last of it before each time' },
  { key: 'poll', label: 'Poll', help: 'bounds a wait, 1m by default' }
] as const

const rule = ref('')
const action = ref('')
const enabled = ref(true)
const options = ref<Record<(typeof durations)[number]['key'], string>>({ max_delay: '', coalesce: '', spin: '', poll: '' })
const params = ref('')
const paramsError = ref('')
const saving = ref(false)

// The form starts from the schedule edited, or from a daily rule.
watch(
  () => props.visible,
  (visible) => {
    if (!visible) {
      return
    }
    const s = props.schedule
    rule.value = s?.rule ?? presets()[2].rule
    action.value = s?.action ?? 'noop'
    enabled.value = s?.enabled ?? true
    options.value = { max_delay: s?.options.max_delay ?? '', coalesce: s?.options.coalesce ?? '', spin: s?.options.spin ?? '', poll: s?.options.poll ?? '' }
    params.value = s?.options.params === undefined ? '' : JSON.stringify(s.options.params, null, 2)
    paramsError.value = ''
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

const close = () => emit('update:visible', false)

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
      paramsError.value = `Params are not JSON: ${e instanceof Error ? e.message : e}`
      return
    }
  }
  paramsError.value = ''

  const s = { rule: rule.value.trim(), action: action.value.trim(), options: o, enabled: enabled.value }
  saving.value = true
  try {
    await (props.schedule ? store.update({ ...s, id: props.schedule.id }) : store.create(s))
    close()
  } catch (e) {
    fail(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog :visible="visible" modal :header="schedule ? 'Edit schedule' : 'Create schedule'" class="w-full max-w-2xl mx-4" @update:visible="emit('update:visible', $event)">
    <form class="flex flex-col gap-5" @submit.prevent="submit">
      <div class="flex flex-col gap-2">
        <label for="rule" class="font-medium">Rule</label>
        <div class="flex flex-wrap gap-2">
          <Button v-for="p in presets()" :key="p.label" type="button" :label="p.label" size="small" severity="secondary" outlined @click="rule = p.rule" />
        </div>
        <Textarea id="rule" v-model="rule" rows="3" autoResize spellcheck="false" class="!font-mono !text-sm" :invalid="!!ruleError" />
        <small class="text-surface-500">A DTSTART line, then RRULE, RDATE, EXDATE or EXRULE lines, as RFC 5545 writes them.</small>
        <small v-if="ruleError" class="text-red-500 break-all">{{ ruleError }}</small>
        <div v-else class="flex flex-wrap items-center gap-2 text-sm">
          <span class="text-surface-500">Next</span>
          <span v-if="!times.length">none: the rule has ended</span>
          <Tag v-for="t in times" :key="t" :value="formatTime(t)" severity="secondary" />
        </div>
      </div>

      <div class="flex flex-wrap items-end gap-6">
        <div class="flex flex-col gap-2 grow">
          <label for="action" class="font-medium">Action</label>
          <InputText id="action" v-model="action" />
        </div>
        <div class="flex items-center gap-2 pb-2">
          <ToggleSwitch v-model="enabled" inputId="enabled" />
          <label for="enabled" class="font-medium">Enabled</label>
        </div>
      </div>

      <Fieldset legend="Options" toggleable :collapsed="!schedule">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div v-for="d in durations" :key="d.key" class="flex flex-col gap-1">
            <label :for="d.key" class="font-medium">{{ d.label }}</label>
            <InputText :id="d.key" v-model="options[d.key]" placeholder="such as 250ms or 1m30s" />
            <small class="text-surface-500">{{ d.help }}</small>
          </div>
          <div class="flex flex-col gap-1 sm:col-span-2">
            <label for="params" class="font-medium">Params</label>
            <Textarea id="params" v-model="params" rows="2" autoResize spellcheck="false" class="!font-mono !text-sm" placeholder='{"relay": 1}' :invalid="!!paramsError" />
            <small v-if="paramsError" class="text-red-500">{{ paramsError }}</small>
            <small v-else class="text-surface-500">any JSON, handed to the action as is</small>
          </div>
        </div>
      </Fieldset>
      <!-- Enter submits. -->
      <button type="submit" class="hidden" />
    </form>

    <template #footer>
      <Button label="Cancel" icon="pi pi-times" severity="danger" outlined @click="close" />
      <Button :label="schedule ? 'Save' : 'Create'" icon="pi pi-check" severity="success" :loading="saving" @click="submit" />
    </template>
  </Dialog>
</template>
