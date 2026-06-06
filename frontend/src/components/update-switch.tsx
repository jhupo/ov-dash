import axios from 'axios'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Rocket, UploadCloud } from 'lucide-react'
import { toast } from 'sonner'
import {
  applyUpdate,
  checkUpdate,
  getUpdateStatus,
  type UpdateStatus,
} from '@/services/updates'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export function UpdateSwitch() {
  const queryClient = useQueryClient()
  const status = useQuery({
    queryKey: ['updates'],
    queryFn: getUpdateStatus,
    refetchInterval: (query) =>
      query.state.data?.status.updating ? 5_000 : false,
  })
  const value = status.data?.status
  const update = status.data?.update

  const checkMutation = useMutation({
    mutationFn: checkUpdate,
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: ['updates'] })
      toast[result.hasUpdate ? 'success' : 'info'](result.message)
    },
    onError: (error) => toast.error(errorMessage(error, '检查更新失败')),
  })

  const applyMutation = useMutation({
    mutationFn: applyUpdate,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['updates'] })
      toast.success('更新已开始，系统会自动拉取并重建')
    },
    onError: (error) => toast.error(errorMessage(error, '启动更新失败')),
  })

  const busy =
    status.isFetching || checkMutation.isPending || applyMutation.isPending

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger asChild>
        <Button variant='ghost' size='icon' className='rounded-full'>
          <UploadCloud className='size-[1.2rem]' />
          <span className='sr-only'>在线更新</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-80 p-0'>
        <DropdownMenuLabel className='flex items-center justify-between gap-3 px-4 py-3'>
          <span>在线更新</span>
          <StatusBadge status={value} />
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <div className='space-y-4 p-4 text-sm'>
          <div className='grid gap-2'>
            <InfoLine label='当前版本' value={versionText(value)} />
            <InfoLine label='最新版本' value={value?.latestVersion || '未检查'} />
            <InfoLine label='检查时间' value={formatDateTime(value?.checkedAt)} />
          </div>

          {value?.message && (
            <div className='rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground'>
              {value.message}
            </div>
          )}
          {update?.status === 'running' && (
            <div className='rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground'>
              正在更新到 {update.version}，请稍候。
            </div>
          )}
          {update?.status === 'error' && (
            <div className='max-h-24 overflow-auto rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive'>
              {update.message}
            </div>
          )}

          <div className='grid grid-cols-2 gap-2'>
            <Button
              type='button'
              variant='outline'
              disabled={busy}
              onClick={() => checkMutation.mutate()}
            >
              <RefreshCw className={busy ? 'animate-spin' : undefined} />
              刷新
            </Button>
            <Button
              type='button'
              disabled={!value?.hasUpdate || value.updating || busy}
              onClick={() => applyMutation.mutate()}
            >
              <Rocket />
              一键更新
            </Button>
          </div>
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function StatusBadge({ status }: { status?: UpdateStatus }) {
  if (status?.updating) return <Badge variant='secondary'>更新中</Badge>
  if (status?.hasUpdate) return <Badge>有新版本</Badge>
  return <Badge variant='outline'>当前版本</Badge>
}

function InfoLine({ label, value }: { label: string; value: string }) {
  return (
    <div className='flex items-center justify-between gap-3'>
      <span className='text-muted-foreground'>{label}</span>
      <span className='truncate font-medium tabular-nums'>{value}</span>
    </div>
  )
}

function versionText(status?: UpdateStatus) {
  if (!status) return '读取中'
  if (status.currentVersion && status.currentVersion !== 'local') {
    return status.currentVersion
  }
  return status.currentCommit ? `local (${status.currentCommit})` : 'local'
}

function formatDateTime(value?: string) {
  if (!value) return '未检查'
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function errorMessage(error: unknown, fallback: string) {
  if (axios.isAxiosError(error)) {
    const data = error.response?.data
    if (data && typeof data === 'object' && 'error' in data) {
      if ('message' in data && data.message) {
        return String(data.message)
      }
      return String(data.error)
    }
  }
  return fallback
}
