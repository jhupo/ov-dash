import { ContentSection } from '../components/content-section'
import { AccountForm } from './account-form'

export function SettingsAccount() {
  return (
    <ContentSection
      title='账号'
      desc='更新账号设置，配置偏好的语言和时区。'
    >
      <AccountForm />
    </ContentSection>
  )
}
