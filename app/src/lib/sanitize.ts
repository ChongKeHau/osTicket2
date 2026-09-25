import DOMPurify from 'dompurify'

const purify = DOMPurify as typeof DOMPurify & { __linkHookRegistered?: boolean }

if (!purify.__linkHookRegistered) {
  purify.addHook('afterSanitizeAttributes', (node) => {
    if (node.hasAttribute('href')) {
      node.setAttribute('target', '_blank')
      node.setAttribute('rel', 'noopener noreferrer')
    }
  })
  purify.__linkHookRegistered = true
}

export function sanitizeHtml(html: string): string {
  return DOMPurify.sanitize(html, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['style', 'form', 'input', 'button', 'textarea', 'select'],
    FORBID_ATTR: ['style'],
  })
}
