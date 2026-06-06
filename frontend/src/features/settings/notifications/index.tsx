import { ContentSection } from '../components/content-section'
import { NotificationsForm } from './notifications-form'

export function SettingsNotifications() {
  return (
    <ContentSection
      title='Telegram 通知'
      desc='配置 Bot Token、入站消息 Token、群组通知，以及用户到 Telegram Chat ID 的映射。'
    >
      <NotificationsForm />
    </ContentSection>
  )
}
