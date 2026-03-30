const githubApiBase = 'https://api.github.com'

async function githubRequest(path, { method = 'GET', token, body } = {}) {
  const response = await fetch(`${githubApiBase}${path}`, {
    method,
    headers: {
      Accept: 'application/vnd.github+json',
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
      'X-GitHub-Api-Version': '2022-11-28',
      'User-Agent': 'gist-manager-app'
    },
    body: body ? JSON.stringify(body) : undefined
  })

  const isJson = response.headers.get('content-type')?.includes('application/json')
  const payload = isJson ? await response.json() : await response.text()

  if (!response.ok) {
    const message = typeof payload === 'object' && payload?.message
      ? payload.message
      : 'GitHub API 请求失败。'

    const error = new Error(message)
    error.status = response.status
    error.details = payload
    throw error
  }

  return payload
}

async function resolveFileContent(file) {
  if (!file.truncated) {
    return file.content ?? ''
  }

  const response = await fetch(file.raw_url)

  if (!response.ok) {
    throw new Error(`读取 ${file.filename} 的原始内容失败。`)
  }

  return response.text()
}

async function formatGistDetail(gist) {
  const fileEntries = await Promise.all(
    Object.entries(gist.files || {}).map(async ([filename, file]) => [
      filename,
      {
        filename,
        language: file.language,
        type: file.type,
        size: file.size,
        truncated: file.truncated,
        rawUrl: file.raw_url,
        content: await resolveFileContent(file)
      }
    ])
  )

  return {
    id: gist.id,
    description: gist.description || '',
    public: gist.public,
    htmlUrl: gist.html_url,
    createdAt: gist.created_at,
    updatedAt: gist.updated_at,
    owner: gist.owner?.login || 'unknown',
    files: Object.fromEntries(fileEntries)
  }
}

function formatGistSummary(gist) {
  return {
    id: gist.id,
    description: gist.description || '(无描述)',
    public: gist.public,
    htmlUrl: gist.html_url,
    createdAt: gist.created_at,
    updatedAt: gist.updated_at,
    fileCount: Object.keys(gist.files || {}).length,
    files: Object.values(gist.files || {}).map((file) => ({
      filename: file.filename,
      language: file.language,
      size: file.size,
      truncated: file.truncated
    }))
  }
}

function ensureToken(token) {
  if (!token?.trim()) {
    const error = new Error('缺少 GitHub Token。')
    error.status = 400
    throw error
  }

  return token.trim()
}

async function getSession(token) {
  const validToken = ensureToken(token)
  const user = await githubRequest('/user', { token: validToken })

  return {
    login: user.login,
    avatarUrl: user.avatar_url,
    htmlUrl: user.html_url,
    name: user.name
  }
}

async function listGists(token, { page = 1, perPage = 100 } = {}) {
  const validToken = ensureToken(token)
  const safePage = Number(page || 1)
  const safePerPage = Math.min(Number(perPage || 100), 100)
  const gists = await githubRequest(`/gists?page=${safePage}&per_page=${safePerPage}`, {
    token: validToken
  })

  return {
    items: gists.map(formatGistSummary),
    page: safePage,
    perPage: safePerPage
  }
}

async function getGist(token, gistId) {
  const validToken = ensureToken(token)
  const gist = await githubRequest(`/gists/${gistId}`, { token: validToken })
  return formatGistDetail(gist)
}

async function createGist(token, payload = {}) {
  const validToken = ensureToken(token)
  const { description = '', public: isPublic = false, files } = payload

  if (!files || typeof files !== 'object' || Object.keys(files).length === 0) {
    const error = new Error('创建 Gist 时至少需要一个文件。')
    error.status = 400
    throw error
  }

  const created = await githubRequest('/gists', {
    method: 'POST',
    token: validToken,
    body: {
      description,
      public: Boolean(isPublic),
      files
    }
  })

  return formatGistDetail(created)
}

async function updateGist(token, gistId, payload = {}) {
  const validToken = ensureToken(token)
  const { description, files } = payload

  if (!files || typeof files !== 'object' || Object.keys(files).length === 0) {
    const error = new Error('更新 Gist 时没有提供文件内容。')
    error.status = 400
    throw error
  }

  const updated = await githubRequest(`/gists/${gistId}`, {
    method: 'PATCH',
    token: validToken,
    body: {
      description,
      files
    }
  })

  return formatGistDetail(updated)
}

async function deleteGist(token, gistId) {
  const validToken = ensureToken(token)
  await githubRequest(`/gists/${gistId}`, {
    method: 'DELETE',
    token: validToken
  })

  return null
}

module.exports = {
  getSession,
  listGists,
  getGist,
  createGist,
  updateGist,
  deleteGist
}
