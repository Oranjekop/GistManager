import cors from 'cors'
import express from 'express'

const app = express()
const port = process.env.PORT || 3000
const githubApiBase = 'https://api.github.com'

app.use(cors())
app.use(express.json({ limit: '5mb' }))

function getTokenFromRequest(req) {
  const authHeader = req.headers.authorization

  if (authHeader?.startsWith('Bearer ')) {
    return authHeader.slice(7).trim()
  }

  return req.body?.token?.trim() || req.query?.token?.trim()
}

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
      : 'GitHub API request failed.'

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
    throw new Error(`Failed to fetch raw content for ${file.filename}.`)
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

function ensureToken(req, res) {
  const token = getTokenFromRequest(req)

  if (!token) {
    res.status(400).json({ message: 'Missing GitHub token.' })
    return null
  }

  return token
}

app.get('/api/session', async (req, res) => {
  const token = ensureToken(req, res)

  if (!token) {
    return
  }

  try {
    const user = await githubRequest('/user', { token })
    res.json({
      login: user.login,
      avatarUrl: user.avatar_url,
      htmlUrl: user.html_url,
      name: user.name
    })
  } catch (error) {
    res.status(error.status || 500).json({
      message: error.message || 'Failed to load user profile.',
      details: error.details
    })
  }
})

app.get('/api/gists', async (req, res) => {
  const token = ensureToken(req, res)

  if (!token) {
    return
  }

  const page = Number(req.query.page || 1)
  const perPage = Math.min(Number(req.query.per_page || 100), 100)

  try {
    const gists = await githubRequest(`/gists?page=${page}&per_page=${perPage}`, { token })
    res.json({
      items: gists.map(formatGistSummary),
      page,
      perPage
    })
  } catch (error) {
    res.status(error.status || 500).json({
      message: error.message || 'Failed to list gists.',
      details: error.details
    })
  }
})

app.post('/api/gists', async (req, res) => {
  const token = ensureToken(req, res)
  const { description = '', public: isPublic = false, files } = req.body || {}

  if (!token) {
    return
  }

  if (!files || typeof files !== 'object' || Object.keys(files).length === 0) {
    return res.status(400).json({ message: 'At least one file is required to create a gist.' })
  }

  try {
    const created = await githubRequest('/gists', {
      method: 'POST',
      token,
      body: {
        description,
        public: Boolean(isPublic),
        files
      }
    })

    const detail = await formatGistDetail(created)
    res.status(201).json(detail)
  } catch (error) {
    res.status(error.status || 500).json({
      message: error.message || 'Failed to create gist.',
      details: error.details
    })
  }
})

app.get('/api/gists/:gistId', async (req, res) => {
  const token = ensureToken(req, res)

  if (!token) {
    return
  }

  try {
    const gist = await githubRequest(`/gists/${req.params.gistId}`, { token })
    res.json(await formatGistDetail(gist))
  } catch (error) {
    res.status(error.status || 500).json({
      message: error.message || 'Failed to load gist.',
      details: error.details
    })
  }
})

app.patch('/api/gists/:gistId', async (req, res) => {
  const token = ensureToken(req, res)
  const { description, files } = req.body || {}

  if (!token) {
    return
  }

  if (!files || typeof files !== 'object' || Object.keys(files).length === 0) {
    return res.status(400).json({ message: 'No files provided for update.' })
  }

  try {
    const updated = await githubRequest(`/gists/${req.params.gistId}`, {
      method: 'PATCH',
      token,
      body: {
        description,
        files
      }
    })

    res.json(await formatGistDetail(updated))
  } catch (error) {
    res.status(error.status || 500).json({
      message: error.message || 'Failed to update gist.',
      details: error.details
    })
  }
})

app.delete('/api/gists/:gistId', async (req, res) => {
  const token = ensureToken(req, res)

  if (!token) {
    return
  }

  try {
    await githubRequest(`/gists/${req.params.gistId}`, {
      method: 'DELETE',
      token
    })

    res.status(204).send()
  } catch (error) {
    res.status(error.status || 500).json({
      message: error.message || 'Failed to delete gist.',
      details: error.details
    })
  }
})

app.listen(port, () => {
  console.log(`Gist manager server listening on http://localhost:${port}`)
})
