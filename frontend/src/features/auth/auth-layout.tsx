import { Logo } from '@/assets/logo'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout({ children }: AuthLayoutProps) {
  return (
    <div className='grid min-h-svh place-items-center px-6 py-10'>
      <div className='flex w-full max-w-[420px] -translate-y-6 flex-col justify-center sm:-translate-y-10'>
        <div className='mb-6 flex items-center justify-center'>
          <Logo className='me-2' />
          <h1 className='text-2xl font-semibold tracking-tight'>OV Dash</h1>
        </div>
        {children}
      </div>
    </div>
  )
}
