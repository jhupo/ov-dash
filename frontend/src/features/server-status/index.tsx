import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  Database,
  RefreshCw,
  Server,
  Wifi,
  type LucideIcon,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { getBackendHealth } from '@/services/health'

export function ServerStatus() {
  const health = useQuery({
    queryKey: ['backend-health'],
    queryFn: ({ signal }) => getBackendHealth(signal),
    refetchInterval: 10_000,
  })

  const status = health.isError ? 'down' : health.data?.status
  const services = health.data?.checks ?? health.data?.services ?? {}

  return (
    <>
      <Header>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ConfigDrawer />
        <ProfileDropdown />
      </Header>

      <Main>
        <div className='mb-4 flex flex-wrap items-end justify-between gap-3'>
          <div>
            <h1 className='text-2xl font-bold tracking-tight'>服务器状态</h1>
            <p className='text-muted-foreground'>
              查看后端接口、数据库和队列连接状态。
            </p>
          </div>
          <Button
            variant='outline'
            size='sm'
            onClick={() => health.refetch()}
            disabled={health.isFetching}
          >
            <RefreshCw
              className={health.isFetching ? 'animate-spin' : undefined}
            />
            刷新
          </Button>
        </div>

        <div className='grid gap-4 md:grid-cols-3'>
          <StatusCard
            icon={Server}
            title='API 服务'
            description='Go HTTP 服务'
            status={status}
            isLoading={health.isLoading}
          />
          <StatusCard
            icon={Database}
            title='PostgreSQL'
            description='主业务数据库'
            status={services.postgres}
            isLoading={health.isLoading}
          />
          <StatusCard
            icon={Wifi}
            title='Redis'
            description='任务队列与缓存'
            status={services.redis}
            isLoading={health.isLoading}
          />
        </div>
      </Main>
    </>
  )
}

function StatusCard({
  icon: Icon,
  title,
  description,
  status,
  isLoading,
}: {
  icon: LucideIcon
  title: string
  description: string
  status?: string
  isLoading: boolean
}) {
  return (
    <Card>
      <CardHeader className='flex flex-row items-start justify-between gap-4 space-y-0'>
        <div className='space-y-1'>
          <CardTitle className='flex items-center gap-2 text-base'>
            <Icon className='size-4 text-muted-foreground' />
            {title}
          </CardTitle>
          <CardDescription>{description}</CardDescription>
        </div>
        <StatusBadge status={status} isLoading={isLoading} />
      </CardHeader>
      <CardContent>
        <div className='flex items-center gap-2 text-sm text-muted-foreground'>
          <Activity className='size-4' />
          <span>状态：{statusLabel(status, isLoading)}</span>
        </div>
      </CardContent>
    </Card>
  )
}

function StatusBadge({
  status,
  isLoading,
}: {
  status?: string
  isLoading: boolean
}) {
  const isOK = status === 'ok' && !isLoading

  return (
    <Badge variant={isOK ? 'default' : 'secondary'}>
      {statusLabel(status, isLoading)}
    </Badge>
  )
}

function statusLabel(status: string | undefined, isLoading: boolean) {
  if (isLoading) return '检查中'
  if (status === 'ok') return '正常'
  if (status === 'degraded') return '降级'
  if (status === 'down') return '异常'
  return '未知'
}
