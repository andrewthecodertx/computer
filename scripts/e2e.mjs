import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { chromium } from 'playwright'

// Run with the Docker stack up. Only uniquely created QA accounts are cleaned
// up; existing users, bookmarks and database tables are never reset.
const base = process.env.BASE_URL || 'http://localhost:3000'
const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || '/usr/bin/chromium', headless: true, args: ['--no-sandbox'] })
const accounts = []
const errors = []
const password = 'qa-test-long-password'
const email = `qa-${Date.now()}@computer.test`
const context = await browser.newContext()
const page = await context.newPage()
page.on('pageerror', error => errors.push(error.message))

async function json(response, status = 200) {
  assert.equal(response.status(), status, `${response.url()}: ${await response.text()}`)
  return response.json()
}
try {
  const signup = await json(await context.request.post(`${base}/api/signup`, { data: { email, password, name: 'Browser QA' } }), 201)
  accounts.push(signup.userId)
  await page.goto(`${base}/login`)
  await page.getByPlaceholder('Email', { exact: true }).fill(email)
  await page.getByPlaceholder('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in with Email', exact: true }).click()
  await page.waitForURL('**/pages')
  await page.getByRole('heading', { name: 'No pages yet', exact: true }).waitFor()
  const me = await json(await context.request.get(`${base}/api/me`))
  assert.equal(me.user.id, signup.userId)
  await page.getByRole('button', { name: 'Add Bookmark', exact: true }).first().click()
  const dialog = page.getByRole('dialog')
  await dialog.getByPlaceholder('https://...').fill('https://example.org/browser-qa')
  await dialog.getByPlaceholder('Auto-filled from URL').fill('Browser QA bookmark')
  await dialog.getByRole('button', { name: 'Save Bookmark', exact: true }).click()
  await page.goto(`${base}/bookmarks`)
  await page.getByText('Browser QA bookmark', { exact: true }).first().waitFor()
  let bookmarks = await json(await context.request.get(`${base}/api/bookmarks`))
  const bookmark = bookmarks.find(b => b.title === 'Browser QA bookmark')
  assert.ok(bookmark?.id, 'UI-created bookmark was not persisted by Go')
  await page.getByText('Browser QA bookmark', { exact: true }).first().click()
  await page.getByRole('link', { name: 'Download Markdown', exact: true }).waitFor()
  const update = await json(await context.request.put(`${base}/api/bookmarks/${bookmark.id}`, { data: { notes: '# Browser verified', isPublic: true } }))
  assert.equal(update.notes, '# Browser verified')
  const publicContext = await browser.newContext()
  const publicPage = await publicContext.newPage()
  publicPage.on('pageerror', error => errors.push(error.message))
  await publicPage.goto(`${base}/share/bookmark/${bookmark.id}`)
  await publicPage.getByText('Browser QA bookmark', { exact: true }).first().waitFor()
  await publicContext.close()
  await page.goto(`${base}/kanban`)
  await page.getByRole('heading', { name: 'Kanban Board' }).waitFor()
  await page.getByText('Browser QA bookmark', { exact: true }).waitFor()
  const columns = await json(await context.request.get(`${base}/api/kanban/columns`))
  assert.equal(columns.length, 4)
  const custom = await json(await context.request.post(`${base}/api/kanban/columns`, { data: { label: 'QA done' } }), 201)
  await json(await context.request.put(`${base}/api/bookmarks/${bookmark.id}`, { data: { kanbanColumnId: custom.id } }))
  const moved = await json(await context.request.delete(`${base}/api/kanban/columns/${custom.id}`))
  assert.equal(moved.moved, 1)
  await page.goto(`${base}/pages`)
  await page.locator('main').getByRole('button', { name: 'New page', exact: true }).click()
  await page.getByRole('textbox', { name: 'Page title' }).waitFor()
  await page.getByPlaceholder('Write Markdown notes… paste links, lists, thoughts.').fill('# QA page\n\nAutosaved markdown.')
  await page.getByText('Saved', { exact: true }).waitFor()
  const pages = await json(await context.request.get(`${base}/api/pages`))
  const detail = await json(await context.request.get(`${base}/api/pages/${pages[0].id}`))
  assert.equal(detail.content, '# QA page\n\nAutosaved markdown.')
  await json(await context.request.patch(`${base}/api/pages/${pages[0].id}`, { data: { addBookmarkId: bookmark.id } }))
  const markdown = await context.request.get(`${base}/api/pages/${pages[0].id}/markdown`)
  assert.equal(markdown.status(), 200)
  assert.ok((await markdown.text()).includes('Browser QA bookmark'))
  for (const [path, heading] of [['bookmarks', 'All Bookmarks'], ['tags', 'Tags'], ['calendar', 'Calendar'], ['contacts', 'Contacts'], ['settings', 'Settings']]) {
    await page.goto(`${base}/${path}`)
    await page.getByRole('heading', { name: heading, exact: true }).first().waitFor()
  }
  assert.deepEqual(errors, [], 'Browser runtime errors')
  await json(await context.request.delete(`${base}/api/bookmarks/${bookmark.id}`))
  console.log('PASS: browser login, Go-backed bookmark create/edit/delete, public sharing, kanban, pages/autosave/export, all application screens')
} finally {
  await browser.close()
  if (process.env.E2E_KEEP_DATA !== '1' && accounts.length) {
    assert.ok(accounts.every(id => /^[a-z0-9]+$/i.test(id)))
    execFileSync('docker', ['compose', 'exec', '-T', 'db', 'psql', '-U', 'computer', '-d', 'computer', '-v', 'ON_ERROR_STOP=1', '-c', `DELETE FROM "User" WHERE id IN (${accounts.map(id => `'${id}'`).join(',')});`], { cwd: new URL('..', import.meta.url), stdio: 'inherit' })
  }
}
