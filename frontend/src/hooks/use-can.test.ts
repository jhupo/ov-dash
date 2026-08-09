import { describe, expect, it } from 'vitest'
import { roleAllows } from './use-can'

describe('roleAllows', () => {
  it('allows admin without enumerated capabilities', () => {
    expect(
      roleAllows({ role: 'admin', capabilities: [] }, 'updates:apply')
    ).toBe(true)
  })

  it('uses database-provided capabilities for non-admin users', () => {
    const user = {
      role: 'operator',
      capabilities: ['servers:read' as const],
    }

    expect(roleAllows(user, 'servers:read')).toBe(true)
    expect(roleAllows(user, 'servers:ssh')).toBe(false)
  })

  it('denies access without a current user', () => {
    expect(roleAllows(null, 'dashboard:read')).toBe(false)
  })
})
