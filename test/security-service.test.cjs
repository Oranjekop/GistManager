const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { app } = require('electron')

const testUserData = fs.mkdtempSync(path.join(os.tmpdir(), 'gist-manager-security-'))
app.setPath('userData', testUserData)

app.whenReady().then(async () => {
  const security = require('../electron/security-service.cjs')
  security.initialize()

  let state = await security.getState()
  assert.equal(state.locked, false)
  assert.equal(state.settings.hasPassword, false)

  state = await security.configure({
    lockEnabled: true,
    biometricEnabled: false,
    newPassword: 'test-password'
  })
  assert.equal(state.settings.lockEnabled, true)
  assert.equal(state.settings.hasPassword, true)

  security.lock()
  assert.equal(security.isLocked(), true)
  assert.equal(security.unlockWithPassword('wrong-password').success, false)
  assert.equal(security.unlockWithPassword('test-password').success, true)

  security.saveToken('github-test-token')
  assert.equal(security.loadToken(), 'github-test-token')
  security.clearToken()
  assert.equal(security.loadToken(), '')

  console.log('security-service tests passed')
}).catch((error) => {
  console.error(error)
  process.exitCode = 1
}).finally(() => {
  app.quit()
})

app.on('will-quit', () => {
  fs.rmSync(testUserData, { recursive: true, force: true })
})
