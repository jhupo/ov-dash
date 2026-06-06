import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  Cpu,
  HardDrive,
  Info,
  MemoryStick,
  Network,
  RefreshCw,
  Settings2,
} from 'lucide-react'
import {
  listServerConnections,
  type ServerConnection,
} from '@/services/server-connections'
import { Button } from '@/components/ui/button'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'

type MetricState = {
  label: string
  value: string
  percent: number
  tone: 'green' | 'yellow' | 'muted'
}

export function ServerStatus() {
  const [activeGroup, setActiveGroup] = useState('全部')
  const servers = useQuery({
    queryKey: ['server-connections'],
    queryFn: listServerConnections,
    refetchInterval: 30_000,
  })

  const groups = useMemo(() => {
    const names = new Set<string>()
    for (const item of servers.data ?? []) {
      names.add(item.group_name || '默认')
    }
    return ['全部', ...Array.from(names)]
  }, [servers.data])

  const filteredServers = useMemo(() => {
    if (activeGroup === '全部') return servers.data ?? []
    return (servers.data ?? []).filter(
      (item) => (item.group_name || '默认') === activeGroup
    )
  }, [activeGroup, servers.data])

  const regionCount = useMemo(() => {
    const regions = new Set(
      (servers.data ?? []).map((item) => item.region.trim()).filter(Boolean)
    )
    return regions.size
  }, [servers.data])

  return (
    <>
      <Header>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ConfigDrawer />
        <ProfileDropdown />
      </Header>

      <Main>
        <div className='mx-auto flex w-full max-w-[1540px] flex-col gap-4'>
          <SummaryBar
            currentCount={servers.data?.length ?? 0}
            regionCount={regionCount}
            isFetching={servers.isFetching}
            onRefresh={() => servers.refetch()}
          />

          <div className='flex min-h-10 items-center gap-2 rounded-md border bg-card/70 px-3'>
            <span className='text-sm text-muted-foreground'>分组</span>
            <div className='flex flex-wrap gap-1'>
              {groups.map((group) => (
                <Button
                  key={group}
                  type='button'
                  size='sm'
                  variant={activeGroup === group ? 'secondary' : 'ghost'}
                  className='h-7 px-3'
                  onClick={() => setActiveGroup(group)}
                >
                  {group}
                </Button>
              ))}
            </div>
          </div>

          {filteredServers.length ? (
            <div className='grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4'>
              {filteredServers.map((server) => (
                <ServerCard key={server.id} server={server} />
              ))}
            </div>
          ) : (
            <div className='flex min-h-[360px] flex-col items-center justify-center gap-3 rounded-md border border-dashed bg-card/50'>
              <p className='text-sm text-muted-foreground'>
                还没有服务器配置
              </p>
              <Button asChild>
                <Link to='/settings/servers'>添加服务器</Link>
              </Button>
            </div>
          )}
        </div>
      </Main>
    </>
  )
}

function SummaryBar({
  currentCount,
  regionCount,
  isFetching,
  onRefresh,
}: {
  currentCount: number
  regionCount: number
  isFetching: boolean
  onRefresh: () => void
}) {
  const now = new Date()
  const time = now.toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })

  return (
    <div className='grid min-h-20 grid-cols-2 items-center gap-3 rounded-md border bg-card/80 px-4 py-3 shadow-sm md:grid-cols-5 xl:grid-cols-[1fr_1fr_1fr_1fr_1fr_auto]'>
      <SummaryItem label='当前时间' value={time} />
      <SummaryItem label='当前在线' value={`${currentCount} / ${currentCount}`} />
      <SummaryItem label='点亮地区' value={String(regionCount)} />
      <SummaryItem label='流量概览' value='待采集' subValue='待采集' />
      <SummaryItem label='网络速率' value='待采集' subValue='待采集' />
      <Button
        type='button'
        variant='ghost'
        size='icon'
        className='ms-auto'
        onClick={onRefresh}
        aria-label='刷新服务器状态'
      >
        {isFetching ? <RefreshCw className='animate-spin' /> : <Settings2 />}
      </Button>
    </div>
  )
}

function SummaryItem({
  label,
  value,
  subValue,
}: {
  label: string
  value: string
  subValue?: string
}) {
  return (
    <div className='min-w-0 text-center'>
      <div className='text-sm text-muted-foreground'>{label}</div>
      <div className='truncate text-sm font-semibold'>{value}</div>
      {subValue && (
        <div className='truncate text-sm font-semibold'>{subValue}</div>
      )}
    </div>
  )
}

function ServerCard({ server }: { server: ServerConnection }) {
  const metrics = getPendingMetrics()
  const group = server.group_name || '默认'

  return (
    <article className='min-h-[350px] rounded-md border bg-card/80 p-4 shadow-sm'>
      <div className='flex items-start justify-between gap-3 border-b pb-3'>
        <div className='min-w-0'>
          <div className='flex min-w-0 items-center gap-2'>
            <RegionMark region={server.region} />
            <span className='truncate font-semibold'>
              [{group}] {server.name}
            </span>
          </div>
          <div className='mt-2 grid grid-cols-3 gap-2 text-xs text-muted-foreground'>
            <IconValue icon={<Cpu />} value='待采集' />
            <IconValue icon={<MemoryStick />} value='待采集' />
            <IconValue icon={<HardDrive />} value='待采集' />
          </div>
        </div>
        <Info className='size-5 shrink-0 text-muted-foreground' />
      </div>

      <div className='mt-3 space-y-3'>
        {metrics.map((metric) => (
          <MetricRow key={metric.label} metric={metric} />
        ))}
      </div>

      <div className='mt-4 grid gap-2 text-xs'>
        <InfoLine label='网络' value='↑ 待采集 / ↓ 待采集' />
        <InfoLine label='流量' value='↑ 待采集 / ↓ 待采集' />
        <InfoLine label='负载' value='待采集' />
      </div>

      <div className='mt-4 flex items-center justify-between gap-3 border-t pt-3 text-xs text-muted-foreground'>
        <span className='truncate'>到期: 未设置</span>
        <span className='h-4 w-px bg-border' />
        <span className='truncate'>连接: {server.connection_hint}</span>
      </div>
    </article>
  )
}

function IconValue({
  icon,
  value,
}: {
  icon: ReactNode
  value: string
}) {
  return (
    <span className='flex min-w-0 items-center gap-1'>
      <span className='[&_svg]:size-3.5 [&_svg]:text-primary'>{icon}</span>
      <span className='truncate'>{value}</span>
    </span>
  )
}

function MetricRow({ metric }: { metric: MetricState }) {
  const toneClass =
    metric.tone === 'green'
      ? 'bg-emerald-500'
      : metric.tone === 'yellow'
        ? 'bg-amber-400'
        : 'bg-muted-foreground/25'

  return (
    <div className='grid grid-cols-[3.5rem_minmax(0,1fr)_4rem] items-center gap-3 text-sm'>
      <span>{metric.label}</span>
      <div className='h-3 overflow-hidden rounded-full bg-muted'>
        <div
          className={`h-full rounded-full ${toneClass}`}
          style={{ width: `${metric.percent}%` }}
        />
      </div>
      <span className='text-right text-xs'>{metric.value}</span>
    </div>
  )
}

function InfoLine({ label, value }: { label: string; value: string }) {
  return (
    <div className='grid grid-cols-[3.5rem_minmax(0,1fr)] gap-3'>
      <span>{label}</span>
      <span className='truncate text-right text-muted-foreground'>{value}</span>
    </div>
  )
}

function RegionMark({ region }: { region: string }) {
  const normalized = region.trim().toLowerCase()
  const flag = normalized.includes('hk')
    ? '🇭🇰'
    : normalized.includes('us') || normalized.includes('usa')
      ? '🇺🇸'
      : normalized.includes('jp')
        ? '🇯🇵'
        : ''

  if (flag) return <span className='text-lg leading-none'>{flag}</span>

  return (
    <span className='flex size-5 items-center justify-center rounded-sm bg-primary/10 text-primary'>
      <Network className='size-3.5' />
    </span>
  )
}

function getPendingMetrics(): MetricState[] {
  return [
    { label: 'CPU', value: '--', percent: 0, tone: 'muted' },
    { label: '内存', value: '--', percent: 0, tone: 'muted' },
    { label: 'SWAP', value: '--', percent: 0, tone: 'muted' },
    { label: '硬盘', value: '--', percent: 0, tone: 'muted' },
  ]
}
