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
} from 'lucide-react'
import {
  listServerConnections,
  type ServerConnection,
  type ServerMetric,
} from '@/services/server-connections'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'

type MetricRow = {
  label: string
  value: string
  percent: number
  tone: 'green' | 'yellow' | 'red' | 'muted'
}

export function ServerStatus() {
  const [activeGroup, setActiveGroup] = useState('全部')
  const [detail, setDetail] = useState<ServerConnection | null>(null)
  const servers = useQuery({
    queryKey: ['server-connections'],
    queryFn: listServerConnections,
    refetchInterval: 10_000,
  })

  const items = servers.data ?? []
  const groups = useMemo(() => {
    const names = new Set(items.map((item) => item.group_name || '默认'))
    return ['全部', ...Array.from(names)]
  }, [items])
  const filtered = useMemo(() => {
    if (activeGroup === '全部') return items
    return items.filter((item) => (item.group_name || '默认') === activeGroup)
  }, [activeGroup, items])
  const regions = useMemo(
    () => new Set(items.map((item) => item.region.trim()).filter(Boolean)).size,
    [items]
  )
  const online = items.filter((item) => item.collect_status === 'ok').length
  const traffic = items.reduce(
    (sum, item) => {
      sum.rx += item.metric?.network_rx_bytes ?? 0
      sum.tx += item.metric?.network_tx_bytes ?? 0
      sum.rxRate += item.metric?.network_rx_rate_bps ?? 0
      sum.txRate += item.metric?.network_tx_rate_bps ?? 0
      return sum
    },
    { rx: 0, tx: 0, rxRate: 0, txRate: 0 }
  )

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
            online={online}
            total={items.length}
            regions={regions}
            traffic={traffic}
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

          {filtered.length ? (
            <div className='grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4'>
              {filtered.map((server) => (
                <ServerCard
                  key={server.id}
                  server={server}
                  onDetail={() => setDetail(server)}
                />
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

      <ServerDetailDialog server={detail} onOpenChange={() => setDetail(null)} />
    </>
  )
}

function SummaryBar({
  online,
  total,
  regions,
  traffic,
  isFetching,
  onRefresh,
}: {
  online: number
  total: number
  regions: number
  traffic: { rx: number; tx: number; rxRate: number; txRate: number }
  isFetching: boolean
  onRefresh: () => void
}) {
  const time = new Date().toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })

  return (
    <div className='grid min-h-20 grid-cols-2 items-center gap-3 rounded-md border bg-card/80 px-4 py-3 shadow-sm md:grid-cols-5 xl:grid-cols-[1fr_1fr_1fr_1fr_1fr_auto]'>
      <SummaryItem label='当前时间' value={time} />
      <SummaryItem label='当前在线' value={`${online} / ${total}`} />
      <SummaryItem label='点亮地区' value={String(regions)} />
      <SummaryItem
        label='流量概览'
        value={`↑ ${formatBytes(traffic.tx)}`}
        subValue={`↓ ${formatBytes(traffic.rx)}`}
      />
      <SummaryItem
        label='网络速率'
        value={`↑ ${formatRate(traffic.txRate)}`}
        subValue={`↓ ${formatRate(traffic.rxRate)}`}
      />
      <Button
        type='button'
        variant='ghost'
        size='icon'
        className='ms-auto'
        onClick={onRefresh}
        aria-label='刷新服务器状态'
      >
        <RefreshCw className={isFetching ? 'animate-spin' : undefined} />
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

function ServerCard({
  server,
  onDetail,
}: {
  server: ServerConnection
  onDetail: () => void
}) {
  const metric = server.metric
  const rows = buildRows(metric)
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
            <IconValue icon={<Cpu />} value={metric ? 'CPU' : '待采集'} />
            <IconValue icon={<MemoryStick />} value={memoryText(metric)} />
            <IconValue icon={<HardDrive />} value={diskText(metric)} />
          </div>
        </div>
        <Button
          type='button'
          size='icon'
          variant='ghost'
          className='size-8 shrink-0'
          onClick={onDetail}
          aria-label='查看服务器详情'
        >
          <Info className='size-5' />
        </Button>
      </div>

      <div className='mt-3 space-y-3'>
        {rows.map((row) => (
          <ProgressRow key={row.label} row={row} />
        ))}
      </div>

      <div className='mt-4 grid gap-2 text-xs'>
        <InfoLine
          label='网络'
          value={
            metric
              ? `↑ ${formatRate(metric.network_tx_rate_bps)} / ↓ ${formatRate(metric.network_rx_rate_bps)}`
              : '待采集'
          }
        />
        <InfoLine
          label='流量'
          value={
            metric
              ? `↑ ${formatBytes(metric.network_tx_bytes)} / ↓ ${formatBytes(metric.network_rx_bytes)}`
              : '待采集'
          }
        />
        <InfoLine
          label='负载'
          value={
            metric
              ? `${metric.load1.toFixed(2)} | ${metric.load5.toFixed(2)} | ${metric.load15.toFixed(2)}`
              : '待采集'
          }
        />
      </div>

      <div className='mt-4 flex items-center justify-between gap-3 border-t pt-3 text-xs text-muted-foreground'>
        <span className='truncate'>到期: {formatDate(server.expires_at)}</span>
        <span className='h-4 w-px bg-border' />
        <span className='truncate'>{statusText(server)}</span>
      </div>
    </article>
  )
}

function ServerDetailDialog({
  server,
  onOpenChange,
}: {
  server: ServerConnection | null
  onOpenChange: () => void
}) {
  const metric = server?.metric
  return (
    <Dialog open={!!server} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-4xl'>
        <DialogHeader>
          <DialogTitle>{server?.name ?? '服务器详情'}</DialogTitle>
        </DialogHeader>
        {server && (
          <div className='grid gap-x-10 gap-y-5 text-sm md:grid-cols-3'>
            <DetailItem label='CPU' value={metric?.cpu_model || '待采集'} />
            <DetailItem label='架构' value={metric?.architecture || '待采集'} />
            <DetailItem
              label='虚拟化'
              value={metric?.virtualization || '待采集'}
            />
            <DetailItem label='GPU' value={metric?.gpu_model || 'None'} />
            <DetailItem label='操作系统' value={metric?.os_name || '待采集'} />
            <DetailItem
              label='内存'
              value={
                metric
                  ? `${formatBytes(metric.memory_used_bytes)} / ${formatBytes(metric.memory_total_bytes)}`
                  : '待采集'
              }
            />
            <DetailItem
              label='硬盘'
              value={
                metric
                  ? `${formatBytes(metric.disk_used_bytes)} / ${formatBytes(metric.disk_total_bytes)}`
                  : '待采集'
              }
            />
            <DetailItem
              label='实时网络'
              value={
                metric
                  ? `↑ ${formatRate(metric.network_tx_rate_bps)} / ↓ ${formatRate(metric.network_rx_rate_bps)}`
                  : '待采集'
              }
            />
            <DetailItem
              label='总流量'
              value={
                metric
                  ? `↑ ${formatBytes(metric.network_tx_bytes)} / ↓ ${formatBytes(metric.network_rx_bytes)}`
                  : '待采集'
              }
            />
            <DetailItem
              label='运行时间'
              value={metric ? formatDuration(metric.uptime_seconds) : '待采集'}
            />
            <DetailItem
              label='最后上报'
              value={server.last_collected_at ? formatDateTime(server.last_collected_at) : '待采集'}
            />
            <DetailItem label='到期时间' value={formatDate(server.expires_at)} />
            <DetailItem label='连接' value={server.connection_hint} />
            <DetailItem label='采集状态' value={statusText(server)} />
            <DetailItem
              label='错误'
              value={server.collect_error || '无'}
              className='md:col-span-3'
            />
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function DetailItem({
  label,
  value,
  className,
}: {
  label: string
  value: string
  className?: string
}) {
  return (
    <div className={className}>
      <div className='text-muted-foreground'>{label}</div>
      <div className='mt-1 break-words font-medium'>{value}</div>
    </div>
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

function ProgressRow({ row }: { row: MetricRow }) {
  const toneClass =
    row.tone === 'green'
      ? 'bg-emerald-500'
      : row.tone === 'yellow'
        ? 'bg-amber-400'
        : row.tone === 'red'
          ? 'bg-red-500'
          : 'bg-muted-foreground/25'

  return (
    <div className='grid grid-cols-[3.5rem_minmax(0,1fr)_4rem] items-center gap-3 text-sm'>
      <span>{row.label}</span>
      <div className='h-3 overflow-hidden rounded-full bg-muted'>
        <div
          className={`h-full rounded-full ${toneClass}`}
          style={{ width: `${Math.max(0, Math.min(100, row.percent))}%` }}
        />
      </div>
      <span className='text-right text-xs'>{row.value}</span>
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

function buildRows(metric: ServerMetric | null): MetricRow[] {
  if (!metric) {
    return ['CPU', '内存', 'SWAP', '硬盘'].map((label) => ({
      label,
      value: '--',
      percent: 0,
      tone: 'muted',
    }))
  }
  const memory = percent(metric.memory_used_bytes, metric.memory_total_bytes)
  const swap = percent(metric.swap_used_bytes, metric.swap_total_bytes)
  const disk = percent(metric.disk_used_bytes, metric.disk_total_bytes)
  return [
    {
      label: 'CPU',
      value: `${metric.cpu_percent.toFixed(0)}%`,
      percent: metric.cpu_percent,
      tone: tone(metric.cpu_percent),
    },
    {
      label: '内存',
      value: `${memory.toFixed(0)}%`,
      percent: memory,
      tone: tone(memory),
    },
    {
      label: 'SWAP',
      value: metric.swap_total_bytes ? `${swap.toFixed(0)}%` : 'OFF',
      percent: swap,
      tone: metric.swap_total_bytes ? tone(swap) : 'muted',
    },
    {
      label: '硬盘',
      value: `${disk.toFixed(0)}%`,
      percent: disk,
      tone: tone(disk),
    },
  ]
}

function percent(used: number, total: number) {
  if (!total) return 0
  return (used / total) * 100
}

function tone(value: number): MetricRow['tone'] {
  if (value >= 90) return 'red'
  if (value >= 60) return 'yellow'
  return 'green'
}

function memoryText(metric: ServerMetric | null) {
  if (!metric) return '待采集'
  return formatBytes(metric.memory_total_bytes)
}

function diskText(metric: ServerMetric | null) {
  if (!metric) return '待采集'
  return formatBytes(metric.disk_total_bytes)
}

function formatBytes(value: number) {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size >= 10 || index === 0 ? size.toFixed(0) : size.toFixed(2)} ${units[index]}`
}

function formatRate(value: number) {
  return `${formatBytes(value)}/s`
}

function formatDate(value: string | null) {
  if (!value) return '未设置'
  return new Date(value).toLocaleDateString('zh-CN')
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function formatDuration(seconds: number) {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  if (days > 0) return `${days}天${hours}小时`
  return `${hours}小时`
}

function statusText(server: ServerConnection) {
  if (server.collect_status === 'ok') return '在线'
  if (server.collect_status === 'collecting') return '采集中'
  if (server.collect_status === 'error') return '异常'
  return '等待采集'
}
