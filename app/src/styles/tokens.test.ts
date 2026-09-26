import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else if (p.endsWith('.module.css')) out.push(p)
  }
  return out
}

const COLOR = /#[0-9a-f]{3,8}\b|\brgba?\(|\bhsla?\(/i
const FONT = /font-family\s*:(?!\s*var\()/i
const RADIUS = /border-radius\s*:\s*[0-9.]+px/i

const LEGACY = new Set<string>([
])

describe('css modules use tokens', () => {
  const files = walk(join(__dirname, '..'))
  it('finds modules', () => expect(files.length).toBeGreaterThan(0))
  for (const f of files) {
    const rel = f.replace(join(__dirname, '..'), 'src')
    const relPath = rel.replace(/^src\//, '')
    const run = LEGACY.has(relPath) ? it.skip : it
    run(rel, () => {
      const css = readFileSync(f, 'utf8')
      expect(css).not.toMatch(COLOR)
      expect(css).not.toMatch(FONT)
      expect(css).not.toMatch(RADIUS)
    })
  }
})
