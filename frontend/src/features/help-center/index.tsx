import { useEffect, useState } from 'react'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { HELP_DOCUMENT_STORAGE_KEY, readHelpDocument } from './help-document'
import { MarkdownViewer } from './markdown-viewer'

export function HelpCenter() {
  const [markdown, setMarkdown] = useState(readHelpDocument)

  useEffect(() => {
    const handleStorage = (event: StorageEvent) => {
      if (event.key === HELP_DOCUMENT_STORAGE_KEY) {
        setMarkdown(readHelpDocument())
      }
    }

    window.addEventListener('storage', handleStorage)
    return () => window.removeEventListener('storage', handleStorage)
  }, [])

  return (
    <>
      <Header>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ConfigDrawer />
        <ProfileDropdown />
      </Header>

      <Main fixed>
        <div className='space-y-0.5'>
          <h1 className='text-2xl font-bold tracking-tight md:text-3xl'>
            帮助中心
          </h1>
          <p className='text-muted-foreground'>
            查看系统使用说明、图片资料和流程文档。
          </p>
        </div>
        <div className='faded-bottom mt-6 flex-1 overflow-y-auto scroll-smooth pb-12'>
          <MarkdownViewer markdown={markdown} />
        </div>
      </Main>
    </>
  )
}
