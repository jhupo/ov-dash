import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2, Lock } from 'lucide-react'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { changePassword } from '@/services/auth'
import { Button } from '@/components/ui/button'
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
import { PasswordInput } from '@/components/password-input'

const accountFormSchema = z
  .object({
    email: z.string(),
    current_password: z.string().min(1, '请输入当前密码'),
    new_password: z.string().min(8, '新密码至少需要 8 个字符'),
    confirm_password: z.string().min(1, '请再次输入新密码'),
  })
  .refine((data) => data.new_password === data.confirm_password, {
    message: '两次输入的新密码不一致',
    path: ['confirm_password'],
  })

type AccountFormValues = z.infer<typeof accountFormSchema>

export function AccountForm() {
  const user = useAuthStore((state) => state.auth.user)

  const form = useForm<AccountFormValues>({
    resolver: zodResolver(accountFormSchema),
    defaultValues: {
      email: user?.email || 'classicriver@jhupo.com',
      current_password: '',
      new_password: '',
      confirm_password: '',
    },
  })

  async function onSubmit(data: AccountFormValues) {
    try {
      await changePassword({
        current_password: data.current_password,
        new_password: data.new_password,
      })
      form.reset({
        email: data.email,
        current_password: '',
        new_password: '',
        confirm_password: '',
      })
      toast.success('密码已更新')
    } catch {
      toast.error('修改密码失败，请检查当前密码')
    }
  }

  const isSubmitting = form.formState.isSubmitting

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className='grid max-w-xl gap-6'
      >
        <FormField
          control={form.control}
          name='email'
          render={({ field }) => (
            <FormItem>
              <FormLabel>登录邮箱</FormLabel>
              <FormControl>
                <Input disabled {...field} />
              </FormControl>
              <FormDescription>
                账号邮箱由系统维护，当前页面只修改登录密码。
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <div className='grid gap-4 rounded-lg border p-4'>
          <div className='flex items-center gap-2 text-sm font-medium'>
            <Lock className='size-4' />
            修改密码
          </div>
          <FormField
            control={form.control}
            name='current_password'
            render={({ field }) => (
              <FormItem>
                <FormLabel>当前密码</FormLabel>
                <FormControl>
                  <PasswordInput placeholder='请输入当前密码' {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='new_password'
            render={({ field }) => (
              <FormItem>
                <FormLabel>新密码</FormLabel>
                <FormControl>
                  <PasswordInput placeholder='至少 8 个字符' {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='confirm_password'
            render={({ field }) => (
              <FormItem>
                <FormLabel>确认新密码</FormLabel>
                <FormControl>
                  <PasswordInput placeholder='再次输入新密码' {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>
        <Button type='submit' className='w-fit' disabled={isSubmitting}>
          {isSubmitting ? <Loader2 className='animate-spin' /> : <Lock />}
          保存密码
        </Button>
      </form>
    </Form>
  )
}
