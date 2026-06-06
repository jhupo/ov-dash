import { ContentSection } from '../components/content-section'
import { HelpCenterForm } from './help-center-form'

export function SettingsHelpCenter() {
  return (
    <ContentSection
      title='帮助中心设置'
      desc='编辑帮助中心展示的 Markdown 文档内容。'
      size='wide'
    >
      <HelpCenterForm />
    </ContentSection>
  )
}
