import { sanitizeHtml } from './sanitize'

test('removes script tags', () => {
  const out = sanitizeHtml('<p>hi</p><script>window.__pwned=1</script>')
  expect(out).not.toContain('<script')
})

test('removes onerror attributes', () => {
  const out = sanitizeHtml('<img src=x onerror="window.__pwned=2">')
  expect(out).not.toContain('onerror')
})

test('removes style elements', () => {
  const out = sanitizeHtml('<style>body{display:none}</style><p>hi</p>')
  expect(out).not.toContain('<style')
})

test('removes style attributes', () => {
  const out = sanitizeHtml('<div style="position:fixed;inset:0">hi</div>')
  expect(out).not.toContain('style=')
})

test('removes form and input elements', () => {
  const out = sanitizeHtml('<form action="https://evil"><input type="password"></form>')
  expect(out).not.toContain('<form')
  expect(out).not.toContain('<input')
})

test('strips javascript: hrefs', () => {
  const div = document.createElement('div')
  div.innerHTML = sanitizeHtml('<a href="javascript:alert(1)">click</a>')
  const a = div.querySelector('a')
  expect(a).not.toBeNull()
  expect(a?.getAttribute('href')).toBeNull()
})

test('adds target=_blank and safe rel to links', () => {
  const div = document.createElement('div')
  div.innerHTML = sanitizeHtml('<a href="https://example.test">click</a>')
  const a = div.querySelector('a')
  expect(a?.getAttribute('target')).toBe('_blank')
  expect(a?.getAttribute('rel')).toBe('noopener noreferrer')
})

test('keeps safe content: paragraphs and images', () => {
  const div = document.createElement('div')
  div.innerHTML = sanitizeHtml('<p>hi</p><img src="https://x/y.png">')
  expect(div.querySelector('p')).not.toBeNull()
  expect(div.querySelector('img')?.getAttribute('src')).toBe('https://x/y.png')
})
