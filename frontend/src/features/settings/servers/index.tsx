import { ContentSection } from '../components/content-section'
import { ServerConnectionSettings } from './server-connection-settings'

export function SettingsServers() {
  return (
    <ContentSection title='服务器' desc='管理服务器连接配置。' size='wide'>
      <ServerConnectionSettings />
    </ContentSection>
  )
}
