const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { app } = require('electron')

const testUserData = fs.mkdtempSync(path.join(os.tmpdir(), 'gist-manager-updater-'))
app.setPath('userData', testUserData)

app.whenReady().then(() => {
  const updates = require('../electron/update-service.cjs')
  updates.initialize()

  assert.equal(updates.getState().currentVersion, app.getVersion())
  assert.equal(updates.setChannel('beta').channel, 'beta')
  assert.equal(updates.setChannel('stable').channel, 'stable')
  assert.throws(() => updates.setChannel('unknown'), /未知的更新通道/)

  const saved = JSON.parse(fs.readFileSync(path.join(testUserData, 'updater.json'), 'utf8'))
  assert.equal(saved.channel, 'stable')
  console.log('update-service tests passed')
}).catch((error) => {
  console.error(error)
  process.exitCode = 1
}).finally(() => {
  app.quit()
})

app.on('will-quit', () => {
  fs.rmSync(testUserData, { recursive: true, force: true })
})
