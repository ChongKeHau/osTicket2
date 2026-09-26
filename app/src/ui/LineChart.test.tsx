import { render, screen } from '@testing-library/react'
import { LineChart } from './LineChart'

it('draws one path per series and thins x labels', () => {
  const labels = Array.from({ length: 30 }, (_, i) => `2026-09-${String(i + 1).padStart(2, '0')}`)
  const points = labels.map((_, i) => i % 7)
  render(<LineChart labels={labels} series={[{ label: 'Opened', points, color: 'var(--accent)' }, { label: 'Closed', points: points.map((p) => p * 2), color: 'var(--green)' }]} />)
  const svg = screen.getByRole('img', { name: /Opened, Closed/ })
  expect(svg.querySelectorAll('path[data-series]')).toHaveLength(2)
  const xLabels = svg.querySelectorAll('text[data-axis="x"]')
  expect(xLabels.length).toBeLessThanOrEqual(8)
  expect(xLabels[0]).toHaveTextContent('Sep 1')
  expect(svg.querySelectorAll('circle[data-point]')).toHaveLength(60)
  expect(svg.querySelector('circle[data-point]')?.querySelector('title')).toHaveTextContent('Opened, Sep 1: 0')
  expect(screen.getByText('Opened', { selector: 'li' })).toBeInTheDocument()
})

it('renders an all-zero series without NaN', () => {
  const { container } = render(<LineChart labels={['2026-09-01', '2026-09-02']} series={[{ label: 'A', points: [0, 0], color: 'red' }]} />)
  expect(container.innerHTML).not.toContain('NaN')
  expect(container.querySelectorAll('text[data-axis="y"]').length).toBeGreaterThan(0)
})
