import { ContentSection } from '../components/content-section'
import { AccountForm } from './account-form'

export function SettingsAccount() {
  return (
    <ContentSection
      title='个人设置'
      desc='管理账号信息和登录密码。'
    >
      <AccountForm />
    </ContentSection>
  )
}
