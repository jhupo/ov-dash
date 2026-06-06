import { ContentSection } from '../components/content-section'
import { ProxyForm } from './proxy-form'

export function SettingsProxy() {
  return (
    <ContentSection
      title='代理'
      desc='配置后台统一使用的 SOCKS5 代理，供后续模块复用。'
    >
      <ProxyForm />
    </ContentSection>
  )
}
