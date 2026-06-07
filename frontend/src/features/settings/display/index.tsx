import { ContentSection } from '../components/content-section'
import { DisplayForm } from './display-form'

export function SettingsDisplay() {
  return (
    <ContentSection title='显示' desc='选择应用中展示的侧边栏项目。'>
      <DisplayForm />
    </ContentSection>
  )
}
