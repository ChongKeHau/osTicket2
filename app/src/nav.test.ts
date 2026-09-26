import { queueTitle, ticketSubNav } from './nav'

const active = (search: string) => ticketSubNav(new URLSearchParams(search)).filter((i) => i.active).map((i) => i.label)

it('links each queue to its query string', () => {
  expect(ticketSubNav(new URLSearchParams()).map((i) => [i.label, i.to])).toEqual([
    ['Open', '/tickets?state=open'],
    ['My Tickets', '/tickets?state=open&assigned_to=me'],
    ['Unassigned', '/tickets?state=open&assigned_to=none'],
    ['Closed', '/tickets?state=closed'],
    ['All', '/tickets'],
  ])
})

it.each([
  ['?state=open', 'Open'],
  ['?state=open&assigned_to=me', 'My Tickets'],
  ['?state=open&assigned_to=none', 'Unassigned'],
  ['?state=closed', 'Closed'],
  ['', 'All'],
])('%s activates exactly %s', (search, label) => {
  expect(active(search)).toEqual([label])
})

it('ignores page, sort, q, dept_id and status when matching', () => {
  expect(active('?state=open&page=3&sort=priority&q=x&dept_id=2&status=1')).toEqual(['Open'])
  expect(active('?assigned_to=me&state=open&dept_id=2')).toEqual(['My Tickets'])
  expect(active('?state=closed&status=3')).toEqual(['Closed'])
  expect(active('?page=2&sort=-created_at&dept_id=1')).toEqual(['All'])
})

it('activates nothing for an unknown filter', () => {
  expect(active('?state=resolved')).toEqual([])
})

it.each([
  ['?state=open', 'Open Tickets'],
  ['?state=open&assigned_to=me', 'My Tickets'],
  ['?state=open&assigned_to=none', 'Unassigned Tickets'],
  ['?state=closed', 'Closed Tickets'],
  ['', 'All Tickets'],
  ['?state=open&dept_id=2', 'Open Tickets'],
  ['?state=open&q=printer', 'Search Results'],
  ['?state=resolved', 'Tickets'],
])('queueTitle(%s) is %s', (search, title) => {
  expect(queueTitle(new URLSearchParams(search))).toBe(title)
})
