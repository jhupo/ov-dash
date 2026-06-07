import { ContentSection } from '../components/content-section'
import { ProxyForm } from './proxy-form'

export function SettingsProxy() {
  return (
    <ContentSection title='代理' desc='配置 SOCKS5 代理连接。'>
      <ProxyForm />
    </ContentSection>
  )
}
