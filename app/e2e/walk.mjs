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
await page.getByRole('row').nth(1).waitFor()
await shot('01-queue')

await page.getByRole('link', { name: 'Dashboard' }).click()
await page.getByRole('heading', { name: 'Ticket Activity' }).waitFor()
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
await page.getByRole('status').waitFor()
await shot('04-ticket-note')

await page.getByRole('link', { name: 'Admin Panel' }).click()
await page.waitForURL(/\/admin\/departments/)
await page.getByRole('row').nth(1).getByRole('link').first().click()
await page.getByRole('button', { name: 'Save Changes' }).waitFor()
await shot('05-admin-form')

await page.getByRole('link', { name: 'Email' }).click()
await page.getByRole('heading', { name: /Email Templates/ }).waitFor()
await shot('06-email-templates')

await browser.close()
console.log('walk complete; screenshots in app/e2e/shots')
