import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../../App'
import { signInAsAdmin } from '../../../test/admin'
import { emailFixtures } from '../../../test/fixtures'
import { renderWithProviders } from '../../../test/render'
import { server } from '../../../test/setup'

beforeEach(() => signInAsAdmin())

const bodyRows = () => within(screen.getAllByRole('rowgroup')[1]!).getAllByRole('row')
const subNav = () => screen.getByRole('navigation', { name: 'Secondary' })

/** Records the query string of every outbox/inbound GET, then falls through to the default handler. */
function recordSearches(path: string): string[] {
  const seen: string[] = []
  server.use(http.get(path, ({ request }) => { seen.push(new URL(request.url).search) }))
  return seen
}

describe('templates list', () => {
  test('shows both templates, the email sub-nav, and links each row to its form', async () => {
    renderWithProviders(<App />, { route: '/admin/email/templates' })
    expect(await screen.findByRole('link', { name: 'ticket_autoresp' })).toHaveAttribute('href', '/admin/email/templates/ticket_autoresp')
    expect(screen.getByRole('link', { name: 'ticket_reply' })).toHaveAttribute('href', '/admin/email/templates/ticket_reply')
    expect(screen.getByRole('heading', { name: /^Email Templates/ })).toHaveTextContent('Email Templates (2)')
    const rows = bodyRows()
    expect(rows).toHaveLength(2)
    expect(within(rows[0]!).getAllByRole('cell')[1]).toHaveTextContent('[#{{.Number}}] {{.Subject}}')
    const sub = subNav()
    expect(within(sub).getAllByRole('link').map((a) => [a.textContent, a.getAttribute('href')])).toEqual([
      ['Templates', '/admin/email/templates'], ['Outbox', '/admin/email/outbox'], ['Inbound Log', '/admin/email/inbound'],
    ])
    expect(within(sub).getByRole('link', { name: 'Templates' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Email' })).toHaveAttribute('aria-current', 'page')
  })

  test('shows the template count in the footer only once the list has loaded', async () => {
    let release!: () => void
    const gate = new Promise<void>((r) => { release = r })
    server.use(http.get('/api/v1/email/templates', async () => {
      await gate
      return HttpResponse.json({ items: emailFixtures.templates })
    }))
    renderWithProviders(<App />, { route: '/admin/email/templates' })
    expect(await screen.findByRole('heading', { name: 'Email Templates' })).toBeInTheDocument()
    expect(screen.queryByText(/\d+ templates/)).not.toBeInTheDocument()
    release()
    expect(await screen.findByText('2 templates')).toBeInTheDocument()
  })

  test('/admin/email redirects to the templates list', async () => {
    renderWithProviders(<App />, { route: '/admin/email' })
    expect(await screen.findByRole('link', { name: 'ticket_autoresp' })).toBeInTheDocument()
  })
})

describe('template form', () => {
  const route = '/admin/email/templates/ticket_autoresp'

  test('loads the template, lists the variables, and keeps the Templates sub-nav item active', async () => {
    renderWithProviders(<App />, { route })
    expect(await screen.findByRole('heading', { name: 'Email Template: ticket_autoresp' })).toBeInTheDocument()
    expect(screen.getByLabelText('Subject')).toHaveValue('[#{{.Number}}] {{.Subject}}')
    expect(screen.getByLabelText('HTML body')).toHaveValue('<p>Hi</p>')
    expect(screen.getByLabelText('Text body')).toHaveValue('Hi')
    const vars = screen.getByRole('list', { name: 'Template variables' })
    for (const v of ['{{.Number}}', '{{.Subject}}', '{{.Message}}', '{{.MessageHTML}}', '{{.RequesterName}}', '{{.RequesterEmail}}', '{{.AgentName}}', '{{.SiteName}}', '{{.Link}}']) {
      expect(within(vars).getByText(v)).toBeInTheDocument()
    }
    expect(within(subNav()).getByRole('link', { name: 'Templates' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/admin/email/templates')
  })

  test('save sends only the changed fields and flashes', async () => {
    let body: unknown
    let key: unknown
    server.use(http.patch('/api/v1/email/templates/:key', async ({ request, params }) => {
      key = params.key
      body = await request.json()
      return HttpResponse.json({ ...emailFixtures.templates[0], ...(body as object) })
    }))
    renderWithProviders(<App />, { route })
    const subject = await screen.findByLabelText('Subject')
    await userEvent.clear(subject)
    await userEvent.type(subject, 'Got it')
    await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(body).toEqual({ subject: 'Got it' }))
    expect(key).toBe('ticket_autoresp')
    expect(await screen.findByRole('status')).toHaveTextContent('Template saved')
  })

  test('reset restores the loaded values', async () => {
    renderWithProviders(<App />, { route })
    const text = await screen.findByLabelText('Text body')
    await userEvent.type(text, ' there')
    expect(text).toHaveValue('Hi there')
    await userEvent.click(screen.getByRole('button', { name: 'Reset' }))
    expect(text).toHaveValue('Hi')
  })

  test('a 422 on body_html renders under the HTML body', async () => {
    server.use(http.patch('/api/v1/email/templates/:key', () => HttpResponse.json(
      { error: { code: 'validation_failed', message: 'request validation failed', fields: { body_html: 'unclosed action' } } }, { status: 422 })))
    renderWithProviders(<App />, { route })
    await userEvent.type(await screen.findByLabelText('HTML body'), ' broken')
    await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(await screen.findByText('unclosed action')).toBeInTheDocument()
    expect(screen.getByLabelText('HTML body')).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByLabelText('Subject')).not.toHaveAttribute('aria-invalid')
  })

  test('an unknown key renders not found', async () => {
    renderWithProviders(<App />, { route: '/admin/email/templates/nope' })
    expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
  })
})

describe('outbox', () => {
  test('lists all statuses by default and filters by status through the URL', async () => {
    const seen = recordSearches('/api/v1/email/outbox')
    renderWithProviders(<App />, { route: '/admin/email/outbox' })
    expect(await screen.findByRole('heading', { name: /^Outbox/ })).toBeInTheDocument()
    await waitFor(() => expect(bodyRows()).toHaveLength(2))
    expect(screen.getByRole('heading', { name: /^Outbox/ })).toHaveTextContent('Outbox (2)')
    const status = screen.getByRole('combobox', { name: 'Status' })
    expect(status).toHaveValue('')
    expect(seen[0]).toBe('?page=1&page_size=25')
    await userEvent.selectOptions(status, 'failed')
    await waitFor(() => expect(bodyRows()).toHaveLength(1))
    expect(seen.at(-1)).toBe('?status=failed&page=1&page_size=25')
    expect(status).toHaveValue('failed')
    expect(within(subNav()).getByRole('link', { name: 'Outbox' })).toHaveAttribute('aria-current', 'page')
  })

  test('rows link the ticket, show status, and truncate the last error with the full text in title', async () => {
    renderWithProviders(<App />, { route: '/admin/email/outbox' })
    await waitFor(() => expect(bodyRows()).toHaveLength(2))
    const [failed, sent] = bodyRows()
    expect(within(failed!).getByRole('link', { name: '#7' })).toHaveAttribute('href', '/tickets/7')
    expect(within(failed!).getByText('R <r@x.test>')).toBeInTheDocument()
    expect(within(failed!).getByText('failed')).toBeInTheDocument()
    const full = emailFixtures.outbox[0]!.last_error!
    const err = within(failed!).getByTitle(full)
    expect(err.textContent).toHaveLength(60)
    expect(err.textContent).toBe(`${full.slice(0, 59)}…`)
    expect(within(sent!).getByText('sent')).toBeInTheDocument()
    expect(within(sent!).queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument()
  })

  test('retry on a failed row POSTs and flashes', async () => {
    const posted: string[] = []
    server.use(http.post('/api/v1/email/outbox/:id/retry', ({ params }) => {
      posted.push(String(params.id))
      return HttpResponse.json({ id: Number(params.id), status: 'pending' })
    }))
    renderWithProviders(<App />, { route: '/admin/email/outbox' })
    await waitFor(() => expect(bodyRows()).toHaveLength(2))
    await userEvent.click(within(bodyRows()[0]!).getByRole('button', { name: 'Retry' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Queued for retry')
    expect(posted).toEqual(['1'])
  })

  test('the page comes from the URL and pagination moves it', async () => {
    const many = Array.from({ length: 30 }, (_, i) => ({ ...emailFixtures.outbox[1]!, id: i + 1 }))
    const seen: string[] = []
    server.use(http.get('/api/v1/email/outbox', ({ request }) => {
      const url = new URL(request.url)
      seen.push(url.search)
      const page = Number(url.searchParams.get('page'))
      return HttpResponse.json({ items: many.slice((page - 1) * 25, page * 25), page, page_size: 25, total: many.length })
    }))
    renderWithProviders(<App />, { route: '/admin/email/outbox?page=2' })
    await waitFor(() => expect(bodyRows()).toHaveLength(5))
    expect(seen[0]).toBe('?page=2&page_size=25')
    await userEvent.click(screen.getByRole('button', { name: 'Page 1' }))
    await waitFor(() => expect(bodyRows()).toHaveLength(25))
    expect(seen.at(-1)).toBe('?page=1&page_size=25')
  })
})

describe('outbox message', () => {
  test('the subject links to the message page, which shows headers and the text body in a <pre>', async () => {
    renderWithProviders(<App />, { route: '/admin/email/outbox' })
    await waitFor(() => expect(bodyRows()).toHaveLength(2))
    const link = within(bodyRows()[0]!).getByRole('link', { name: '[#000007] Printer' })
    expect(link).toHaveAttribute('href', '/admin/email/outbox/1')
    await userEvent.click(link)
    expect(await screen.findByRole('heading', { name: 'Outbox Message #1' })).toBeInTheDocument()
    expect(screen.getByText('R <r@x.test>')).toBeInTheDocument()
    expect(screen.getByText('ticket_autoresp')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '#7' })).toHaveAttribute('href', '/tickets/7')
    const pre = document.querySelector('pre')!
    expect(pre).toHaveAccessibleName('Text body')
    expect(pre.textContent).toBe('Hello R,\n\nhttp://localhost:5173/portal/t/abc123')
    expect(screen.getByLabelText('HTML body')).toHaveTextContent('<p>Hello R,</p>')
    expect(within(subNav()).getByRole('link', { name: 'Outbox' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Back to Outbox' })).toHaveAttribute('href', '/admin/email/outbox')
  })

  test('a ticketless row (account mail) shows a dash for the ticket, in the list and on the page', async () => {
    const row = { ...emailFixtures.outbox[1]!, id: 9, ticket_id: null, entry_id: null, template_key: 'client_confirm', subject: 'Confirm your account' }
    server.use(
      http.get('/api/v1/email/outbox', () => HttpResponse.json({ items: [row], page: 1, page_size: 25, total: 1 })),
      http.get('/api/v1/email/outbox/9', () => HttpResponse.json({ ...row, body_text: 'confirm', body_html: '<p>confirm</p>' })),
    )
    renderWithProviders(<App />, { route: '/admin/email/outbox' })
    await screen.findByRole('link', { name: 'Confirm your account' })
    expect(within(bodyRows()[0]!).getAllByRole('cell')[1]).toHaveTextContent(/^—$/)
    expect(within(bodyRows()[0]!).queryByRole('link', { name: /^#/ })).not.toBeInTheDocument()
    await userEvent.click(within(bodyRows()[0]!).getByRole('link', { name: 'Confirm your account' }))
    expect(await screen.findByRole('heading', { name: 'Outbox Message #9' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /^#/ })).not.toBeInTheDocument()
  })

  test('an unknown message says so', async () => {
    renderWithProviders(<App />, { route: '/admin/email/outbox/999' })
    expect(await screen.findByText('Message not found.')).toBeInTheDocument()
  })
})

describe('inbound log', () => {
  test('rows show outcome badges, the reason, and a dash when no ticket', async () => {
    const seen = recordSearches('/api/v1/email/inbound')
    renderWithProviders(<App />, { route: '/admin/email/inbound' })
    expect(await screen.findByRole('heading', { name: /^Inbound Log/ })).toBeInTheDocument()
    await waitFor(() => expect(bodyRows()).toHaveLength(2))
    expect(seen[0]).toBe('?page=1&page_size=25')
    const [replied, ignored] = bodyRows()
    expect(within(replied!).getByText('replied')).toBeInTheDocument()
    expect(within(replied!).getByRole('link', { name: '#7' })).toHaveAttribute('href', '/tickets/7')
    expect(within(replied!).getByText('R <r@x.test>')).toBeInTheDocument()
    expect(within(ignored!).getByText('ignored')).toBeInTheDocument()
    expect(within(ignored!).getByText('auto-submitted')).toBeInTheDocument()
    expect(within(ignored!).queryByRole('link')).not.toBeInTheDocument()
    expect(within(ignored!).getByText('—')).toBeInTheDocument()
    expect(within(subNav()).getByRole('link', { name: 'Inbound Log' })).toHaveAttribute('aria-current', 'page')
  })
})
