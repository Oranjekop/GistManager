<script setup>
import { computed, onMounted, ref } from 'vue'
import CodeEditor from './components/CodeEditor.vue'

const apiBase = window.gistDesktop?.apiBase || '/api'
const token = ref(localStorage.getItem('gist-editor-token') || '')
const tokenDraft = ref(localStorage.getItem('gist-editor-token') || '')
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
const needsTokenSetup = computed(() => !isConnected.value && !loadingSession.value)
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

function persistToken() {
  localStorage.setItem('gist-editor-token', token.value)
}

function clearPersistedToken() {
  localStorage.removeItem('gist-editor-token')
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

function getApiUrl(path) {
  return `${apiBase}${path}`
}

async function api(path, options = {}) {
  const response = await fetch(getApiUrl(path), {
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
  persistToken()

  try {
    const [sessionData, gistData] = await Promise.all([
      api('/session'),
      api('/gists')
    ])

    profile.value = sessionData
    gists.value = gistData.items

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

function startReplaceToken() {
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
  clearPersistedToken()
  setStatus('请输入新的 Token 重新连接。')
}

onMounted(() => {
  if (tokenDraft.value.trim()) {
    connectWorkspace()
  }
})

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

      <div class="topbar-actions" v-if="isConnected">
        <button class="ghost-button compact-button" @click="startReplaceToken">更换 Token</button>
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
  </div>
</template>
