import assert from 'node:assert/strict'
import http from 'node:http'
import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright'

// The real built Next server/UI with isolated API fixtures: no app users or DB writes.
const pages = new Map(['a', 'b'].map((id, position) => [id, { id, title: `Page ${id.toUpperCase()}`, content: '', version: 1, position, pinned: false, bookmarks: [] }]))
const tags = [{ id: 'one', name: 'First tag', color: '#123456' }, { id: 'two', name: 'Second tag', color: '#654321' }]
const bookmarks = Array.from({ length: 205 }, (_, i) => ({
  id: `bookmark-${i}`, ownerId: 'fixture-user', title: `Fixture ${String(i).padStart(3, '0')}`, url: 'https://127.0.0.1/test',
  description: '', notes: '', tags: [{ tag: tags[0] }], sharedWith: [], kanbanStatus: 'INBOX',
  isPublic: false, imapWatchEnabled: false, alertAt: null, dueDate: null, version: 0, createdAt: '2026-10-07T00:00:00Z', updatedAt: '2026-10-07T00:00:00Z',
}))
let failSaves = false
// Review CR-02: forces Go's signout endpoint to fail so the browser test can
// prove a revocation failure is reported accurately and local sign-out still
// completes.
let failSignout = false
// Fails only saves whose body.content matches (deterministic mid-loop
// failure for the DATA-07 envelope test — no racing the serial queue).
let failContent = null
// Records Authorization headers of POST /api/auth/signout so the browser
// sign-out test can prove the Next server reached Go's revocation endpoint.
const signouts = []
let delaySaves = 0
let activeSaves = 0
let maxConcurrentSaves = 0
let paginatedRequests = 0
// Counts every PATCH /api/pages/* attempt (including 409/500 rejections) so
// tests can prove a paused conflict issues no further saves (review TEST-01).
let patchAttempts = 0
const api = http.createServer(async (req, res) => {
  try {
    let raw = ''
    for await (const chunk of req) raw += chunk
    const body = raw ? JSON.parse(raw) : {}
    const url = new URL(req.url, 'http://fixture')
    let result = []
    if (url.pathname === '/api/auth/login') result = { user: { id: 'fixture-user', name: 'Fixture', email: body.email, role: 'USER' }, tokenVersion: 1 }
    else if (url.pathname === '/api/auth/signout' && req.method === 'POST') {
      signouts.push(req.headers.authorization || '')
      if (failSignout) { res.writeHead(500, { 'Content-Type': 'application/json' }); res.end('{"error":"Fixture revocation failed"}'); return }
      result = { ok: true }
    }
    else if (url.pathname === '/api/me') result = { user: { id: 'fixture-user' }, effectiveUser: { id: 'fixture-user' }, isAdmin: false, viewingAs: null }
    // The list endpoint is a metadata projection: content bodies stay out (PERF-01).
    else if (url.pathname === '/api/pages') result = [...pages.values()].map(({ content, ...summary }) => summary)
    else if (url.pathname.startsWith('/api/pages/')) {
      const id = url.pathname.split('/')[3]
      if (req.method === 'PATCH') {
        patchAttempts++
        if (failSaves) { res.writeHead(500, { 'Content-Type': 'application/json' }); res.end('{"error":"Fixture save failed"}'); return }
        if (failContent !== null && body.content === failContent) { res.writeHead(500, { 'Content-Type': 'application/json' }); res.end('{"error":"Fixture save failed"}'); return }
        const target = pages.get(id)
        // Optimistic concurrency (DATA-01): a stale base version is a 409.
        if (body.version !== undefined && body.version !== target.version) {
          res.writeHead(409, { 'Content-Type': 'application/json' })
          res.end('{"error":"Page was modified elsewhere; reload to see the latest version"}')
          return
        }
        activeSaves++
        maxConcurrentSaves = Math.max(maxConcurrentSaves, activeSaves)
        await new Promise(resolve => setTimeout(resolve, delaySaves))
        const { version, ...data } = body
        Object.assign(target, data)
        if ('content' in body) target.version++
        activeSaves--
      }
      result = pages.get(id)
    } else if (url.pathname === '/api/bookmarks') {
      const shared = url.searchParams.get('shared') === 'true'
      const matching = shared ? [] : bookmarks.filter(b => b.title.includes(url.searchParams.get('search') || ''))
      const offset = Number(url.searchParams.get('cursor') || 0)
      const limit = Number(url.searchParams.get('limit') || 200)
      // Mirror the Go permission-specific projections (PRIV-02): shared
      // readers get the narrowed allowlist shape, owners the full row.
      const project = b => shared ? (({ contact, contactId, sharedWith, alertAt, alertSent, imapQuery, imapWatchEnabled, kanbanColumnId, ...rest }) => rest)(b) : b
      result = matching.slice(offset, offset + limit).map(project)
      if (offset + limit < matching.length) res.setHeader('X-Next-Cursor', String(offset + limit))
      if (offset) paginatedRequests++
    } else if (url.pathname === '/api/tags') result = tags
    else if (url.pathname === '/api/kanban/columns') result = [{ id: 'inbox', label: 'Inbox', color: '#123456', key: 'INBOX', position: 0 }]
    else if (/^\/api\/bookmarks\/[^/]+$/.test(url.pathname)) {
      const bookmark = bookmarks.find(b => b.id === url.pathname.split('/')[3])
      if (req.method === 'PUT') {
        // Review CR-04: mirror Go's bookmark concurrency contract — version
        // is required (400), a stale base is 409 with the row untouched,
        // success bumps the revision.
        if (body.version === undefined) {
          res.writeHead(400, { 'Content-Type': 'application/json' })
          res.end('{"error":"version is required when updating a bookmark; refetch it and retry"}')
          return
        }
        if (body.version !== bookmark.version) {
          res.writeHead(409, { 'Content-Type': 'application/json' })
          res.end('{"error":"Bookmark was modified elsewhere; reload to see the latest version"}')
          return
        }
        const { tagIds, version, ...data } = body
        Object.assign(bookmark, data)
        if (tagIds) bookmark.tags = tagIds.map(id => ({ tag: tags.find(tag => tag.id === id) }))
        bookmark.version++
      }
      result = bookmark
    } else if (url.pathname.endsWith('/signals')) result = { sources: [], signals: [], types: [] }
    res.setHeader('Content-Type', 'application/json')
    res.end(JSON.stringify(result))
  } catch (error) { res.writeHead(500); res.end(String(error)) }
})
await new Promise(resolve => api.listen(0, '127.0.0.1', resolve))
// The free-port probe is TOCTOU-racy: another process can grab the port
// between close and `next start`. Retry up to three ports before giving up.
let base = ''
let web = null
let logs = ''
for (let attempt = 1; attempt <= 3; attempt++) {
  const portProbe = http.createServer()
  await new Promise(resolve => portProbe.listen(0, '127.0.0.1', resolve))
  const port = portProbe.address().port
  await new Promise(resolve => portProbe.close(resolve))
  base = `http://localhost:${port}`
  logs = ''
  web = spawn(process.execPath, ['node_modules/next/dist/bin/next', 'start', '--port', String(port)], {
    cwd: fileURLToPath(new URL('../web/', import.meta.url)),
    env: { ...process.env, AUTH_SECRET: 'isolated-browser-fixture-secret', NEXTAUTH_SECRET: 'isolated-browser-fixture-secret', NEXTAUTH_URL: base, API_BASE_URL: `http://127.0.0.1:${api.address().port}`, OIDC_ISSUER: '', OIDC_CLIENT_ID: '', OIDC_CLIENT_SECRET: '' },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  web.stdout.on('data', data => logs += data)
  web.stderr.on('data', data => logs += data)
  let started = false
  for (let i = 0; i < 150; i++) {
    try { if ((await fetch(`${base}/login`)).ok) { started = true; break } } catch { /* not up yet */ }
    if (/EADDRINUSE/.test(logs) || web.exitCode !== null) break
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  if (started) break
  web.kill('SIGKILL')
  if (attempt === 3) assert.fail(`Next server did not start after 3 ports: ${logs}`)
}
const attacker = http.createServer((req, res) => {
  res.setHeader('Content-Type', 'text/html')
  res.end(`<form method="POST" action="${base}/api/auth/login" enctype="text/plain"><input name='{"email":"attacker@computer.test","password":"password","extra":"' value='"}'></form><script>document.forms[0].submit()</script>`)
})
await new Promise(resolve => attacker.listen(0, '127.0.0.1', resolve))
async function eventually(check, label) {
  for (let i = 0; i < 100; i++) {
    if (await check()) return
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  assert.fail(label)
}
let browser
try {
  await eventually(async () => { try { return (await fetch(`${base}/login`)).ok } catch { return false } }, `Next server did not start: ${logs}`)
  browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || '/usr/bin/chromium', headless: true, args: ['--no-sandbox'] })
  const context = await browser.newContext({ timezoneId: 'America/Chicago' })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('dialog', dialog => dialog.accept())
  await page.goto(`http://127.0.0.1:${attacker.address().port}`)
  await page.waitForURL(`${base}/api/auth/login`)
  assert.equal((await context.request.get(`${base}/api/auth/session`).then(r => r.json()))?.user, undefined)
  for (const headers of [{ Origin: 'https://attacker.invalid' }, {}]) {
    const response = await context.request.post(`${base}/api/auth/login`, { headers, data: { email: 'attacker@computer.test', password: 'password' } })
    assert.equal(response.status(), 403)
  }
  const plain = await context.request.post(`${base}/api/auth/login`, { headers: { Origin: base, 'Content-Type': 'text/plain' }, data: '{"email":"attacker@computer.test"}' })
  assert.equal(plain.status(), 415)
  const login = await context.request.post(`${base}/api/auth/login`, { headers: { Origin: base }, data: { email: 'fixture@computer.test', password: 'password' } })
  assert.equal(login.status(), 200)
  console.log('PASS: forged logins rejected; legitimate login works')

  const editor = page.getByPlaceholder('Write Markdown notes… type / for blocks, paste links, lists, thoughts.')
  await page.goto(`${base}/pages?id=a`)
  await editor.fill('A edits before switching')
  await page.getByRole('button', { name: 'Page B', exact: true }).click()
  await eventually(() => page.getByRole('textbox', { name: 'Page title' }).inputValue().then(v => v === 'Page B'), 'Page B not selected')
  await editor.fill('B edits')
  await eventually(() => pages.get('b').content === 'B edits', 'Page B not saved')
  assert.equal(pages.get('a').content, 'A edits before switching')
  console.log('PASS: rapid page switching preserves both pages')

  delaySaves = 900
  maxConcurrentSaves = 0
  await editor.fill('First in-flight edit')
  await eventually(() => activeSaves === 1, 'Save did not start')
  await editor.fill('Newest edit wins')
  await eventually(() => pages.get('b').content === 'Newest edit wins' && activeSaves === 0, 'Newer edit did not save')
  assert.equal(maxConcurrentSaves, 1)
  delaySaves = 0
  console.log('PASS: saves are serialized and newer edits win')

  failSaves = true
  await editor.fill('Recover this failed draft')
  await page.getByRole('button', { name: 'Page A', exact: true }).click()
  await page.getByText('Could not save page. Your draft is kept in this tab; retry saving.').first().waitFor()
  assert.equal(await page.getByRole('textbox', { name: 'Page title' }).inputValue(), 'Page B')
  await page.reload()
  await eventually(() => editor.inputValue().then(value => value === 'Recover this failed draft'), 'Draft not recovered after reload')
  failSaves = false
  await page.getByRole('button', { name: 'Save now', exact: true }).click()
  await eventually(() => pages.get('b').content === 'Recover this failed draft', 'Retry did not persist draft')
  console.log('PASS: failed saves block switching, survive reload, and can be retried')

  // Cross-session conflict (review DATA-04/TEST-01): after another "device"
  // saves a newer version, the 409 pauses autosave in a distinct conflict
  // state. Typing must not schedule another save; only the explicit
  // overwrite button may replace the remote content.
  await page.goto(`${base}/pages?id=a`)
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await editor.fill('Local draft before conflict')
  await eventually(() => pages.get('a').content === 'Local draft before conflict', 'Draft did not save before the conflict')
  pages.get('a').content = 'Remote edit wins'
  pages.get('a').version++
  await editor.fill('Edit after remote change')
  await page.getByText('This page was changed elsewhere', { exact: true }).waitFor()
  const attemptsAtConflict = patchAttempts
  await editor.fill('Typing must not overwrite')
  await new Promise(resolve => setTimeout(resolve, 1200))
  assert.equal(patchAttempts, attemptsAtConflict, 'Typing during a conflict scheduled another save')
  assert.equal(pages.get('a').content, 'Remote edit wins', 'Conflict silently overwrote the remote content')
  await page.getByRole('button', { name: 'Overwrite with my draft', exact: true }).click()
  await eventually(() => pages.get('a').content === 'Typing must not overwrite', 'Explicit overwrite did not persist')
  console.log('PASS: conflicts pause autosave until an explicit overwrite')

  // Recovery after reload (review DATA-03/TEST-01): a failed draft remembers
  // the server version it was based on. When another session saved in the
  // meantime, recovery enters the conflict state instead of silently
  // rebasing the stale draft onto the newer version.
  failSaves = true
  await page.goto(`${base}/pages?id=b`)
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await editor.fill('Draft with an older base')
  await page.getByText('Could not save page. Your draft is kept in this tab; retry saving.').first().waitFor()
  pages.get('b').content = 'Remote saved while draft failed'
  pages.get('b').version++
  failSaves = false
  await page.reload()
  await eventually(() => editor.inputValue().then(value => value === 'Draft with an older base'), 'Draft not recovered after reload')
  await page.getByText('This page was changed elsewhere', { exact: true }).waitFor()
  // Snapshot AFTER recovery: the reload itself lets the departing page flush
  // its dirty draft once (the fixture rejects it with a 409 against the newer
  // version). What must not happen is any save from the recovered session.
  const attemptsAfterRecovery = patchAttempts
  await new Promise(resolve => setTimeout(resolve, 1000))
  assert.equal(patchAttempts, attemptsAfterRecovery, 'Recovered draft saved without an explicit resolution')
  assert.equal(pages.get('b').content, 'Remote saved while draft failed', 'Recovery rebased onto and clobbered the newer version')
  await page.getByRole('button', { name: 'Discard my draft and load latest', exact: true }).click()
  await eventually(() => editor.inputValue().then(value => value === 'Remote saved while draft failed'), 'Discard did not load the remote content')
  console.log('PASS: recovered drafts keep their original base and conflict instead of rebasing')

  // Unknown-provenance draft (review DATA-06): a legacy plain-string draft
  // carries no baseVersion, so recovery cannot prove it predates the remote
  // content. It must pause in the review-required conflict state instead of
  // saving over whatever the server holds.
  await page.goto(`${base}/pages?id=a`)
  await eventually(() => page.getByRole('textbox', { name: 'Page title' }).inputValue().then(v => v === 'Page A'), 'Page A did not open')
  await page.evaluate(() => sessionStorage.setItem('computer:page-draft:a', JSON.stringify('Legacy draft of unknown origin')))
  const attemptsBeforeLegacy = patchAttempts
  await page.reload()
  await eventually(() => editor.inputValue().then(value => value === 'Legacy draft of unknown origin'), 'Legacy draft not recovered')
  await page.getByText('This page was changed elsewhere', { exact: true }).waitFor()
  await new Promise(resolve => setTimeout(resolve, 1000))
  assert.equal(patchAttempts, attemptsBeforeLegacy, 'Unknown-base draft saved without review')
  assert.equal(pages.get('a').content, 'Typing must not overwrite', 'Unknown-base draft clobbered the remote content')
  await page.getByRole('button', { name: 'Discard my draft and load latest', exact: true }).click()
  await eventually(() => editor.inputValue().then(value => value === 'Typing must not overwrite'), 'Discard did not load the remote content')
  console.log('PASS: unknown-base legacy drafts pause for review instead of saving')

  // Envelope refresh (review DATA-07): when a later save in the same serial
  // loop fails, the stored recovery envelope must carry the newest content
  // AND the base advanced by the earlier success — otherwise reload shows a
  // spurious conflict for a draft that was already cleanly rebased.
  delaySaves = 900
  failContent = 'Envelope part two'
  await page.goto(`${base}/pages?id=b`)
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await editor.fill('Envelope part one')
  await eventually(() => activeSaves === 1, 'First envelope save did not start')
  await editor.fill('Envelope part two')
  await page.getByText('Could not save page. Your draft is kept in this tab; retry saving.').first().waitFor()
  assert.equal(pages.get('b').content, 'Envelope part one', 'First save of the loop did not land')
  await page.reload()
  await eventually(() => editor.inputValue().then(value => value === 'Envelope part two'), 'Newest loop content not recovered')
  assert.equal(await page.getByText('This page was changed elsewhere', { exact: true }).count(), 0,
    'Recovery hit a spurious conflict — the stored base was not refreshed after the successful save')
  delaySaves = 0
  failContent = null
  await page.getByRole('button', { name: 'Save now', exact: true }).click()
  await eventually(() => pages.get('b').content === 'Envelope part two', 'Retry did not persist the recovered draft')
  console.log('PASS: recovery envelope keeps the latest content and refreshed base')

  // Metadata mutations must not rebase a stale draft (review CR-17): a
  // rename/pin PATCH response carries the CURRENT content revision but never
  // acknowledges the draft's base. Adopting it would let a stale draft
  // overwrite newer remote content without any conflict.
  await page.goto(`${base}/pages?id=a`)
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await editor.fill('Metadata rebase draft')
  await eventually(() => pages.get('a').content === 'Metadata rebase draft', 'Draft did not save before the metadata mutation')
  pages.get('a').content = 'Remote content stays'
  pages.get('a').version++
  await page.locator('button[title="Pin to sidebar"]').click()
  await eventually(() => pages.get('a').pinned === true, 'Pin metadata patch did not land')
  await editor.fill('Stale draft after metadata patch')
  await page.getByText('This page was changed elsewhere', { exact: true }).waitFor()
  assert.equal(pages.get('a').content, 'Remote content stays', 'Metadata response rebased the stale draft over the remote content')
  await page.getByRole('button', { name: 'Discard my draft and load latest', exact: true }).click()
  await eventually(() => editor.inputValue().then(value => value === 'Remote content stays'), 'Discard did not load the remote content')
  console.log('PASS: metadata mutations do not rebase stale content drafts')

  await page.goto(`${base}/bookmarks`)
  await eventually(() => page.locator('tbody tr').count().then(n => n === 205), 'Collection truncated at 200')
  assert.ok(paginatedRequests > 0, 'Cursor not forwarded through proxy')
  await page.getByText('Fixture 000', { exact: true }).click()
  const sheet = page.getByRole('dialog')
  const fillSheet = async () => {
    await sheet.getByLabel('URL', { exact: true }).fill('https://127.0.0.1/changed')
    await sheet.getByLabel('Description', { exact: true }).fill('Edited description')
    await sheet.getByLabel('First tag', { exact: true }).uncheck()
    await sheet.getByLabel('Second tag', { exact: true }).check()
    await sheet.locator('input[type="datetime-local"]').fill('2026-10-08T10:30')
  }
  await fillSheet()
  // Review CR-04: a concurrent edit elsewhere bumps the revision while this
  // form is open. The save must surface a conflict, preserve the unsaved
  // draft, and leave the remote state untouched — no silent overwrite.
  bookmarks[0].version++
  bookmarks[0].notes = 'Remote edit wins'
  await sheet.getByRole('button', { name: 'Save Changes', exact: true }).click()
  await page.getByText('your edits were NOT saved', { exact: false }).first().waitFor()
  assert.equal(bookmarks[0].url, 'https://127.0.0.1/test', 'Stale bookmark save overwrote the remote state')
  assert.equal(bookmarks[0].notes, 'Remote edit wins', 'Stale bookmark save clobbered the remote notes')
  await page.getByRole('button', { name: 'Reload latest', exact: true }).click()
  await eventually(() => sheet.getByLabel('URL', { exact: true }).inputValue().then(v => v === 'https://127.0.0.1/test'), 'Sheet did not reload the latest version')
  await fillSheet()
  await sheet.getByRole('button', { name: 'Save Changes', exact: true }).click()
  await eventually(() => bookmarks[0].url === 'https://127.0.0.1/changed', 'Bookmark update missing after conflict resolution')
  assert.equal(bookmarks[0].description, 'Edited description')
  assert.deepEqual(bookmarks[0].tags.map(({ tag }) => tag.id), ['two'])
  assert.equal(bookmarks[0].alertAt, '2026-10-08T15:30:00.000Z')
  assert.equal(bookmarks[0].notes, 'Remote edit wins', 'Conflict resolution lost the remote notes')
  assert.deepEqual(errors, [], 'Browser runtime errors')
  console.log('PASS: 205 bookmarks displayed; stale saves conflict visibly; URL/description/tags editable; local alert time converted to UTC')

  // Revocation failure path (review CR-02): when Go's revocation fails, the
  // local sign-out still completes, but the UI must say so — local cookie
  // removal is not confirmed global revocation, and other sessions survive.
  await page.keyboard.press('Escape') // close the bookmark sheet so the top bar is reachable
  failSignout = true
  const failedRevoke = page.waitForResponse(r => r.url().includes('/api/session/revoke') && r.request().method() === 'POST')
  await page.getByRole('button', { name: 'Sign out', exact: true }).click()
  await page.getByText('global revocation failed', { exact: false }).first().waitFor()
  assert.equal((await (await failedRevoke).json()).revoked, false, 'Revoke route reported success despite the backend failure')
  await page.waitForURL(`${base}/login`)
  failSignout = false
  console.log('PASS: failed global revocation is reported and local sign-out still completes')

  // Browser sign-out must reach Go's revocation (review AUTH-LIFECYCLE): the
  // explicit /api/session/revoke call (with the NextAuth signOut event as
  // backstop) posts to Go /api/auth/signout with a signed assertion, so the
  // tokenVersion bump kills Go cookie sessions and any previously minted
  // assertions — not just the local NextAuth JWT.
  await context.request.post(`${base}/api/auth/login`, { headers: { Origin: base }, data: { email: 'fixture@computer.test', password: 'password' } })
  await page.goto(`${base}/pages`)
  const revoked = page.waitForResponse(r => r.url().includes('/api/session/revoke') && r.request().method() === 'POST')
  await page.getByRole('button', { name: 'Sign out', exact: true }).click()
  assert.equal((await (await revoked).json()).revoked, true, 'Revoke route did not confirm global revocation')
  await page.waitForURL(`${base}/login`)
  await eventually(() => signouts.length > 0, 'Go signout was not called during browser sign-out')
  const authHeader = signouts[signouts.length - 1]
  assert.ok(authHeader.startsWith('Bearer '), 'Signout call carried no bearer assertion')
  const claim = JSON.parse(Buffer.from(authHeader.slice('Bearer '.length).split('.')[1], 'base64url').toString())
  assert.equal(claim.uid, 'fixture-user')
  assert.equal(claim.typ, 'assertion')
  assert.equal(claim.ver, 1)
  assert.deepEqual(errors, [], 'Browser runtime errors during sign-out')
  console.log('PASS: browser sign-out revokes sessions through the Go backend')
} finally {
  await browser?.close()
  web.kill('SIGTERM')
  api.closeAllConnections(); api.close()
  attacker.closeAllConnections(); attacker.close()
}
