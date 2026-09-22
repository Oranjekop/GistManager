<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import CodeEditor from './components/CodeEditor.vue'

const webApiBase = '/api'
const legacyToken = localStorage.getItem('gist-editor-token') || ''
const token = ref('')
const tokenDraft = ref('')
const profile = ref(null)
const gists = ref([])
const gist = ref(null)
const workspaceMode = ref('browse')
const selectedFile = ref('')
const editorContent = ref('')
const description = ref('')
const searchText = ref('')
const draftGistDescription = ref('')
const draftGistFilename = ref('notes.txt')
const draftGistContent = ref('')
const draftGistPublic = ref(false)
const draftFileName = ref('new-file.txt')
const draftFileContent = ref('')
const renameTarget = ref('')
const editMode = ref(false)
const loadingSession = ref(false)
const loadingDetail = ref(false)
const saving = ref(false)
const deleting = ref(false)
const statusMessage = ref('')
const errorMessage = ref('')
const securityReady = ref(false)
const securityState = ref({
  locked: false,
  settings: {
    lockEnabled: false,
    biometricEnabled: false,
    hasPassword: false
  },
  biometric: {
    available: false,
    label: '系统身份验证',
    reason: ''
  }
})
const unlockPassword = ref('')
const unlockError = ref('')
const unlockingBiometric = ref(false)
const settingsOpen = ref(false)
const settingsSaving = ref(false)
const settingsError = ref('')
const updateDialogOpen = ref(false)
const updateState = ref({
  status: 'idle',
  currentVersion: '',
  availableVersion: '',
  percent: 0,
  transferred: 0,
  total: 0,
  message: ''
})
let removeUpdateListener = null
const securityForm = ref({
  lockEnabled: false,
  biometricEnabled: false,
  currentPassword: '',
  newPassword: '',
  confirmPassword: ''
})
const confirmDialog = ref({
  open: false,
  title: '',
  message: '',
  actionText: '确认',
  onConfirm: null
})

const files = computed(() => Object.values(gist.value?.files || {}))
const selectedGistId = computed(() => gist.value?.id || '')
const currentFile = computed(() => gist.value?.files?.[selectedFile.value] || null)
const isConnected = computed(() => Boolean(profile.value && token.value))
const hasDesktopSecurity = computed(() => Boolean(window.gistDesktop?.security))
const hasDesktopUpdates = computed(() => Boolean(window.gistDesktop?.updates))
const updateBusy = computed(() => ['checking', 'downloading'].includes(updateState.value.status))
const needsTokenSetup = computed(() => (
  securityReady.value &&
  !securityState.value.locked &&
  !isConnected.value &&
  !loadingSession.value
))
const filteredGists = computed(() => {
  const keyword = searchText.value.trim().toLowerCase()

  if (!keyword) {
    return gists.value
  }

  return gists.value.filter((item) => {
    const haystacks = [
      item.description,
      item.id,
      ...(item.files || []).map((file) => file.filename)
    ]

    return haystacks.some((entry) => String(entry || '').toLowerCase().includes(keyword))
  })
})
const hasUnsavedContent = computed(() => currentFile.value?.content !== editorContent.value)
const hasUnsavedDescription = computed(() => (gist.value?.description || '') !== description.value)
const hasComposerChanges = computed(() => {
  if (workspaceMode.value === 'create-gist') {
    return Boolean(
      draftGistDescription.value.trim() ||
      draftGistFilename.value.trim() !== 'notes.txt' ||
      draftGistContent.value ||
      draftGistPublic.value
    )
  }

  if (workspaceMode.value === 'add-file') {
    return Boolean(
      draftFileName.value.trim() !== 'new-file.txt' ||
      draftFileContent.value
    )
  }

  return false
})
const canSave = computed(() => Boolean(
  selectedGistId.value &&
  selectedFile.value &&
  editMode.value &&
  !saving.value &&
  (hasUnsavedContent.value || hasUnsavedDescription.value)
))

function setStatus(message = '', error = '') {
  statusMessage.value = message
  errorMessage.value = error
}

function openConfirmDialog({ title, message, actionText = '确认', onConfirm }) {
  confirmDialog.value = {
    open: true,
    title,
    message,
    actionText,
    onConfirm
  }
}

function closeConfirmDialog() {
  confirmDialog.value = {
    open: false,
    title: '',
    message: '',
    actionText: '确认',
    onConfirm: null
  }
}

async function confirmDialogAction() {
  const action = confirmDialog.value.onConfirm
  closeConfirmDialog()

  if (action) {
    await action()
  }
}

async function persistToken() {
  if (window.gistDesktop?.security) {
    await window.gistDesktop.security.saveToken(token.value)
    localStorage.removeItem('gist-editor-token')
    return
  }

  localStorage.setItem('gist-editor-token', token.value)
}

async function clearPersistedToken() {
  if (window.gistDesktop?.security) {
    await window.gistDesktop.security.clearToken()
  }
  localStorage.removeItem('gist-editor-token')
}

function applySecurityState(state) {
  securityState.value = {
    ...securityState.value,
    ...state,
    settings: {
      ...securityState.value.settings,
      ...(state?.settings || {})
    },
    biometric: {
      ...securityState.value.biometric,
      ...(state?.biometric || {})
    }
  }
}

async function loadSavedToken() {
  if (window.gistDesktop?.security) {
    let savedToken = await window.gistDesktop.security.loadToken()

    if (!savedToken && legacyToken) {
      await window.gistDesktop.security.saveToken(legacyToken)
      savedToken = legacyToken
    }

    localStorage.removeItem('gist-editor-token')
    token.value = savedToken
    tokenDraft.value = savedToken
    return
  }

  token.value = legacyToken
  tokenDraft.value = legacyToken
}

async function resumeAfterUnlock() {
  await loadSavedToken()
  if (tokenDraft.value.trim() && !profile.value) {
    await connectWorkspace()
  }
}

async function initializeSecurity() {
  try {
    if (window.gistDesktop?.security) {
      const state = await window.gistDesktop.security.getState()
      applySecurityState(state)
      securityReady.value = true

      if (state.locked && state.settings.biometricEnabled && state.biometric.available) {
        await unlockWithBiometric()
      } else if (!state.locked) {
        await resumeAfterUnlock()
      }
      return
    }

    securityReady.value = true
    await resumeAfterUnlock()
  } catch (error) {
    securityReady.value = true
    unlockError.value = error.message || '读取安全设置失败。'
  }
}

async function unlockWithPassword() {
  unlockError.value = ''

  if (!unlockPassword.value) {
    unlockError.value = '请输入软件解锁密码。'
    return
  }

  try {
    const result = await window.gistDesktop.security.unlockWithPassword(unlockPassword.value)
    applySecurityState(result)

    if (!result.success) {
      unlockError.value = result.message || '解锁密码不正确。'
      return
    }

    unlockPassword.value = ''
    await resumeAfterUnlock()
  } catch (error) {
    unlockError.value = error.message || '解锁失败。'
  }
}

async function unlockWithBiometric() {
  if (!window.gistDesktop?.security || unlockingBiometric.value) {
    return
  }

  unlockError.value = ''
  unlockingBiometric.value = true

  try {
    const result = await window.gistDesktop.security.unlockWithBiometric()
    applySecurityState(result)

    if (!result.success) {
      unlockError.value = result.message || '系统身份验证未通过，请使用软件解锁密码。'
      return
    }

    await resumeAfterUnlock()
  } catch (error) {
    unlockError.value = error.message || '系统身份验证不可用，请使用软件解锁密码。'
  } finally {
    unlockingBiometric.value = false
  }
}

async function lockApplication() {
  if (!window.gistDesktop?.security) {
    return
  }

  const state = await window.gistDesktop.security.lock()
  applySecurityState(state)
  unlockPassword.value = ''
  unlockError.value = ''
}

function openSettings() {
  const current = securityState.value.settings
  securityForm.value = {
    lockEnabled: current.lockEnabled,
    biometricEnabled: current.biometricEnabled,
    currentPassword: '',
    newPassword: '',
    confirmPassword: ''
  }
  settingsError.value = ''
  settingsOpen.value = true
}

async function saveSecuritySettings() {
  const form = securityForm.value
  settingsError.value = ''

  if (form.newPassword !== form.confirmPassword) {
    settingsError.value = '两次输入的新密码不一致。'
    return
  }

  settingsSaving.value = true
  try {
    const state = await window.gistDesktop.security.configure({
      lockEnabled: form.lockEnabled,
      biometricEnabled: form.biometricEnabled,
      currentPassword: form.currentPassword,
      newPassword: form.newPassword
    })
    applySecurityState(state)
    settingsOpen.value = false
    setStatus('安全设置已保存。')
  } catch (error) {
    settingsError.value = error.message || '保存安全设置失败。'
  } finally {
    settingsSaving.value = false
  }
}

function applyUpdateState(state) {
  updateState.value = {
    ...updateState.value,
    ...(state || {})
  }

  if (['available', 'downloaded'].includes(updateState.value.status)) {
    updateDialogOpen.value = true
  }
}

async function initializeUpdates() {
  if (!window.gistDesktop?.updates) {
    return
  }

  applyUpdateState(await window.gistDesktop.updates.getState())
  removeUpdateListener = window.gistDesktop.updates.onStateChanged(applyUpdateState)
}

async function checkForUpdates() {
  updateDialogOpen.value = true
  try {
    applyUpdateState(await window.gistDesktop.updates.check())
  } catch (error) {
    applyUpdateState({ status: 'error', message: error.message || '检查更新失败。' })
  }
}

async function selectUpdateChannel(channel) {
  if (channel === updateState.value.channel || updateBusy.value) {
    return
  }

  try {
    applyUpdateState(await window.gistDesktop.updates.setChannel(channel))
    await checkForUpdates()
  } catch (error) {
    applyUpdateState({ status: 'error', message: error.message || '切换更新通道失败。' })
  }
}

async function downloadUpdate() {
  try {
    applyUpdateState({ status: 'downloading', percent: 0, message: '正在准备下载…' })
    await window.gistDesktop.updates.download()
  } catch (error) {
    applyUpdateState({ status: 'error', message: error.message || '下载更新失败。' })
  }
}

async function installUpdate() {
  try {
    await window.gistDesktop.updates.install()
  } catch (error) {
    applyUpdateState({ status: 'error', message: error.message || '启动更新安装失败。' })
  }
}

function formatBytes(value) {
  const bytes = Number(value || 0)
  if (!bytes) {
    return '0 MB'
  }
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

async function initializeApp() {
  await Promise.all([initializeSecurity(), initializeUpdates()])
}

function resetCreateDraft() {
  draftGistDescription.value = ''
  draftGistFilename.value = 'notes.txt'
  draftGistContent.value = ''
  draftGistPublic.value = false
}

function resetFileDraft() {
  draftFileName.value = 'new-file.txt'
  draftFileContent.value = ''
}

function openCreateGistComposer() {
  if (!confirmIfDirty('进入新建 Gist 模式')) {
    return
  }

  resetCreateDraft()
  workspaceMode.value = 'create-gist'
  setStatus('已进入新建 Gist 模式。')
}

function openAddFileComposer() {
  if (!selectedGistId.value) {
    setStatus('', '请先选择一个 gist。')
    return
  }

  if (!confirmIfDirty('进入新增文件模式')) {
    return
  }

  resetFileDraft()
  workspaceMode.value = 'add-file'
  setStatus('已进入新增文件模式。')
}

function cancelComposer() {
  workspaceMode.value = 'browse'
  resetCreateDraft()
  resetFileDraft()
  setStatus('已取消当前新增操作。')
}

async function api(path, options = {}) {
  if (window.gistDesktop?.request) {
    const body = options.body ? JSON.parse(options.body) : undefined

    if (path === '/session') {
      return window.gistDesktop.request('session', {
        token: token.value
      })
    }

    if (path === '/gists' && options.method === 'POST') {
      return window.gistDesktop.request('create-gist', {
        token: token.value,
        payload: body
      })
    }

    if (path === '/gists') {
      return window.gistDesktop.request('list-gists', {
        token: token.value
      })
    }

    const gistMatch = path.match(/^\/gists\/([^/]+)$/)

    if (gistMatch) {
      const gistId = gistMatch[1]

      if (options.method === 'PATCH') {
        return window.gistDesktop.request('update-gist', {
          token: token.value,
          gistId,
          payload: body
        })
      }

      if (options.method === 'DELETE') {
        return window.gistDesktop.request('delete-gist', {
          token: token.value,
          gistId
        })
      }

      return window.gistDesktop.request('get-gist', {
        token: token.value,
        gistId
      })
    }

    throw new Error(`不支持的客户端请求：${path}`)
  }

  const response = await fetch(`${webApiBase}${path}`, {
    ...options,
    headers: {
      Authorization: `Bearer ${token.value}`,
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...(options.headers || {})
    }
  })

  if (response.status === 204) {
    return null
  }

  const data = await response.json()

  if (!response.ok) {
    throw new Error(data.message || '请求失败。')
  }

  return data
}

function openExternal(url) {
  if (!url) {
    return
  }

  if (window.gistDesktop?.openExternal) {
    window.gistDesktop.openExternal(url)
    return
  }

  window.open(url, '_blank', 'noopener,noreferrer')
}

async function copyToClipboard(text, successMessage) {
  if (!text) {
    setStatus('', '没有可复制的链接。')
    return
  }

  try {
    if (window.gistDesktop?.copyText) {
      await window.gistDesktop.copyText(text)
    } else {
      await navigator.clipboard.writeText(text)
    }
    setStatus(successMessage)
  } catch {
    setStatus('', '复制失败，请检查系统剪贴板权限。')
  }
}

function openCurrentFileRaw() {
  openExternal(currentFile.value?.rawUrl)
}

function copyCurrentFileRaw() {
  copyToClipboard(currentFile.value?.rawUrl, 'Raw 链接已复制。')
}

function syncSelection(data, preferredFilename = '') {
  gist.value = data
  workspaceMode.value = 'browse'
  editMode.value = false
  description.value = data.description || ''
  const nextFilename = preferredFilename || Object.keys(data.files || {})[0] || ''
  selectedFile.value = nextFilename
  editorContent.value = data.files?.[nextFilename]?.content || ''
  renameTarget.value = nextFilename
}

function confirmIfDirty(actionLabel) {
  if (!hasUnsavedContent.value && !hasUnsavedDescription.value && !hasComposerChanges.value) {
    return true
  }

  return window.confirm(`当前有未保存改动，确定要继续${actionLabel}吗？`)
}

async function refreshGists() {
  const data = await api('/gists')
  gists.value = data.items
}

async function connectWorkspace() {
  setStatus()

  if (!tokenDraft.value.trim()) {
    setStatus('', '请先输入 GitHub Token。')
    return
  }

  token.value = tokenDraft.value.trim()
  loadingSession.value = true
  try {
    const [sessionData, gistData] = await Promise.all([
      api('/session'),
      api('/gists')
    ])

    profile.value = sessionData
    gists.value = gistData.items
    await persistToken()

    if (gistData.items[0]?.id) {
      const detail = await api(`/gists/${gistData.items[0].id}`)
      syncSelection(detail)
    } else {
      gist.value = null
      selectedFile.value = ''
      editorContent.value = ''
      renameTarget.value = ''
    }

    setStatus(`工作区已连接，加载到 ${gistData.items.length} 个 gist。`)
  } catch (error) {
    setStatus('', error.message)
  } finally {
    loadingSession.value = false
  }
}

async function startReplaceToken() {
  token.value = ''
  tokenDraft.value = ''
  profile.value = null
  gists.value = []
  gist.value = null
  selectedFile.value = ''
  editorContent.value = ''
  renameTarget.value = ''
  description.value = ''
  editMode.value = false
  workspaceMode.value = 'browse'
  await clearPersistedToken()
  setStatus('请输入新的 Token 重新连接。')
}

onMounted(initializeApp)
onBeforeUnmount(() => removeUpdateListener?.())

async function selectGist(gistId) {
  if (!gistId || gistId === selectedGistId.value) {
    return
  }

  if (!confirmIfDirty('切换 Gist')) {
    return
  }

  setStatus()
  loadingDetail.value = true

  try {
    const detail = await api(`/gists/${gistId}`)
    syncSelection(detail)
    setStatus('Gist 已切换。')
  } catch (error) {
    setStatus('', error.message)
  } finally {
    loadingDetail.value = false
  }
}

function selectFile(filename) {
  if (!filename || filename === selectedFile.value) {
    return
  }

  if (!confirmIfDirty('切换文件')) {
    return
  }

  selectedFile.value = filename
  editMode.value = false
  editorContent.value = gist.value?.files?.[filename]?.content || ''
  renameTarget.value = filename
  setStatus('文件已切换。')
}

function startEditing() {
  if (!currentFile.value) {
    setStatus('', '请先选择文件。')
    return
  }

  editMode.value = true
  setStatus('已进入编辑模式。')
}

function cancelEditing() {
  if (!gist.value || !currentFile.value) {
    editMode.value = false
    return
  }

  if (hasUnsavedContent.value || hasUnsavedDescription.value) {
    const shouldDiscard = window.confirm('当前有未保存改动，确定要放弃并退出编辑模式吗？')

    if (!shouldDiscard) {
      return
    }
  }

  description.value = gist.value.description || ''
  editorContent.value = currentFile.value.content || ''
  editMode.value = false
  setStatus('已退出编辑模式。')
}

async function createGist() {
  setStatus()

  if (!draftGistFilename.value.trim()) {
    setStatus('', '创建 gist 时必须提供初始文件名。')
    return
  }

  saving.value = true

  try {
    const created = await api('/gists', {
      method: 'POST',
      body: JSON.stringify({
        description: draftGistDescription.value.trim(),
        public: draftGistPublic.value,
        files: {
          [draftGistFilename.value.trim()]: {
            content: draftGistContent.value
          }
        }
      })
    })

    resetCreateDraft()
    await refreshGists()
    syncSelection(created)
    setStatus('新的 gist 已创建。')
  } catch (error) {
    setStatus('', error.message)
  } finally {
    saving.value = false
  }
}

async function addFile() {
  setStatus()

  if (!selectedGistId.value) {
    setStatus('', '请先选择一个 gist。')
    return
  }

  const nextName = draftFileName.value.trim()

  if (!nextName) {
    setStatus('', '新增文件时必须填写文件名。')
    return
  }

  saving.value = true

  try {
    const updated = await api(`/gists/${selectedGistId.value}`, {
      method: 'PATCH',
      body: JSON.stringify({
        description: description.value,
        files: {
          [nextName]: {
            content: draftFileContent.value
          }
        }
      })
    })

    resetFileDraft()
    await refreshGists()
    syncSelection(updated, nextName)
    setStatus('文件已新增。')
  } catch (error) {
    setStatus('', error.message)
  } finally {
    saving.value = false
  }
}

async function saveCurrent() {
  setStatus()

  if (!currentFile.value) {
    setStatus('', '请先选择要保存的文件。')
    return
  }

  saving.value = true

  try {
    const updated = await api(`/gists/${selectedGistId.value}`, {
      method: 'PATCH',
      body: JSON.stringify({
        description: description.value,
        files: {
          [selectedFile.value]: {
            content: editorContent.value
          }
        }
      })
    })

    await refreshGists()
    syncSelection(updated, selectedFile.value)
    setStatus('当前文件和描述已保存。')
  } catch (error) {
    setStatus('', error.message)
  } finally {
    saving.value = false
  }
}

async function renameFile() {
  setStatus()

  if (!currentFile.value) {
    setStatus('', '请先选择文件。')
    return
  }

  const targetName = renameTarget.value.trim()

  if (!targetName) {
    setStatus('', '文件名不能为空。')
    return
  }

  if (targetName === selectedFile.value) {
    setStatus('文件名未变化。')
    return
  }

  saving.value = true

  try {
    const updated = await api(`/gists/${selectedGistId.value}`, {
      method: 'PATCH',
      body: JSON.stringify({
        description: description.value,
        files: {
          [selectedFile.value]: {
            filename: targetName,
            content: editorContent.value
          }
        }
      })
    })

    await refreshGists()
    syncSelection(updated, targetName)
    setStatus('文件已重命名。')
  } catch (error) {
    setStatus('', error.message)
  } finally {
    saving.value = false
  }
}

async function deleteFile() {
  setStatus()

  if (!currentFile.value) {
    setStatus('', '请先选择文件。')
    return
  }

  openConfirmDialog({
    title: '删除文件',
    message: `确定要删除文件“${selectedFile.value}”吗？这个操作无法撤销。`,
    actionText: '删除文件',
    onConfirm: async () => {
      saving.value = true

      try {
        const deletedName = selectedFile.value
        const updated = await api(`/gists/${selectedGistId.value}`, {
          method: 'PATCH',
          body: JSON.stringify({
            description: description.value,
            files: {
              [deletedName]: null
            }
          })
        })

        await refreshGists()
        syncSelection(updated)
        setStatus(`文件 ${deletedName} 已删除。`)
      } catch (error) {
        setStatus('', error.message)
      } finally {
        saving.value = false
      }
    }
  })
}

async function deleteGist() {
  setStatus()

  if (!selectedGistId.value) {
    setStatus('', '请先选择一个 gist。')
    return
  }

  openConfirmDialog({
    title: '删除 Gist',
    message: '确定要删除当前 Gist 吗？删除后其中的全部文件都无法恢复。',
    actionText: '删除 Gist',
    onConfirm: async () => {
      deleting.value = true

      try {
        await api(`/gists/${selectedGistId.value}`, { method: 'DELETE' })
        await refreshGists()

        const nextId = gists.value[0]?.id || ''

        if (nextId) {
          const detail = await api(`/gists/${nextId}`)
          syncSelection(detail)
        } else {
          gist.value = null
          selectedFile.value = ''
          editorContent.value = ''
          renameTarget.value = ''
        }

        setStatus('Gist 已删除。')
      } catch (error) {
        setStatus('', error.message)
      } finally {
        deleting.value = false
      }
    }
  })
}
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <div class="brand-block">
        <img class="brand-icon" src="/favicon.ico" alt="Gist管理器图标" />
        <div>
          <h1>Gist管理器</h1>
        </div>
      </div>

      <div class="topbar-actions" v-if="securityReady && !securityState.locked">
        <button v-if="isConnected" class="ghost-button compact-button" @click="startReplaceToken">更换 Token</button>
        <button
          v-if="hasDesktopUpdates"
          class="ghost-button compact-button update-trigger"
          :class="{ 'has-update': ['available', 'downloaded'].includes(updateState.status) }"
          :disabled="updateBusy"
          @click="checkForUpdates"
        >
          {{ updateState.status === 'checking'
            ? '检查中…'
            : updateState.status === 'downloading'
              ? `下载 ${Math.round(updateState.percent)}%`
            : ['available', 'downloaded'].includes(updateState.status)
              ? `新版本 ${updateState.availableVersion}`
              : '检查更新' }}
        </button>
        <button
          v-if="securityState.settings.lockEnabled"
          class="ghost-button compact-button"
          @click="lockApplication"
        >
          立即锁定
        </button>
        <button v-if="hasDesktopSecurity" class="ghost-button compact-button" @click="openSettings">
          系统设置
        </button>
      </div>
    </header>

    <section class="status-strip">
      <div class="profile-chip" v-if="profile">
        <strong>{{ profile.name || profile.login }}</strong>
        <span>@{{ profile.login }}</span>
      </div>
      <div class="status-content">
        <span v-if="statusMessage" class="status-text success">{{ statusMessage }}</span>
        <span v-if="errorMessage" class="status-text error">{{ errorMessage }}</span>
        <span v-if="gist?.updatedAt" class="status-text muted">
          上次同步 {{ new Date(gist.updatedAt).toLocaleString() }}
        </span>
      </div>
      <button
        v-if="isConnected"
        class="ghost-button compact-button"
        :disabled="loadingSession"
        @click="refreshGists"
      >
        刷新列表
      </button>
    </section>

    <main class="workbench">
      <aside class="panel gist-panel">
        <div class="panel-header">
          <div>
            <h2>账号资源</h2>
          </div>
          <button class="ghost-button compact-button" @click="openCreateGistComposer">新建</button>
        </div>

        <input
          v-model="searchText"
          class="search-input"
          type="text"
          placeholder="搜索描述、ID 或文件名"
        />

        <div class="list-scroll">
          <button
            v-for="item in filteredGists"
            :key="item.id"
            class="gist-card"
            :class="{ active: item.id === selectedGistId }"
            @click="selectGist(item.id)"
          >
            <strong>{{ item.description }}</strong>
            <span>{{ item.public ? '公开' : '私有' }} · {{ item.fileCount }} 个文件</span>
            <code>{{ item.id }}</code>
          </button>
          <div v-if="!filteredGists.length" class="empty-list">没有匹配的 gist。</div>
        </div>
      </aside>

      <aside class="panel file-panel">
        <template v-if="gist">
          <div class="panel-header">
            <div>
              <h2>{{ gist.description || '(未命名 Gist)' }}</h2>
            </div>
            <div class="button-row">
              <button class="ghost-button compact-button" @click="openExternal(gist.htmlUrl)">网页打开</button>
            </div>
          </div>

          <section class="inline-card form-stack">
            <div class="section-title">Gist 信息</div>
            <input
              v-model="description"
              :disabled="!editMode"
              type="text"
              placeholder="编辑 gist 描述"
            />
            <div class="meta-row">
              <span>{{ gist.public ? '公开' : '私有' }}</span>
              <code>{{ gist.id }}</code>
            </div>
          </section>

          <section class="inline-card form-stack">
            <div class="section-title section-with-action">
              <span>文件树</span>
              <button class="ghost-button compact-button" @click="openAddFileComposer">新增文件</button>
            </div>

            <div class="list-scroll compact">
              <button
                v-for="file in files"
                :key="file.filename"
                class="file-card"
                :class="{ active: file.filename === selectedFile }"
                @click="selectFile(file.filename)"
              >
                <strong>{{ file.filename }}</strong>
                <span>{{ file.language || '纯文本' }} · {{ file.size }} 字节</span>
              </button>
            </div>
          </section>

          <section class="inline-card form-stack" v-if="currentFile">
            <div class="section-title">当前文件设置</div>
            <input v-model="renameTarget" type="text" placeholder="重命名文件" />
            <div class="button-row">
              <button class="ghost-button compact-button" :disabled="saving" @click="renameFile">重命名</button>
              <button class="danger-button compact-button" :disabled="saving" @click="deleteFile">删除文件</button>
              <button class="danger-button compact-button" :disabled="deleting" @click="deleteGist">删除 Gist</button>
            </div>
          </section>
        </template>

        <div v-else class="empty-pane">
          <p class="eyebrow">Explorer</p>
          <h2>先连接 GitHub 工作区</h2>
          <p>连接后这里会显示 gist 列表、文件树和操作面板。</p>
        </div>
      </aside>

      <section class="panel editor-panel">
        <template v-if="workspaceMode === 'create-gist'">
          <div class="editor-header">
            <div>
              <h2>新建 Gist</h2>
            </div>
            <div class="button-row">
              <label class="checkbox-row subtle-check">
                <input v-model="draftGistPublic" type="checkbox" />
                <span>公开</span>
              </label>
              <button class="ghost-button compact-button" @click="cancelComposer">取消</button>
              <button class="primary-button compact-button" :disabled="saving" @click="createGist">创建</button>
            </div>
          </div>

          <div class="composer-meta">
            <input v-model="draftGistDescription" type="text" placeholder="Gist 描述" />
            <input v-model="draftGistFilename" type="text" placeholder="初始文件名" />
          </div>

          <div class="editor-frame">
            <CodeEditor
              v-model="draftGistContent"
              :filename="draftGistFilename || 'notes.txt'"
              :read-only="saving"
            />
          </div>

          <footer class="editor-footer">
            <span>在这里直接编写初始文件内容</span>
            <span>{{ draftGistFilename || '未命名文件' }}</span>
          </footer>
        </template>

        <template v-else-if="workspaceMode === 'add-file'">
          <div class="editor-header">
            <div>
              <h2>新增文件到当前 Gist</h2>
            </div>
            <div class="button-row">
              <button class="ghost-button compact-button" @click="cancelComposer">取消</button>
              <button class="primary-button compact-button" :disabled="saving" @click="addFile">添加文件</button>
            </div>
          </div>

          <div class="composer-meta single">
            <input v-model="draftFileName" type="text" placeholder="新文件名" />
          </div>

          <div class="editor-frame">
            <CodeEditor
              v-model="draftFileContent"
              :filename="draftFileName || 'new-file.txt'"
              :read-only="saving"
            />
          </div>

          <footer class="editor-footer">
            <span>新增文件会写入当前选中的 Gist</span>
            <span>{{ gist?.description || '(未命名 Gist)' }}</span>
          </footer>
        </template>

        <template v-else-if="gist && currentFile">
          <div class="editor-header">
            <div>
              <h2>{{ currentFile.filename }}</h2>
            </div>
            <div class="button-row">
              <span class="editor-badge">{{ currentFile.language || '纯文本' }}</span>
              <button class="ghost-button compact-button" @click="copyCurrentFileRaw">复制 Raw 链接</button>
              <button class="ghost-button compact-button" @click="openCurrentFileRaw">打开 Raw</button>
              <button
                v-if="!editMode"
                class="ghost-button compact-button"
                :disabled="loadingDetail || saving"
                @click="startEditing"
              >
                编辑
              </button>
              <button
                v-else
                class="ghost-button compact-button"
                :disabled="saving"
                @click="cancelEditing"
              >
                取消编辑
              </button>
              <button
                class="primary-button compact-button"
                :disabled="!canSave"
                @click="saveCurrent"
              >
                {{ saving ? '保存中...' : '保存改动' }}
              </button>
            </div>
          </div>

          <div class="editor-frame">
            <CodeEditor
              v-model="editorContent"
              :filename="currentFile.filename"
              :read-only="loadingDetail || saving || !editMode"
            />
          </div>

          <footer class="editor-footer">
            <span>{{ editMode ? '编辑模式' : '查看模式' }}</span>
            <span>{{ hasUnsavedContent ? '内容已修改' : '内容已同步' }}</span>
            <span>{{ hasUnsavedDescription ? '描述待保存' : '描述已同步' }}</span>
            <span>{{ currentFile.size }} 字节</span>
          </footer>
        </template>

        <div v-else class="empty-pane editor-empty">
          <h2>选择一个文件开始编辑</h2>
          <p>这里会显示代码编辑器、语法高亮和保存操作。</p>
        </div>
      </section>
    </main>

    <div v-if="confirmDialog.open" class="dialog-backdrop" @click="closeConfirmDialog">
      <div class="dialog-card" @click.stop>
        <div class="dialog-title">{{ confirmDialog.title }}</div>
        <p class="dialog-message">{{ confirmDialog.message }}</p>
        <div class="dialog-actions">
          <button class="ghost-button compact-button" @click="closeConfirmDialog">取消</button>
          <button class="danger-button compact-button" @click="confirmDialogAction">
            {{ confirmDialog.actionText }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="updateDialogOpen" class="dialog-backdrop" @click="!updateBusy && (updateDialogOpen = false)">
      <div class="dialog-card update-card" @click.stop>
        <div class="update-heading">
          <div class="update-icon">↻</div>
          <div>
            <div class="dialog-title">软件更新</div>
            <p class="dialog-message">当前版本 v{{ updateState.currentVersion || '—' }}</p>
          </div>
        </div>

        <div class="update-summary">
          <strong v-if="updateState.status === 'available'">发现新版本 v{{ updateState.availableVersion }}</strong>
          <strong v-else-if="updateState.status === 'downloaded'">v{{ updateState.availableVersion }} 已准备就绪</strong>
          <strong v-else-if="updateState.status === 'checking'">正在连接 GitHub Releases</strong>
          <strong v-else-if="updateState.status === 'downloading'">正在下载 v{{ updateState.availableVersion }}</strong>
          <strong v-else-if="updateState.status === 'not-available'">已经是最新版本</strong>
          <strong v-else-if="updateState.status === 'development'">开发环境</strong>
          <strong v-else-if="updateState.status === 'error'">更新检查失败</strong>
          <strong v-else>检查可用更新</strong>
          <span>{{ updateState.message || '点击检查更新获取最新版本。' }}</span>
        </div>

        <div class="update-channel-row">
          <div>
            <strong>版本通道</strong>
            <span>{{ updateState.channel === 'beta' ? '优先获取最新测试版本' : '仅获取稳定正式版本' }}</span>
          </div>
          <div class="channel-switch" role="group" aria-label="更新版本通道">
            <button
              type="button"
              :class="{ active: updateState.channel === 'stable' }"
              :disabled="updateBusy"
              @click="selectUpdateChannel('stable')"
            >
              正式版
            </button>
            <button
              type="button"
              :class="{ active: updateState.channel === 'beta' }"
              :disabled="updateBusy"
              @click="selectUpdateChannel('beta')"
            >
              测试版
            </button>
          </div>
        </div>

        <div v-if="updateState.status === 'downloading'" class="update-progress">
          <div class="update-progress-track">
            <span :style="{ width: `${updateState.percent}%` }"></span>
          </div>
          <div class="update-progress-meta">
            <span>{{ Math.round(updateState.percent) }}%</span>
            <span>{{ formatBytes(updateState.transferred) }} / {{ formatBytes(updateState.total) }}</span>
          </div>
        </div>

        <div class="dialog-actions">
          <button
            class="ghost-button compact-button"
            :disabled="updateBusy"
            @click="updateDialogOpen = false"
          >
            稍后
          </button>
          <button
            v-if="updateState.status === 'available'"
            class="primary-button compact-button"
            @click="downloadUpdate"
          >
            下载更新
          </button>
          <button
            v-else-if="updateState.status === 'downloaded'"
            class="primary-button compact-button"
            @click="installUpdate"
          >
            重启并安装
          </button>
          <button
            v-else-if="!updateBusy"
            class="primary-button compact-button"
            @click="checkForUpdates"
          >
            重新检查
          </button>
        </div>
      </div>
    </div>

    <div v-if="settingsOpen" class="dialog-backdrop" @click="settingsOpen = false">
      <div class="dialog-card settings-card" @click.stop>
        <div>
          <div class="dialog-title">系统与解锁设置</div>
          <p class="dialog-message">配置启动保护、系统身份验证和软件解锁密码。</p>
        </div>

        <label class="settings-toggle">
          <span>
            <strong>启动时锁定</strong>
            <small>每次启动应用都需要先验证身份</small>
          </span>
          <input v-model="securityForm.lockEnabled" type="checkbox" />
        </label>

        <label class="settings-toggle" :class="{ disabled: !securityState.biometric.available }">
          <span>
            <strong>{{ securityState.biometric.label }}</strong>
            <small>
              {{ securityState.biometric.available
                ? '优先调用系统验证，失败或不可用时使用软件密码'
                : securityState.biometric.reason }}
            </small>
          </span>
          <input
            v-model="securityForm.biometricEnabled"
            type="checkbox"
            :disabled="!securityForm.lockEnabled || !securityState.biometric.available"
          />
        </label>

        <div class="settings-fields">
          <label v-if="securityState.settings.hasPassword" class="token-field token-setup-field">
            <span>当前软件解锁密码</span>
            <input
              v-model="securityForm.currentPassword"
              type="password"
              autocomplete="current-password"
              placeholder="修改设置时需要验证"
            />
          </label>
          <label class="token-field token-setup-field">
            <span>{{ securityState.settings.hasPassword ? '新密码（留空则不修改）' : '软件解锁密码' }}</span>
            <input
              v-model="securityForm.newPassword"
              type="password"
              autocomplete="new-password"
              placeholder="至少 6 位"
            />
          </label>
          <label class="token-field token-setup-field">
            <span>确认新密码</span>
            <input
              v-model="securityForm.confirmPassword"
              type="password"
              autocomplete="new-password"
              placeholder="再次输入新密码"
            />
          </label>
        </div>

        <p v-if="settingsError" class="settings-error">{{ settingsError }}</p>
        <div class="dialog-actions">
          <button class="ghost-button compact-button" :disabled="settingsSaving" @click="settingsOpen = false">取消</button>
          <button class="primary-button compact-button" :disabled="settingsSaving" @click="saveSecuritySettings">
            {{ settingsSaving ? '保存中...' : '保存设置' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="needsTokenSetup" class="dialog-backdrop">
      <div class="dialog-card token-setup-card" @click.stop>
        <div class="dialog-title">连接 GitHub Token</div>
        <p class="dialog-message">
          首次使用需要输入具有 `gist` 权限的 GitHub Token。连接成功后，Token 将隐藏显示，并可随时更换。
        </p>
        <label class="token-field token-setup-field">
          <span>GitHub Token</span>
          <input
            v-model="tokenDraft"
            type="password"
            placeholder="github_pat_xxx / ghp_xxx"
            autocomplete="off"
            spellcheck="false"
          />
        </label>
        <div class="dialog-actions">
          <button class="primary-button compact-button" :disabled="loadingSession" @click="connectWorkspace">
            {{ loadingSession ? '连接中...' : '连接工作区' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="!securityReady" class="dialog-backdrop security-backdrop">
      <div class="security-loader">正在读取安全设置...</div>
    </div>

    <div v-else-if="securityState.locked" class="dialog-backdrop security-backdrop">
      <div class="dialog-card unlock-card" @click.stop>
        <div class="unlock-icon">🔒</div>
        <div class="unlock-heading">
          <div class="dialog-title">Gist管理器已锁定</div>
          <p class="dialog-message">验证身份后才能访问本地 Token 和 Gist 工作区。</p>
        </div>

        <button
          v-if="securityState.settings.biometricEnabled && securityState.biometric.available"
          class="primary-button biometric-button"
          :disabled="unlockingBiometric"
          @click="unlockWithBiometric"
        >
          {{ unlockingBiometric ? '正在等待系统验证...' : `使用 ${securityState.biometric.label} 解锁` }}
        </button>

        <div v-if="securityState.settings.biometricEnabled" class="unlock-divider">
          <span>或使用软件密码</span>
        </div>

        <form class="unlock-form" @submit.prevent="unlockWithPassword">
          <input
            v-model="unlockPassword"
            type="password"
            autocomplete="current-password"
            placeholder="输入软件解锁密码"
            autofocus
          />
          <button class="primary-button" type="submit">解锁</button>
        </form>
        <p v-if="unlockError" class="settings-error">{{ unlockError }}</p>
      </div>
    </div>
  </div>
</template>
