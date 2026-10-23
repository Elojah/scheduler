import { useToast } from 'primevue/usetoast'
import { APIError } from '@/utils/api'

// useFail returns a func that shows an error in a toast.
export const useFail = () => {
  const toast = useToast()
  return (e: unknown) => {
    toast.add({
      severity: 'error',
      summary: e instanceof APIError ? `Refused: ${e.code}` : 'Failed',
      detail: e instanceof Error ? e.message : String(e),
      life: 6000
    })
  }
}
