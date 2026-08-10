import { useState } from 'react'
import axios from 'axios'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  applyUpdate,
  checkUpdate,
  getUpdateOperation,
  getUpdateStatus,
  type InstalledRelease,
  type UpdateOperation,
  type UpdateOperationState,
} from '@/services/updates'
import { CheckCircle2, CloudUpload, RefreshCw, UploadCloud } from 'lucide-react'
import { toast } from 'sonner'
import { useCan } from '@/hooks/use-can'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

const operationProgress: Record<UpdateOperationState, number> = {
  requested: 5,
  downloaded: 35,
  verified: 65,
  switching: 85,
  committed: 100,
  failed: 100,
}

export function UpdateSwitch() {
  const queryClient = useQueryClient()
  const can = useCan()
  const canApplyUpdate = can('updates:apply')
  const [requestedOperationId, setRequestedOperationId] = useState<string>()

  const statusQuery = useQuery({
    queryKey: ['updates'],
    queryFn: getUpdateStatus,
    refetchInterval: (query) =>
      isActiveOperation(query.state.data?.operation) ? 2_000 : false,
  })
  const statusOperation = statusQuery.data?.operation
  const operationId = statusOperation?.id ?? requestedOperationId

  const operationQuery = useQuery({
    queryKey: ['updates', 'operations', operationId],
    queryFn: () => getUpdateOperation(operationId!),
    enabled: Boolean(operationId),
    refetchInterval: (query) =>
      isTerminalOperation(query.state.data) ? false : 2_000,
  })

  const checkMutation = useMutation({
    mutationFn: checkUpdate,
    onSuccess: (result) => {
      queryClient.setQueryData(['updates'], {
        current: result.current,
        operation: statusOperation ?? null,
      })
      if (result.has_update) {
        toast.success(`发现新版本 ${result.candidate.version}`)
      } else {
        toast.info('当前已是最新版本')
      }
    },
    onError: (error) => toast.error(errorMessage(error, '检查更新失败')),
  })

  const applyMutation = useMutation({
    mutationFn: applyUpdate,
    onSuccess: async (operation) => {
      setRequestedOperationId(operation.id)
      queryClient.setQueryData(
        ['updates', 'operations', operation.id],
        operation
      )
      await queryClient.invalidateQueries({ queryKey: ['updates'] })
      toast.success('更新任务已启动')
    },
    onError: (error) => toast.error(errorMessage(error, '启动更新失败')),
  })

  const current = statusQuery.data?.current ?? checkMutation.data?.current
  const candidate = checkMutation.data?.candidate
  const operation = operationQuery.data ?? statusOperation
  const hasUpdate = Boolean(
    checkMutation.data?.has_update &&
    candidate &&
    candidate.sequence > (current?.sequence ?? 0)
  )
  const busy = checkMutation.isPending || applyMutation.isPending
  const canApply = Boolean(
    canApplyUpdate &&
    candidate &&
    hasUpdate &&
    !busy &&
    !isActiveOperation(operation)
  )
  const showUpdatePanel = hasUpdate || Boolean(operation)

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
          {(hasUpdate || isActiveOperation(operation)) && (
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
                disabled={busy || isActiveOperation(operation)}
                onClick={() => checkMutation.mutate()}
                aria-label='检查更新'
              >
                <RefreshCw
                  className={
                    checkMutation.isPending ? 'size-4 animate-spin' : 'size-4'
                  }
                />
              </Button>
            </div>

            <div className='mt-3 space-y-1'>
              <div className='text-xs text-muted-foreground'>当前版本</div>
              <div className='font-mono text-base leading-5 font-semibold break-all'>
                {versionText(current)}
              </div>
              <div className='flex items-center gap-1.5 text-xs text-muted-foreground'>
                <span className='size-1.5 rounded-full bg-emerald-500' />
                <span>{current?.committed_at ? '已安装' : '读取中'}</span>
                {current?.committed_at && (
                  <span className='font-mono'>
                    {formatDateTime(current.committed_at)}
                  </span>
                )}
              </div>
            </div>
          </section>

          {showUpdatePanel && (
            <section className='space-y-3 rounded-lg border bg-card p-3 shadow-sm'>
              {hasUpdate && candidate && (
                <div className='space-y-2'>
                  <div className='space-y-1'>
                    <div className='text-xs text-muted-foreground'>
                      更新版本
                    </div>
                    <div className='font-mono text-base leading-5 font-semibold break-all'>
                      {candidate.version}
                    </div>
                  </div>
                  {!isActiveOperation(operation) && (
                    <Button
                      type='button'
                      className='h-9 w-full'
                      disabled={!canApply}
                      onClick={() => applyMutation.mutate(candidate.release_id)}
                    >
                      <UploadCloud className='size-4' />
                      一键更新
                    </Button>
                  )}
                </div>
              )}

              {operation && <UpdateProgress operation={operation} />}
            </section>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function UpdateProgress({ operation }: { operation: UpdateOperation }) {
  const progress = operationProgress[operation.state]
  const isDone = operation.state === 'committed'
  const isError = isFailedOperation(operation)

  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-3 text-sm'>
        <span className='min-w-0 truncate font-medium'>
          {statusText(operation.state)}
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
          style={{ width: `${progress}%` }}
        />
      </div>
      <div className='flex items-center justify-between gap-2 font-mono text-xs text-muted-foreground'>
        <span className='truncate'>{operation.release_id}</span>
        <span className='shrink-0'>#{operation.revision}</span>
      </div>
      {isError && (
        <div className='max-h-20 overflow-auto rounded-md bg-destructive/10 p-2 text-xs text-destructive'>
          {operation.last_error || '更新未完成'}
        </div>
      )}
    </div>
  )
}

function isTerminalOperation(operation?: UpdateOperation | null) {
  return operation
    ? ['committed', 'failed'].includes(operation.state)
    : false
}

function isActiveOperation(operation?: UpdateOperation | null) {
  return Boolean(operation && !isTerminalOperation(operation))
}

function isFailedOperation(operation: UpdateOperation) {
  return operation.state === 'failed'
}

function statusText(state: UpdateOperationState) {
  const labels: Record<UpdateOperationState, string> = {
    requested: '等待执行',
    downloaded: '已下载发布包',
    verified: '签名验证完成',
    switching: '正在切换版本',
    committed: '更新完成',
    failed: '更新失败',
  }
  return labels[state]
}

function versionText(release?: InstalledRelease) {
  if (!release) return '读取中'
  return release.version || release.release_id || 'local'
}

function formatDateTime(value: string) {
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
      if (
        data.error &&
        typeof data.error === 'object' &&
        'message' in data.error &&
        data.error.message
      ) {
        return String(data.error.message)
      }
      if ('message' in data && data.message) {
        return String(data.message)
      }
      return typeof data.error === 'string' ? data.error : fallback
    }
  }
  return fallback
}
