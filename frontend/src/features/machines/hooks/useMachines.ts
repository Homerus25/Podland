import { useQuery } from '@tanstack/react-query'
import { listMachines } from '@features/machines/services/machinesApi'

export function useMachines() {
  return useQuery({ queryKey: ['machines'], queryFn: listMachines })
}
