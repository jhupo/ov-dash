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
        <div className='faded-bottom flex-1 overflow-y-auto scroll-smooth pb-12'>
          <MarkdownViewer markdown={markdown} />
        </div>
      </Main>
    </>
  )
}
