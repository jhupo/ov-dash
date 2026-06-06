import { useSearch } from '@tanstack/react-router'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { AuthLayout } from '../auth-layout'
import { UserAuthForm } from './components/user-auth-form'

export function SignIn() {
  const { redirect } = useSearch({ from: '/(auth)/sign-in' })

  return (
    <AuthLayout>
      <Card className='w-full gap-6 rounded-lg border-border/80 shadow-xl shadow-black/10'>
        <CardHeader className='px-8 pt-8 pb-0'>
          <CardTitle className='text-xl tracking-tight'>登录</CardTitle>
        </CardHeader>
        <CardContent className='px-8 pb-8'>
          <UserAuthForm redirectTo={redirect} />
        </CardContent>
      </Card>
    </AuthLayout>
  )
}
