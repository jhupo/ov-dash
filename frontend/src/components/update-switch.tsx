import axios from 'axios'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  applyUpdate,
  checkUpdate,
  getUpdateStatus,
  restartUpdate,
  type UpdateRun,
  type UpdateStatus,
} from '@/services/updates'
import {
  CheckCircle2,
  CloudUpload,
  RefreshCw,
  RotateCw,
  UploadCloud,
} from 'lucide-react'
import { toast } from 'sonner'
import { useCan } from '@/hooks/use-can'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export function UpdateSwitch() {
  const queryClient = useQueryClient()
  const can = useCan()
  const canApplyUpdate = can('updates:apply')
  const status = useQuery({
    queryKey: ['updates'],
    queryFn: getUpdateStatus,
    refetchInterval: (query) => {
      const update = query.state.data?.update
      return update?.status === 'running' || update?.status === 'restarting'
        ? 2_000
        : false
    },
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
      toast.success('开始准备更新')
    },
    onError: (error) => toast.error(errorMessage(error, '启动更新失败')),
  })

  const restartMutation = useMutation({
    mutationFn: restartUpdate,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['updates'] })
      toast.success('服务正在重启')
    },
    onError: (error) => toast.error(errorMessage(error, '重启服务失败')),
  })

  const busy =
    status.isFetching ||
    checkMutation.isPending ||
    applyMutation.isPending ||
    restartMutation.isPending
  const canApply = Boolean(
    canApplyUpdate && value?.hasUpdate && !busy && !isActiveUpdate(update)
  )
  const canRestart = canApplyUpdate && update?.status === 'ready' && !busy
  const showUpdatePanel =
    Boolean(value?.hasUpdate) || Boolean(update && update.status !== 'success')

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger asChild>
        <Button
          variant='ghost'
          size='icon'
          className='relative rounded-full'
          aria-label='在线更新'
        >
          <CloudUpload className='size-[1.2rem]' />
          {(value?.hasUpdate || isActiveUpdate(update) || canRestart) && (
            <span className='absolute top-1.5 right-1.5 size-2 rounded-full bg-primary ring-2 ring-background' />
          )}
          <span className='sr-only'>在线更新</span>
        </Button>
      </DropdownMenuTrigger>

      <DropdownMenuContent align='end' className='w-[288px] p-2'>
        <div className='space-y-2'>
          <section className='rounded-lg border bg-card p-3 shadow-sm'>
            <div className='flex items-start justify-between gap-3'>
              <div className='flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary'>
                <CloudUpload className='size-4' />
              </div>
              <Button
                type='button'
                size='icon'
                variant='ghost'
                className='size-8 shrink-0 rounded-md border bg-background hover:bg-accent'
                disabled={busy}
                onClick={() => checkMutation.mutate()}
                aria-label='刷新版本'
              >
                <RefreshCw
                  className={busy ? 'size-4 animate-spin' : 'size-4'}
                />
              </Button>
            </div>

            <div className='mt-3 space-y-1'>
              <div className='text-xs text-muted-foreground'>当前版本</div>
              <div className='font-mono text-base leading-5 font-semibold break-all'>
                {versionText(value)}
              </div>
              <div className='flex items-center gap-1.5 text-xs text-muted-foreground'>
                <span className='size-1.5 rounded-full bg-emerald-500' />
                <span>{value?.checkedAt ? '已检查' : '未检查'}</span>
                {value?.checkedAt && (
                  <span className='font-mono'>
                    {formatDateTime(value.checkedAt)}
                  </span>
                )}
              </div>
            </div>
          </section>

          {showUpdatePanel && (
            <section className='space-y-3 rounded-lg border bg-card p-3 shadow-sm'>
              {value?.hasUpdate && (
                <div className='space-y-2'>
                  <div className='space-y-1'>
                    <div className='text-xs text-muted-foreground'>
                      更新版本
                    </div>
                    <div className='font-mono text-base leading-5 font-semibold break-all'>
                      {value.latestVersion}
                    </div>
                  </div>
                  {!isActiveUpdate(update) && update?.status !== 'ready' && (
                    <Button
                      type='button'
                      className='h-9 w-full'
                      disabled={!canApply}
                      onClick={() => applyMutation.mutate()}
                    >
                      <UploadCloud className='size-4' />
                      一键更新
                    </Button>
                  )}
                </div>
              )}

              {update && <UpdateProgress update={update} />}

              {canRestart && (
                <Button
                  type='button'
                  className='h-9 w-full'
                  disabled={!canApplyUpdate || restartMutation.isPending}
                  onClick={() => restartMutation.mutate()}
                >
                  <RotateCw className='size-4' />
                  重启服务
                </Button>
              )}
            </section>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function UpdateProgress({ update }: { update: UpdateRun }) {
  const progress = Math.max(0, Math.min(100, update.progress || 0))
  const isDone = update.status === 'ready' || update.status === 'success'
  const isError = update.status === 'error'

  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-3 text-sm'>
        <span className='min-w-0 truncate font-medium'>
          {statusText(update)}
        </span>
        {isDone ? (
          <CheckCircle2 className='size-4 shrink-0 text-emerald-500' />
        ) : (
          <span className='shrink-0 font-mono text-xs text-muted-foreground'>
            {progress}%
          </span>
        )}
      </div>
      <div className='h-2 overflow-hidden rounded-full bg-muted'>
        <div
          className={
            isError
              ? 'h-full bg-destructive transition-all'
              : 'h-full bg-primary transition-all'
          }
          style={{ width: `${isError ? 100 : progress}%` }}
        />
      </div>
      {update.status === 'ready' && (
        <div className='text-xs text-muted-foreground'>
          更新已准备完成，重启服务后切换到新版本。
        </div>
      )}
      {update.status === 'restarting' && (
        <div className='text-xs text-muted-foreground'>服务正在重启...</div>
      )}
      {isError && (
        <div className='max-h-20 overflow-auto rounded-md bg-destructive/10 p-2 text-xs text-destructive'>
          {update.message}
        </div>
      )}
    </div>
  )
}

function isActiveUpdate(update?: UpdateRun | null) {
  return update?.status === 'running' || update?.status === 'restarting'
}

function statusText(update: UpdateRun) {
  if (update.status === 'running') return update.message || '正在更新'
  if (update.status === 'ready') return '更新准备完成'
  if (update.status === 'restarting') return '正在重启服务'
  if (update.status === 'success') return '更新完成'
  if (update.status === 'error') return '更新失败'
  return update.message
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
