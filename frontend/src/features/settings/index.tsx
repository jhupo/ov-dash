import { Outlet, useLocation } from '@tanstack/react-router'
import {
  getSettingsNavItems,
  isSystemSettingsPath,
} from '@/services/module-registry'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { SidebarNav } from './components/sidebar-nav'

export function Settings() {
  const pathname = useLocation({ select: (location) => location.pathname })
  const navItems = getSettingsNavItems(
    isSystemSettingsPath(pathname) ? 'system' : 'profile'
  )

  return (
    <>
      <Header>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ConfigDrawer />
        <ProfileDropdown />
      </Header>

      <Main fixed>
        <div className='flex flex-1 flex-col gap-4 overflow-hidden'>
          <SidebarNav items={navItems} />
          <div className='flex w-full flex-1 overflow-y-hidden p-1'>
            <Outlet />
          </div>
        </div>
      </Main>
    </>
  )
}
