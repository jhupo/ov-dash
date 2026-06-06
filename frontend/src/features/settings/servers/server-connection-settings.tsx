import { useEffect, useMemo, useState } from 'react'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { zodResolver } from '@hookform/resolvers/zod'
import {
  Edit3,
  KeyRound,
  LockKeyhole,
  Plus,
  Server,
  Trash2,
} from 'lucide-react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import {
  deleteServerConnection,
  listServerConnections,
  saveServerConnection,
  type ServerConnection,
  type ServerAuthType,
} from '@/services/server-connections'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

const formSchema = z.object({
  name: z.string().trim().min(1, '请输入名称。'),
  group_name: z.string().trim(),
  region: z.string().trim(),
  host: z.string().trim().min(1, '请输入服务器 IP 或域名。'),
  port: z
    .string()
    .trim()
    .refine((value) => {
      const port = Number(value)
      return Number.isInteger(port) && port >= 1 && port <= 65535
    }, '端口必须在 1 到 65535 之间。'),
  username: z.string().trim().min(1, '请输入用户名。'),
  auth_type: z.enum(['password', 'key']),
  password: z.string(),
  private_key: z.string(),
  passphrase: z.string(),
  clear_secret: z.boolean(),
})

type FormValues = z.infer<typeof formSchema>

const defaultValues: FormValues = {
  name: '',
  group_name: '',
  region: '',
  host: '',
  port: '22',
  username: 'root',
  auth_type: 'password',
  password: '',
  private_key: '',
  passphrase: '',
  clear_secret: false,
}

export function ServerConnectionSettings() {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<ServerConnection | null>(null)

  const query = useQuery({
    queryKey: ['server-connections'],
    queryFn: listServerConnections,
  })

  const groupedItems = useMemo(() => {
    return (query.data ?? []).reduce<Record<string, ServerConnection[]>>(
      (groups, item) => {
        const key = item.group_name || '默认'
        groups[key] = groups[key] ?? []
        groups[key].push(item)
        return groups
      },
      {}
    )
  }, [query.data])

  const saveMutation = useMutation({
    mutationFn: saveServerConnection,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['server-connections'] })
      setOpen(false)
      setEditing(null)
      toast.success('服务器设置已保存')
    },
    onError: () => {
      toast.error('服务器设置保存失败')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteServerConnection,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['server-connections'] })
      toast.success('服务器已删除')
    },
    onError: () => {
      toast.error('服务器删除失败')
    },
  })

  function handleCreate() {
    setEditing(null)
    setOpen(true)
  }

  function handleEdit(item: ServerConnection) {
    setEditing(item)
    setOpen(true)
  }

  return (
    <div className='space-y-5'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='text-sm text-muted-foreground'>
          已配置 {query.data?.length ?? 0} 台服务器
        </div>
        <Button type='button' size='sm' onClick={handleCreate}>
          <Plus />
          添加服务器
        </Button>
      </div>

      {query.data?.length ? (
        <div className='grid gap-4 lg:grid-cols-2'>
          {Object.entries(groupedItems).map(([group, items]) =>
            items.map((item) => (
              <Card key={item.id} className='overflow-hidden'>
                <CardContent className='p-4'>
                  <div className='flex items-start justify-between gap-4'>
                    <div className='min-w-0 space-y-2'>
                      <div className='flex min-w-0 items-center gap-2'>
                        <Server className='size-4 shrink-0 text-muted-foreground' />
                        <span className='truncate font-semibold'>
                          {item.name}
                        </span>
                        <Badge variant='secondary' className='shrink-0'>
                          {group}
                        </Badge>
                      </div>
                      <div className='grid gap-1 text-sm text-muted-foreground'>
                        <span className='truncate'>
                          {item.username}@{item.host}:{item.port}
                        </span>
                        <span className='truncate'>
                          {item.region || '未设置地区'}
                        </span>
                      </div>
                    </div>
                    <div className='flex shrink-0 gap-1'>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon'
                        onClick={() => handleEdit(item)}
                        aria-label='编辑服务器'
                      >
                        <Edit3 />
                      </Button>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon'
                        disabled={deleteMutation.isPending}
                        onClick={() => deleteMutation.mutate(item.id)}
                        aria-label='删除服务器'
                      >
                        <Trash2 className='text-destructive' />
                      </Button>
                    </div>
                  </div>
                  <div className='mt-4 flex flex-wrap gap-2'>
                    <CredentialBadge
                      type={item.auth_type}
                      active={item.has_password || item.has_private_key}
                    />
                    {item.has_passphrase && (
                      <Badge variant='outline'>密钥口令</Badge>
                    )}
                  </div>
                </CardContent>
              </Card>
            ))
          )}
        </div>
      ) : (
        <div className='flex min-h-56 items-center justify-center rounded-md border border-dashed'>
          <Button type='button' variant='outline' onClick={handleCreate}>
            <Plus />
            添加第一台服务器
          </Button>
        </div>
      )}

      <ServerConnectionDialog
        open={open}
        item={editing}
        isSaving={saveMutation.isPending}
        onOpenChange={setOpen}
        onSave={(values) => {
          saveMutation.mutate({
            id: editing?.id,
            name: values.name.trim(),
            group_name: values.group_name.trim(),
            region: values.region.trim(),
            host: values.host.trim(),
            port: Number(values.port),
            username: values.username.trim(),
            auth_type: values.auth_type,
            ...(values.password ? { password: values.password } : {}),
            ...(values.private_key ? { private_key: values.private_key } : {}),
            ...(values.passphrase ? { passphrase: values.passphrase } : {}),
            ...(values.clear_secret ? { clear_secret: true } : {}),
          })
        }}
      />
    </div>
  )
}

function CredentialBadge({
  type,
  active,
}: {
  type: ServerAuthType
  active: boolean
}) {
  const Icon = type === 'key' ? KeyRound : LockKeyhole
  const label = type === 'key' ? '密钥登录' : '密码登录'

  return (
    <Badge variant={active ? 'default' : 'secondary'} className='gap-1'>
      <Icon className='size-3' />
      {label}
    </Badge>
  )
}

function ServerConnectionDialog({
  open,
  item,
  isSaving,
  onOpenChange,
  onSave,
}: {
  open: boolean
  item: ServerConnection | null
  isSaving: boolean
  onOpenChange: (open: boolean) => void
  onSave: (values: FormValues) => void
}) {
  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues,
  })
  const authType = form.watch('auth_type')

  useEffect(() => {
    if (!open) return

    form.reset(
      item
        ? {
            name: item.name,
            group_name: item.group_name,
            region: item.region,
            host: item.host,
            port: String(item.port || 22),
            username: item.username,
            auth_type: item.auth_type,
            password: '',
            private_key: '',
            passphrase: '',
            clear_secret: false,
          }
        : defaultValues
    )
  }, [form, item, open])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[92vh] overflow-y-auto sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{item ? '编辑服务器' : '添加服务器'}</DialogTitle>
        </DialogHeader>

        <Form {...form}>
          <form
            id='server-connection-form'
            onSubmit={form.handleSubmit(onSave)}
            className='space-y-5'
          >
            <div className='grid gap-4 md:grid-cols-3'>
              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>名称</FormLabel>
                    <FormControl>
                      <Input autoComplete='off' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='group_name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>分组</FormLabel>
                    <FormControl>
                      <Input autoComplete='off' placeholder='默认' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='region'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>地区</FormLabel>
                    <FormControl>
                      <Input
                        autoComplete='off'
                        placeholder='HK / US'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <div className='grid gap-4 md:grid-cols-[minmax(0,1fr)_9rem]'>
              <FormField
                control={form.control}
                name='host'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>服务器 IP / 域名</FormLabel>
                    <FormControl>
                      <Input autoComplete='off' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='port'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>端口</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        max={65535}
                        inputMode='numeric'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <div className='grid gap-4 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='username'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>用户名</FormLabel>
                    <FormControl>
                      <Input autoComplete='off' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='auth_type'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>登录方式</FormLabel>
                    <Select
                      value={field.value}
                      onValueChange={(value) => field.onChange(value)}
                    >
                      <FormControl>
                        <SelectTrigger className='w-full'>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectItem value='password'>密码</SelectItem>
                        <SelectItem value='key'>密钥</SelectItem>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            {authType === 'password' ? (
              <FormField
                control={form.control}
                name='password'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>密码</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        autoComplete='new-password'
                        placeholder={item?.has_password ? '留空则保持原密码' : ''}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ) : (
              <div className='grid gap-4 md:grid-cols-[minmax(0,1fr)_16rem]'>
                <FormField
                  control={form.control}
                  name='private_key'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>私钥</FormLabel>
                      <FormControl>
                        <Textarea
                          className='min-h-40 font-mono text-xs leading-5'
                          spellCheck={false}
                          placeholder={
                            item?.has_private_key ? '留空则保持原私钥' : ''
                          }
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='passphrase'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>密钥口令</FormLabel>
                      <FormControl>
                        <Input
                          type='password'
                          autoComplete='new-password'
                          placeholder={
                            item?.has_passphrase ? '留空则保持原口令' : ''
                          }
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            )}

            {item && (item.has_password || item.has_private_key) && (
              <FormField
                control={form.control}
                name='clear_secret'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-start gap-2'>
                    <FormControl>
                      <Checkbox
                        checked={field.value}
                        onCheckedChange={(checked) => field.onChange(!!checked)}
                      />
                    </FormControl>
                    <FormLabel>清除已保存凭据</FormLabel>
                  </FormItem>
                )}
              />
            )}
          </form>
        </Form>

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            取消
          </Button>
          <Button
            type='submit'
            form='server-connection-form'
            disabled={isSaving}
          >
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
