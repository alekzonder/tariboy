import { createContext, useContext } from "react"

export interface CustomerQuestionNotificationsValue {
  attention: ReadonlySet<string>
  refreshHost: (hostId: string) => Promise<void>
}

const emptyValue: CustomerQuestionNotificationsValue = {
  attention: new Set(),
  refreshHost: async () => {},
}

export const CustomerQuestionNotificationsContext = createContext<CustomerQuestionNotificationsValue>(emptyValue)

export function useCustomerQuestionNotifications(): CustomerQuestionNotificationsValue {
  return useContext(CustomerQuestionNotificationsContext)
}
