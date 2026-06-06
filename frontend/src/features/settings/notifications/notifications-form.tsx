import { useEffect, useState } from 'react'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { zodResolver } from '@hookform/resolvers/zod'
import { Send, Save } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import {
  getTelegramNotificationSettings,
  getUserTelegramNotificationSettings,
  updateTelegramNotificationSettings,
  updateUserTelegramNotificationSettings,
  type UserTelegramNotificationSettings,
} from '@/services/telegram-notifications'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const telegramFormSchema = z.object({
  enabled: z.boolean(),
  botToken: z.string(),
  inboundToken: z.string(),
  clearBotToken: z.boolean(),
  clearInboundToken: z.boolean(),
})

type TelegramFormValues = z.infer<typeof telegramFormSchema>

const defaultValues: TelegramFormValues = {
  enabled: false,
  botToken: '',
  inboundToken: '',
  clearBotToken: false,
  clearInboundToken: false,
}

export function NotificationsForm() {
  const queryClient = useQueryClient()
  const settings = useQuery({
    queryKey: ['telegram-notifications', 'settings'],
    queryFn: getTelegramNotificationSettings,
  })
  const users = useQuery({
    queryKey: ['telegram-notifications', 'users'],
    queryFn: getUserTelegramNotificationSettings,
  })

  const form = useForm<TelegramFormValues>({
    resolver: zodResolver(telegramFormSchema),
    defaultValues,
  })

  useEffect(() => {
    if (!settings.data) return
    form.reset({
      enabled: settings.data.enabled,
      botToken: '',
      inboundToken: '',
      clearBotToken: false,
      clearInboundToken: false,
    })
  }, [form, settings.data])

  const settingsMutation = useMutation({
    mutationFn: updateTelegramNotificationSettings,
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ['telegram-notifications', 'settings'],
      })
      form.reset({ ...form.getValues(), botToken: '', inboundToken: '' })
      toast.success('Telegram 通知配置已保存')
    },
    onError: () => {
      toast.error('Telegram 通知配置保存失败')
    },
  })

  function onSubmit(data: TelegramFormValues) {
    settingsMutation.mutate({
      enabled: data.enabled,
      ...(data.botToken.trim() ? { bot_token: data.botToken.trim() } : {}),
      ...(data.inboundToken.trim()
        ? { inbound_token: data.inboundToken.trim() }
        : {}),
      ...(!data.botToken.trim() && data.clearBotToken
        ? { clear_bot_token: true }
        : {}),
      ...(!data.inboundToken.trim() && data.clearInboundToken
        ? { clear_inbound_token: true }
        : {}),
    })
  }

  return (
    <div className='space-y-8'>
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-5'>
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                <div className='space-y-0.5'>
                  <FormLabel className='text-base'>启用 Telegram 上报</FormLabel>
                  <FormDescription>
                    外部消息传入后，系统会根据用户映射发送到对应的
                    Telegram 账号。
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />

          <div className='grid gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='botToken'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Bot Token</FormLabel>
                  <FormControl>
                    <Input
                      type='password'
                      autoComplete='new-password'
                      placeholder={
                        settings.data?.has_bot_token
                          ? '留空则保持已保存 Token'
                          : '123456:ABC...'
                      }
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    从 BotFather 获取，用于调用 Telegram Bot API。
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='inboundToken'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>入站接口 Token</FormLabel>
                  <FormControl>
                    <Input
                      type='password'
                      autoComplete='new-password'
                      placeholder={
                        settings.data?.has_inbound_token
                          ? '留空则保持已保存 Token'
                          : '给外部系统调用时使用'
                      }
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    调用消息入口时放在 Bearer Token 或 X-OV-Dash-Token。
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {(settings.data?.has_bot_token ||
            settings.data?.has_inbound_token) && (
            <div className='grid gap-3 md:grid-cols-2'>
              {settings.data?.has_bot_token && (
                <FormField
                  control={form.control}
                  name='clearBotToken'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-start'>
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={(checked) =>
                            field.onChange(!!checked)
                          }
                        />
                      </FormControl>
                      <div className='space-y-1 leading-none'>
                        <FormLabel>清除已保存 Bot Token</FormLabel>
                      </div>
                    </FormItem>
                  )}
                />
              )}
              {settings.data?.has_inbound_token && (
                <FormField
                  control={form.control}
                  name='clearInboundToken'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-start'>
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={(checked) =>
                            field.onChange(!!checked)
                          }
                        />
                      </FormControl>
                      <div className='space-y-1 leading-none'>
                        <FormLabel>清除已保存入站 Token</FormLabel>
                      </div>
                    </FormItem>
                  )}
                />
              )}
            </div>
          )}

          <Button
            type='submit'
            disabled={settings.isLoading || settingsMutation.isPending}
          >
            <Save className='me-2 size-4' />
            保存 Telegram 配置
          </Button>
        </form>
      </Form>

      <UserTelegramSettingsTable
        items={users.data ?? []}
        loading={users.isLoading}
      />
    </div>
  )
}

function UserTelegramSettingsTable({
  items,
  loading,
}: {
  items: UserTelegramNotificationSettings[]
  loading: boolean
}) {
  const queryClient = useQueryClient()
  const [drafts, setDrafts] = useState<
    Record<string, { enabled: boolean; chatId: string }>
  >({})

  useEffect(() => {
    const next: Record<string, { enabled: boolean; chatId: string }> = {}
    for (const item of items) {
      next[item.user_id] = {
        enabled: item.enabled,
        chatId: item.chat_id,
      }
    }
    setDrafts(next)
  }, [items])

  const mutation = useMutation({
    mutationFn: ({
      userId,
      enabled,
      chatId,
    }: {
      userId: string
      enabled: boolean
      chatId: string
    }) =>
      updateUserTelegramNotificationSettings(userId, {
        enabled,
        chat_id: chatId,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ['telegram-notifications', 'users'],
      })
      toast.success('用户 Telegram 映射已保存')
    },
    onError: () => {
      toast.error('用户 Telegram 映射保存失败')
    },
  })

  function updateDraft(
    userId: string,
    value: Partial<{ enabled: boolean; chatId: string }>
  ) {
    setDrafts((current) => ({
      ...current,
      [userId]: {
        enabled: current[userId]?.enabled ?? false,
        chatId: current[userId]?.chatId ?? '',
        ...value,
      },
    }))
  }

  return (
    <div className='space-y-3'>
      <div className='space-y-1'>
        <h3 className='text-lg font-medium'>用户 Telegram 映射</h3>
        <p className='text-sm text-muted-foreground'>
          每个系统用户配置一个 Telegram Chat ID。入站消息指定用户后，会发送到这里绑定的账号。
        </p>
      </div>

      <div className='rounded-md border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='w-[26%]'>用户</TableHead>
              <TableHead>Chat ID</TableHead>
              <TableHead className='w-24'>启用</TableHead>
              <TableHead className='w-24 text-right'>操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && (
              <TableRow>
                <TableCell colSpan={4} className='h-20 text-center'>
                  正在加载...
                </TableCell>
              </TableRow>
            )}
            {!loading && items.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className='h-20 text-center'>
                  暂无用户
                </TableCell>
              </TableRow>
            )}
            {items.map((item) => {
              const draft = drafts[item.user_id] ?? {
                enabled: item.enabled,
                chatId: item.chat_id,
              }
              return (
                <TableRow key={item.user_id}>
                  <TableCell>
                    <div className='font-medium'>
                      {item.full_name || item.username}
                    </div>
                    <div className='text-xs text-muted-foreground'>
                      @{item.username} · {item.email}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Input
                      value={draft.chatId}
                      onChange={(event) =>
                        updateDraft(item.user_id, {
                          chatId: event.target.value,
                        })
                      }
                      placeholder='例如 123456789'
                      autoComplete='off'
                    />
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={draft.enabled}
                      onCheckedChange={(checked) =>
                        updateDraft(item.user_id, { enabled: checked })
                      }
                    />
                  </TableCell>
                  <TableCell className='text-right'>
                    <Button
                      type='button'
                      size='sm'
                      disabled={mutation.isPending}
                      onClick={() =>
                        mutation.mutate({
                          userId: item.user_id,
                          enabled: draft.enabled,
                          chatId: draft.chatId.trim(),
                        })
                      }
                    >
                      <Send className='me-2 size-4' />
                      保存
                    </Button>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}
