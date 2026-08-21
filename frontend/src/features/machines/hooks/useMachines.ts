import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getMachine, listMachines } from '@features/machines/services/machinesApi'
import { createMachine, type CreateMachineInput } from '@features/machines/services/machinesApi'

export function useMachines() {
  return useQuery({ queryKey: ['machines'], queryFn: listMachines })
}

export function useMachine(id: string) {
  return useQuery({ queryKey: ['machines', id], queryFn: () => getMachine(id) })
}

export function useCreateMachine() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateMachineInput) => createMachine(input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['machines'] })
    },
  })
}
