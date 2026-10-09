'use client'

import { useState } from 'react'
import { signIn } from 'next-auth/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Link2, Shield, Mail, Lock } from 'lucide-react'
import Link from 'next/link'
import { toast } from 'sonner'
import { useRouter } from 'next/navigation'

export function LoginClient({ oidcEnabled }: { oidcEnabled: boolean }) {
  // Client-owned integration point. Keep disabled until verified Authelia
  // identities are mapped to Go users in auth.ts's sign-in callback.
  const oidcReady = false
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const router = useRouter()

  const handleEmailLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    try {
      // signIn with redirect:false still rejects on network failures.
      const result = await signIn('credentials', { email, password, redirect: false })
      if (result?.ok) {
        router.push('/pages')
      } else {
        toast.error('Invalid credentials')
      }
    } catch {
      toast.error('Sign-in failed; check your connection')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <div className="w-full max-w-sm space-y-8">
        <div className="text-center">
          <div className="flex justify-center mb-4">
            <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
              <Link2 className="h-7 w-7" />
            </div>
          </div>
          <h1 className="font-display text-3xl font-bold tracking-tight">computer</h1>
          <p className="text-sm text-muted-foreground mt-2">Your personal binder of life on the web</p>
        </div>

        <div className="space-y-4">
          <Button
            onClick={() => oidcEnabled && signIn('authelia', { redirectTo: '/pages' })}
            className="w-full gap-2"
            size="lg"
            variant="default"
            disabled={!oidcEnabled || !oidcReady}
          >
            <Shield className="h-5 w-5" />
            Sign in with Authelia
          </Button>
          {(!oidcEnabled || !oidcReady) && (
            <p className="text-center text-xs text-muted-foreground -mt-2">
              Authelia single sign-on awaits client integration. Use email below.
            </p>
          )}

          <div className="relative">
            <div className="absolute inset-0 flex items-center"><div className="w-full border-t" /></div>
            <div className="relative flex justify-center text-xs"><span className="bg-background px-2 text-muted-foreground">or continue with email</span></div>
          </div>

          <form onSubmit={handleEmailLogin} className="space-y-3">
            <div className="relative">
              <Mail className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                type="email"
                placeholder="Email"
                value={email}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setEmail(e.target.value)}
                className="pl-10"
                required
              />
            </div>
            <div className="relative">
              <Lock className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                type="password"
                placeholder="Password"
                value={password}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setPassword(e.target.value)}
                className="pl-10"
                required
              />
            </div>
            <Button type="submit" variant="outline" className="w-full" loading={loading}>
              Sign in with Email
            </Button>
          </form>

          <p className="text-center text-xs text-muted-foreground">
            Don't have an account? <Link href="/signup" className="text-primary hover:underline">Sign up</Link>
          </p>
        </div>
      </div>
    </div>
  )
}
