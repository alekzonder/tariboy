import { createContext, useContext } from "react"

export interface CustomerQuestionNotificationsValue {
  // Attention key -> number of that agent's tasks with an unread question.
  attention: ReadonlyMap<string, number>
  refreshHost: (hostId: string) => Promise<void>
}

const emptyValue: CustomerQuestionNotificationsValue = {
  attention: new Map(),
  refreshHost: async () => {},
}

export const CustomerQuestionNotificationsContext = createContext<CustomerQuestionNotificationsValue>(emptyValue)

export function useCustomerQuestionNotifications(): CustomerQuestionNotificationsValue {
  return useContext(CustomerQuestionNotificationsContext)
}
