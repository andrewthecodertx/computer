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
  isPublic: false, imapWatchEnabled: false, alertAt: null, dueDate: null, createdAt: '2026-10-07T00:00:00Z', updatedAt: '2026-10-07T00:00:00Z',
}))
let failSaves = false
let delaySaves = 0
let activeSaves = 0
let maxConcurrentSaves = 0
let paginatedRequests = 0
const api = http.createServer(async (req, res) => {
  try {
    let raw = ''
    for await (const chunk of req) raw += chunk
    const body = raw ? JSON.parse(raw) : {}
    const url = new URL(req.url, 'http://fixture')
    let result = []
    if (url.pathname === '/api/auth/login') result = { user: { id: 'fixture-user', name: 'Fixture', email: body.email, role: 'USER' }, tokenVersion: 1 }
    else if (url.pathname === '/api/me') result = { user: { id: 'fixture-user' }, effectiveUser: { id: 'fixture-user' }, isAdmin: false, viewingAs: null }
    // The list endpoint is a metadata projection: content bodies stay out (PERF-01).
    else if (url.pathname === '/api/pages') result = [...pages.values()].map(({ content, ...summary }) => summary)
    else if (url.pathname.startsWith('/api/pages/')) {
      const id = url.pathname.split('/')[3]
      if (req.method === 'PATCH') {
        if (failSaves) { res.writeHead(500, { 'Content-Type': 'application/json' }); res.end('{"error":"Fixture save failed"}'); return }
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
      const matching = url.searchParams.get('shared') === 'true' ? [] : bookmarks.filter(b => b.title.includes(url.searchParams.get('search') || ''))
      const offset = Number(url.searchParams.get('cursor') || 0)
      const limit = Number(url.searchParams.get('limit') || 200)
      result = matching.slice(offset, offset + limit)
      if (offset + limit < matching.length) res.setHeader('X-Next-Cursor', String(offset + limit))
      if (offset) paginatedRequests++
    } else if (url.pathname === '/api/tags') result = tags
    else if (url.pathname === '/api/kanban/columns') result = [{ id: 'inbox', label: 'Inbox', color: '#123456', key: 'INBOX', position: 0 }]
    else if (/^\/api\/bookmarks\/[^/]+$/.test(url.pathname)) {
      const bookmark = bookmarks.find(b => b.id === url.pathname.split('/')[3])
      if (req.method === 'PUT') {
        const { tagIds, ...data } = body
        Object.assign(bookmark, data)
        if (tagIds) bookmark.tags = tagIds.map(id => ({ tag: tags.find(tag => tag.id === id) }))
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

  const editor = page.getByPlaceholder('Write Markdown notes… paste links, lists, thoughts.')
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

  await page.goto(`${base}/bookmarks`)
  await eventually(() => page.locator('tbody tr').count().then(n => n === 205), 'Collection truncated at 200')
  assert.ok(paginatedRequests > 0, 'Cursor not forwarded through proxy')
  await page.getByText('Fixture 000', { exact: true }).click()
  const sheet = page.getByRole('dialog')
  await sheet.getByLabel('URL', { exact: true }).fill('https://127.0.0.1/changed')
  await sheet.getByLabel('Description', { exact: true }).fill('Edited description')
  await sheet.getByLabel('First tag', { exact: true }).uncheck()
  await sheet.getByLabel('Second tag', { exact: true }).check()
  await sheet.locator('input[type="datetime-local"]').fill('2026-10-08T10:30')
  await sheet.getByRole('button', { name: 'Save Changes', exact: true }).click()
  await eventually(() => bookmarks[0].url === 'https://127.0.0.1/changed', 'Bookmark update missing')
  assert.equal(bookmarks[0].description, 'Edited description')
  assert.deepEqual(bookmarks[0].tags.map(({ tag }) => tag.id), ['two'])
  assert.equal(bookmarks[0].alertAt, '2026-10-08T15:30:00.000Z')
  assert.deepEqual(errors, [], 'Browser runtime errors')
  console.log('PASS: 205 bookmarks displayed; URL/description/tags editable; local alert time converted to UTC')
} finally {
  await browser?.close()
  web.kill('SIGTERM')
  api.closeAllConnections(); api.close()
  attacker.closeAllConnections(); attacker.close()
}
