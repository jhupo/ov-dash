export type Convo = {
  id: string
  sender: string
  message: string
  timestamp: string | Date
}

export type ChatUser = {
  id: string
  fullName: string
  username: string
  profile: string
  title: string
  messages: Convo[]
}
