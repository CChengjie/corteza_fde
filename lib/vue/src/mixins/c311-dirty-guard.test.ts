import { expect } from 'chai'
import guard, { sanitizeC311Draft } from './c311-dirty-guard.js'

describe('C311 dirty guard', () => {
  const methods = (guard as any).methods

  afterEach(() => {
    delete (globalThis as any).window
  })

  it('keeps a form usable when browser storage is unavailable', () => {
    ;(globalThis as any).window = {
      sessionStorage: {
        setItem: () => { throw new Error('storage disabled') },
        removeItem: () => { throw new Error('storage disabled') },
      },
    }
    const context = { c311DirtyStorageKey: 'c311.form', c311DraftStorage: methods.c311DraftStorage }
    expect(() => methods.c311SaveDirtyDraft.call(context, { summary: 'keep' })).not.to.throw()
    expect(() => methods.c311ClearDirtyDraft.call(context)).not.to.throw()
  })

  it('never persists credential-shaped keys', () => {
    expect(sanitizeC311Draft({ summary: 'keep', password: 'drop', nested: { access_token: 'drop', value: 1 } })).to.deep.equal({
      summary: 'keep',
      nested: { value: 1 },
    })
  })
})
