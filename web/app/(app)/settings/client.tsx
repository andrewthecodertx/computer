'use client'

import { useState, useEffect, useCallback } from 'react'
import { useSession } from 'next-auth/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { User, Mail, Server, Cloud, Shield, Check, X, AlertTriangle } from 'lucide-react'
import { toast } from 'sonner'
import { invalidateReference } from '@/lib/reference-data'
import type { OidcStatus } from '@/lib/oidc-config'

export function SettingsClient({ oidc, callbackUrl }: { oidc: OidcStatus; callbackUrl: string }) {
  const { data: session } = useSession()
  const [imapConfig, setImapConfig] = useState({ host: '', port: 993, tls: true, username: '', password: '', folder: 'INBOX' })
  const [ncConfig, setNcConfig] = useState({ nextcloudUrl: '', cardDavPath: '', username: '', password: '' })
  const [savingImap, setSavingImap] = useState(false)
  const [testingImap, setTestingImap] = useState(false)
  const [syncingContacts, setSyncingContacts] = useState(false)

  useEffect(() => {
    fetch('/api/settings/imap').then(r => r.json()).then(data => {
      if (data) setImapConfig(prev => ({ ...prev, host: data.host ?? '', port: data.port ?? 993, tls: data.tls ?? true, username: data.username ?? '', folder: data.folder ?? 'INBOX' }))
    }).catch(() => {})
  }, [])

  // Every action clears its spinner in finally and tolerates non-JSON or
  // failed network responses, so no state can stick after an outage.
  const saveImap = async () => {
    setSavingImap(true)
    try {
      const res = await fetch('/api/settings/imap', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(imapConfig),
      })
      if (res.ok) toast.success('IMAP settings saved')
      else toast.error((await res.json().catch(() => null))?.error ?? 'Failed to save')
    } catch { toast.error('Failed to save') }
    finally { setSavingImap(false) }
  }

  const testImap = async () => {
    setTestingImap(true)
    try {
      const res = await fetch('/api/imap/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(imapConfig),
      })
      const data = await res.json().catch(() => null)
      if (res.ok && data?.ok) toast.success('Connection successful!')
      else toast.error(data?.error ?? 'Connection failed')
    } catch { toast.error('Connection failed') }
    finally { setTestingImap(false) }
  }

  const syncContacts = async () => {
    if (!ncConfig.nextcloudUrl || !ncConfig.username || !ncConfig.password) {
      toast.error('Fill in Nextcloud URL, username, and password')
      return
    }
    setSyncingContacts(true)
    try {
      const res = await fetch('/api/contacts/sync', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(ncConfig),
      })
      const data = await res.json().catch(() => null)
      if (res.ok && data?.synced !== undefined) { toast.success(`Synced ${data.synced} contacts`); invalidateReference('contacts') }
      else toast.error(data?.error ?? 'Sync failed')
    } catch { toast.error('Sync failed') }
    finally { setSyncingContacts(false) }
  }

  return (
    <div className="max-w-2xl space-y-6">
      <div>
        <h1 className="font-display text-2xl font-bold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground mt-1">Configure your integrations and preferences</p>
      </div>

      <Tabs defaultValue="profile" className="w-full">
        <TabsList className="w-full justify-start">
          <TabsTrigger value="profile" className="gap-1.5"><User className="h-3.5 w-3.5" /> Profile</TabsTrigger>
          <TabsTrigger value="imap" className="gap-1.5"><Mail className="h-3.5 w-3.5" /> IMAP</TabsTrigger>
          <TabsTrigger value="nextcloud" className="gap-1.5"><Cloud className="h-3.5 w-3.5" /> Nextcloud</TabsTrigger>
          <TabsTrigger value="oidc" className="gap-1.5"><Shield className="h-3.5 w-3.5" /> OIDC</TabsTrigger>
        </TabsList>

        <TabsContent value="profile" className="space-y-4 mt-4">
          <div className="flex items-center gap-4">
            <div className="h-16 w-16 rounded-full bg-primary/20 flex items-center justify-center text-2xl font-bold text-primary">
              {(session?.user?.name ?? session?.user?.email ?? '?')[0]?.toUpperCase()}
            </div>
            <div>
              <p className="font-medium">{session?.user?.name ?? 'User'}</p>
              <p className="text-sm text-muted-foreground" suppressHydrationWarning>{session?.user?.email}</p>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="imap" className="space-y-4 mt-4">
          <div className="rounded-lg border p-4 space-y-3">
            <h3 className="font-medium flex items-center gap-2"><Server className="h-4 w-4" /> IMAP Configuration</h3>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs font-medium text-muted-foreground">Host</label>
                <Input value={imapConfig.host} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setImapConfig(p => ({ ...p, host: e.target.value }))} placeholder="imap.example.com" />
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">Port</label>
                <Input type="number" value={imapConfig.port} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setImapConfig(p => ({ ...p, port: parseInt(e.target.value) || 993 }))} />
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">Username</label>
                <Input value={imapConfig.username} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setImapConfig(p => ({ ...p, username: e.target.value }))} />
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">Password</label>
                <Input type="password" value={imapConfig.password} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setImapConfig(p => ({ ...p, password: e.target.value }))} />
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">Folder</label>
                <Input value={imapConfig.folder} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setImapConfig(p => ({ ...p, folder: e.target.value }))} />
              </div>
              <div className="flex items-end">
                <label className="flex items-center gap-2 text-sm">
                  <input type="checkbox" checked={imapConfig.tls} onChange={() => setImapConfig(p => ({ ...p, tls: !p.tls }))} className="rounded" />
                  Use TLS
                </label>
              </div>
            </div>
            <div className="flex gap-2">
              <Button onClick={testImap} variant="outline" loading={testingImap}>Test Connection</Button>
              <Button onClick={saveImap} loading={savingImap}>Save IMAP Settings</Button>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="nextcloud" className="space-y-4 mt-4">
          <div className="rounded-lg border p-4 space-y-3">
            <h3 className="font-medium flex items-center gap-2"><Cloud className="h-4 w-4" /> Nextcloud / CardDAV</h3>
            <div className="grid grid-cols-2 gap-3">
              <div className="col-span-2">
                <label className="text-xs font-medium text-muted-foreground">Nextcloud URL</label>
                <Input value={ncConfig.nextcloudUrl} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNcConfig(p => ({ ...p, nextcloudUrl: e.target.value }))} placeholder="https://nextcloud.example.com" />
              </div>
              <div className="col-span-2">
                <label className="text-xs font-medium text-muted-foreground">CardDAV Path (optional)</label>
                <Input value={ncConfig.cardDavPath} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNcConfig(p => ({ ...p, cardDavPath: e.target.value }))} placeholder="/remote.php/dav/addressbooks/users/USER/contacts/" />
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">Username</label>
                <Input value={ncConfig.username} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNcConfig(p => ({ ...p, username: e.target.value }))} />
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">App Password</label>
                <Input type="password" value={ncConfig.password} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNcConfig(p => ({ ...p, password: e.target.value }))} />
              </div>
            </div>
            <Button onClick={syncContacts} loading={syncingContacts}>Sync Contacts Now</Button>
          </div>
        </TabsContent>

        <TabsContent value="oidc" className="space-y-4 mt-4">
          <div className="rounded-lg border p-4 space-y-3">
            <h3 className="font-medium flex items-center gap-2"><Shield className="h-4 w-4" /> OIDC Configuration</h3>
            <p className="text-sm text-muted-foreground">Authelia single sign-on, configured via environment variables (read-only)</p>
            <div className={`flex items-start gap-2 rounded-md p-3 text-sm ${oidc.enabled ? 'bg-green-500/10 text-green-700 dark:text-green-400' : 'bg-amber-500/10 text-amber-800 dark:text-amber-300'}`}>
              {oidc.enabled ? <Check className="h-4 w-4 mt-0.5 shrink-0" /> : <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />}
              <span>
                {oidc.enabled
                  ? <>Configured for <span className="font-mono break-all">{oidc.issuer}</span>. Client integration is still pending.</>
                  : <>Not connected. Authelia sign-in is disabled until all three variables below hold real values. Users can sign in with email and password meanwhile. Setup guide: <span className="font-mono">docs/AUTHELIA.md</span></>}
              </span>
            </div>
            <div className="space-y-2">
              {([['OIDC_ISSUER', oidc.issuerSet], ['OIDC_CLIENT_ID', oidc.clientIdSet], ['OIDC_CLIENT_SECRET', oidc.clientSecretSet]] as const).map(([name, set]) => (
                <div key={name} className="flex items-center gap-2">
                  <span className="text-xs font-mono bg-muted px-2 py-1 rounded">{name}</span>
                  <span className="text-sm text-muted-foreground">{set ? 'Set' : 'Not set'}</span>
                  {set ? <Check className="h-4 w-4 text-green-500" /> : <X className="h-4 w-4 text-muted-foreground" />}
                </div>
              ))}
            </div>
            <div className="space-y-1 pt-2 border-t">
              <p className="text-xs font-medium text-muted-foreground">Redirect URI to register in Authelia</p>
              <p className="text-xs font-mono bg-muted px-2 py-1.5 rounded break-all" suppressHydrationWarning>{callbackUrl}</p>
            </div>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}
