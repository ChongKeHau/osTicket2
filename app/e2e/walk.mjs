import { chromium } from 'playwright'
import { mkdirSync } from 'node:fs'

const BASE = process.env.APP_URL ?? 'http://localhost:5173'
const USER = process.env.E2E_USER ?? 'admin'
const PASS = process.env.E2E_PASS ?? 'admin1234'
mkdirSync('shots', { recursive: true })
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 900 } })
const shot = (name) => page.screenshot({ path: `shots/${name}.png`, fullPage: true })

await page.goto(`${BASE}/login`)
await page.getByLabel('Username').fill(USER)
await page.getByLabel('Password').fill(PASS)
await page.getByRole('button', { name: 'Sign In' }).click()
await page.waitForURL(/\/tickets/)
await page.getByRole('row').nth(1).getByRole('link').first().waitFor()
await shot('01-queue')

await page.getByRole('link', { name: 'Dashboard' }).click()
await page.getByRole('heading', { name: 'Ticket Activity' }).waitFor()
await page.getByRole('img', { name: /Line chart/ }).waitFor()
await shot('02-dashboard')

await page.getByRole('link', { name: 'Tickets' }).click()
await page.getByRole('link', { name: 'New Ticket' }).click()
await page.getByLabel(/Email/).fill('walk@example.test')
await page.getByLabel(/Department/).selectOption({ index: 1 })
await page.getByLabel(/Subject/).fill('E2E walk ticket')
await page.getByLabel(/Message/).fill('Created by the e2e walk')
await page.getByRole('button', { name: 'Open Ticket' }).click()
await page.waitForURL(/\/tickets\/\d+/)
await shot('03-ticket')

// "Post Note" appears twice (sticky bar and composer submit), hence .last().
await page.getByRole('heading', { name: /Ticket #/ }).waitFor()
await page.getByRole('button', { name: 'Post Note' }).click()
await page.getByRole('tab', { name: 'Internal Note', selected: true }).waitFor()
await page.getByLabel('Note', { exact: true }).fill('internal note from the walk')
await page.getByRole('button', { name: 'Post Note' }).last().click()
await page.getByText('Note posted').waitFor()
await shot('04-ticket-note')

await page.getByRole('link', { name: 'Admin Panel' }).click()
await page.waitForURL(/\/admin\/departments/)
await page.getByRole('row').nth(1).getByRole('link').first().click()
await page.getByRole('button', { name: 'Save Changes' }).waitFor()
await shot('05-admin-form')

await page.getByRole('link', { name: 'Email' }).click()
await page.getByRole('heading', { name: /Email Templates/ }).waitFor()
// Row 1 is the "Loading…" placeholder until the list arrives; wait for a real template link.
await page.getByRole('row').nth(1).getByRole('link').first().waitFor()
await shot('06-email-templates')

// Portal leg: a customer in a fresh context (no staff session), anonymous until the access link.
const ctx2 = await browser.newContext({ viewport: { width: 1280, height: 900 } })
const portal = await ctx2.newPage()
const shotP = (name) => portal.screenshot({ path: `shots/${name}.png`, fullPage: true })
await portal.goto(`${BASE}/portal`)
await shotP('07-portal-landing')
// The tab bar also has an "Open a New Ticket" link; either goes to /portal/open.
await portal.getByRole('link', { name: 'Open a New Ticket' }).first().click()
await portal.getByLabel(/^Name/).fill('Walk Customer')
await portal.getByLabel(/^Email/).fill('walk-customer@example.test')
// Without a help topic the portal needs a department.
await portal.getByLabel(/^Department/).selectOption({ index: 1 })
await portal.getByLabel(/^Subject/).fill('Portal walk ticket')
await portal.getByLabel(/^Message/).fill('Opened from the portal walk')
await portal.getByRole('button', { name: 'Open Ticket' }).click()
await portal.getByText(/Ticket #\d+ opened/).waitFor()
await shotP('08-portal-opened')
const number = (await portal.getByText(/Ticket #\d+ opened/).textContent()).match(/#(\d+)/)[1]

// Request a guest access link, then read the emailed link from the outbox through the admin
// pages (the staff page is still signed in on the first context). The mail is queued in the
// background after the 202, so reload the outbox until this ticket's access mail is listed.
await portal.goto(`${BASE}/portal/login`)
await portal.getByLabel(/Ticket Number/).fill(number)
await portal.getByLabel(/^Email/).last().fill('walk-customer@example.test')
await portal.getByRole('button', { name: /access link/i }).click()
await portal.getByText(/sent an access link/i).waitFor()
const accessRow = page.getByRole('link', { name: `[#${number}] Access link for Portal walk ticket` })
for (let i = 0; ; i++) {
  await page.goto(`${BASE}/admin/email/outbox`)
  await page.getByRole('heading', { name: /^Outbox/ }).waitFor()
  await page.getByRole('row').nth(1).getByRole('link').first().waitFor()
  if (await accessRow.count()) break
  if (i === 10) throw new Error(`no access-link mail for ticket #${number} in the outbox`)
  await page.waitForTimeout(1000)
}
await accessRow.first().click()
const body = await page.locator('pre').first().textContent()
const link = body.match(/https?:\S+\/portal\/t\/[0-9a-f]+/)[0]
await portal.goto(link)
await portal.getByRole('heading', { name: /Portal walk ticket/ }).waitFor()
await shotP('09-portal-ticket')
await portal.getByLabel('Reply', { exact: true }).fill('Customer reply from the walk')
await portal.getByRole('button', { name: 'Post Reply' }).click()
await portal.getByText('Reply posted').waitFor()
await portal.getByRole('button', { name: 'Close ticket' }).click()
await portal.getByRole('button', { name: 'Confirm' }).click()
await portal.getByText('Ticket closed').waitFor()
await portal.getByRole('button', { name: 'Reopen' }).waitFor()
await shotP('10-portal-closed')
await ctx2.close()

await browser.close()
console.log('walk complete; screenshots in app/e2e/shots')
