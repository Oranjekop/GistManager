const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const { execFile } = require('node:child_process')
const { promisify } = require('node:util')
const { app, safeStorage, systemPreferences } = require('electron')

const execFileAsync = promisify(execFile)
const defaultSettings = {
  version: 1,
  lockEnabled: false,
  biometricEnabled: false,
  passwordSalt: '',
  passwordHash: '',
  encryptedToken: ''
}

let unlocked = false

function settingsPath() {
  return path.join(app.getPath('userData'), 'security.json')
}

function readSettings() {
  try {
    return {
      ...defaultSettings,
      ...JSON.parse(fs.readFileSync(settingsPath(), 'utf8'))
    }
  } catch {
    return { ...defaultSettings }
  }
}

function writeSettings(settings) {
  const target = settingsPath()
  const temporary = `${target}.tmp`
  fs.mkdirSync(path.dirname(target), { recursive: true })
  fs.writeFileSync(temporary, JSON.stringify(settings, null, 2), {
    encoding: 'utf8',
    mode: 0o600
  })
  fs.renameSync(temporary, target)
}

function hashPassword(password, salt) {
  return crypto.scryptSync(password, salt, 64).toString('hex')
}

function verifyPassword(password, settings = readSettings()) {
  if (!settings.passwordHash || !settings.passwordSalt || !password) {
    return false
  }

  const actual = Buffer.from(hashPassword(password, settings.passwordSalt), 'hex')
  const expected = Buffer.from(settings.passwordHash, 'hex')
  return actual.length === expected.length && crypto.timingSafeEqual(actual, expected)
}

async function runWindowsHello(mode) {
  const script = app.isPackaged
    ? path.join(process.resourcesPath, 'app.asar.unpacked', 'electron', 'windows-hello.ps1')
    : path.join(__dirname, 'windows-hello.ps1')

  try {
    const { stdout } = await execFileAsync('powershell.exe', [
      '-NoLogo',
      '-NoProfile',
      '-NonInteractive',
      '-ExecutionPolicy',
      'Bypass',
      '-File',
      script,
      '-Mode',
      mode
    ], {
      windowsHide: true,
      timeout: 60_000
    })
    return JSON.parse(stdout.trim())
  } catch (error) {
    try {
      return JSON.parse(String(error.stdout || '').trim())
    } catch {
      return {
        success: false,
        available: false,
        result: 'Unavailable',
        error: error.message
      }
    }
  }
}

async function getBiometricAvailability() {
  if (process.platform === 'darwin') {
    const available = systemPreferences.canPromptTouchID()
    return {
      available,
      type: 'touch-id',
      label: 'Touch ID',
      reason: available ? '' : '此 Mac 未配置 Touch ID。'
    }
  }

  if (process.platform === 'win32') {
    const result = await runWindowsHello('check')
    return {
      available: Boolean(result.available),
      type: 'windows-hello',
      label: 'Windows Hello / PIN',
      reason: result.available ? '' : '此设备未配置 Windows Hello 或 PIN。'
    }
  }

  return {
    available: false,
    type: 'unsupported',
    label: '系统生物识别',
    reason: '当前系统暂不支持系统身份验证。'
  }
}

function publicState(settings = readSettings()) {
  const locked = settings.lockEnabled && !unlocked
  return {
    locked,
    settings: {
      lockEnabled: Boolean(settings.lockEnabled),
      biometricEnabled: Boolean(settings.biometricEnabled),
      hasPassword: Boolean(settings.passwordHash)
    }
  }
}

async function getState() {
  return {
    ...publicState(),
    biometric: await getBiometricAvailability()
  }
}

function initialize() {
  const settings = readSettings()
  unlocked = !settings.lockEnabled
}

function isLocked() {
  const settings = readSettings()
  return settings.lockEnabled && !unlocked
}

function lock() {
  const settings = readSettings()
  unlocked = !settings.lockEnabled
  return publicState(settings)
}

function unlockWithPassword(password) {
  const settings = readSettings()

  if (!settings.lockEnabled) {
    unlocked = true
    return { success: true, ...publicState(settings) }
  }

  const success = verifyPassword(password, settings)
  unlocked = success
  return {
    success,
    message: success ? '' : '解锁密码不正确。',
    ...publicState(settings)
  }
}

async function unlockWithBiometric() {
  const settings = readSettings()

  if (!settings.lockEnabled) {
    unlocked = true
    return { success: true, ...publicState(settings) }
  }

  if (!settings.biometricEnabled) {
    return { success: false, message: '系统身份验证未启用。', ...publicState(settings) }
  }

  const availability = await getBiometricAvailability()

  if (!availability.available) {
    return {
      success: false,
      fallback: true,
      message: `${availability.reason} 请使用软件解锁密码。`,
      ...publicState(settings)
    }
  }

  try {
    if (process.platform === 'darwin') {
      await systemPreferences.promptTouchID('解锁 Gist管理器')
      unlocked = true
    } else if (process.platform === 'win32') {
      const result = await runWindowsHello('verify')
      unlocked = Boolean(result.success)
    }
  } catch {
    unlocked = false
  }

  return {
    success: unlocked,
    fallback: !unlocked,
    message: unlocked ? '' : '系统身份验证未通过，请使用软件解锁密码。',
    ...publicState(settings)
  }
}

async function configure(options = {}) {
  const settings = readSettings()
  const hasPassword = Boolean(settings.passwordHash)
  const currentPassword = String(options.currentPassword || '')
  const newPassword = String(options.newPassword || '')
  const lockEnabled = Boolean(options.lockEnabled)
  const biometricEnabled = Boolean(options.biometricEnabled)

  if (hasPassword && !verifyPassword(currentPassword, settings)) {
    throw new Error('当前解锁密码不正确。')
  }

  if (newPassword && newPassword.length < 6) {
    throw new Error('新的解锁密码至少需要 6 位。')
  }

  if (lockEnabled && !hasPassword && !newPassword) {
    throw new Error('开启启动锁定前，请先设置至少 6 位的软件解锁密码。')
  }

  if (biometricEnabled) {
    const availability = await getBiometricAvailability()
    if (!availability.available) {
      throw new Error(`${availability.reason} 请先在系统中完成配置。`)
    }
  }

  if (newPassword) {
    settings.passwordSalt = crypto.randomBytes(16).toString('hex')
    settings.passwordHash = hashPassword(newPassword, settings.passwordSalt)
  }

  settings.lockEnabled = lockEnabled
  settings.biometricEnabled = lockEnabled && biometricEnabled
  writeSettings(settings)
  unlocked = true
  return getState()
}

function saveToken(token) {
  const settings = readSettings()
  const value = String(token || '').trim()

  if (!value) {
    settings.encryptedToken = ''
  } else {
    if (!safeStorage.isEncryptionAvailable()) {
      throw new Error('系统安全存储当前不可用，无法保存 GitHub Token。')
    }
    settings.encryptedToken = safeStorage.encryptString(value).toString('base64')
  }

  writeSettings(settings)
  return true
}

function loadToken() {
  if (isLocked()) {
    throw new Error('应用尚未解锁。')
  }

  const settings = readSettings()
  if (!settings.encryptedToken) {
    return ''
  }

  try {
    return safeStorage.decryptString(Buffer.from(settings.encryptedToken, 'base64'))
  } catch {
    return ''
  }
}

function clearToken() {
  return saveToken('')
}

module.exports = {
  clearToken,
  configure,
  getState,
  initialize,
  isLocked,
  loadToken,
  lock,
  saveToken,
  unlockWithBiometric,
  unlockWithPassword
}
