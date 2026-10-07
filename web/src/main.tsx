import { StrictMode, useEffect, useRef, useState, type ClipboardEvent, type DragEvent, type FormEvent } from 'react'
import { createRoot } from 'react-dom/client'
import {
  ArrowUp, ArrowUpRight, ChevronLeft, ChevronRight, Download, File, FileText, Image as ImageIcon,
  LogOut, Menu, Paperclip, Pencil, Plus, RotateCcw, Square, Trash2, UserCog, X,
} from 'lucide-react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import './style.css'

type User = { id: string; username: string; role: string; active: boolean }
type Session = { user: User; csrf_token: string }
type Citation = { fragment_id: string; title: string; section: string; url: string }
type AttachmentGap = { page?: number; code: string; text: string }
type Attachment = {
  id: string; conversation_id: string; name: string; media_type: string; sha256: string
  bytes: number; status: string; width?: number; height?: number; page_count?: number
  line_count?: number; available_pages?: number[]; available_lines?: number[]; gaps?: AttachmentGap[]
}
type AttachmentDraft = Attachment & {
  local_id: string; progress: number; error?: string
}
type AttachmentAsset = {
  id: string; name: string; media_type: string; kind: string; bytes: number; width?: number; height?: number
}
type AttachmentPage = {
  page: number; status: string; text?: string; text_sha256?: string; start_line?: number; end_line?: number
  tables?: { index: number; rows: string[][]; sha256: string }[]
  preview?: AttachmentAsset; images?: AttachmentAsset[]; error?: Failure
}
type AttachmentPageResult = {
  attachment: Attachment; page: AttachmentPage
  source: { attachment_hash: string; content_sha256: string; coverage_status: string; unparsed_summary?: string }
}
type Artifact = {
  id: string; execution_id: string; name: string; sha256: string; bytes: number; media_type: string
}
type PythonExecution = {
  id: string; response_id: string; conversation_id: string; code: string; code_sha256: string
  inputs: { id: string; kind: string; name: string; sha256: string; bytes: number }[]
  status: string; state_version: number; stdout_truncated?: boolean; stderr_truncated?: boolean
  result: {
    stdout: string; stderr: string; duration_ms: number; complete: boolean; cleaned: boolean
    error?: string; artifacts: Artifact[]
  }
}
type Answer = {
  id: string; conversation_id: string; question: string; status: string
  answer: string; data_mode: string; citations: Citation[]; gaps: string[]
  sequence: number; retry_of?: string
  attachments?: Attachment[]
  executions?: PythonExecution[]
  conflicts?: { subject: string; fragment_ids?: string[]; source_ids?: string[] }[]
  error?: { message: string }
  reasoning?: { text: string; status: string; event_id: number }
}
type Conversation = { id: string; title: string; state_version: number; created_at: string }
type Failure = { message?: string; code?: string }
type ResponseEvent = {
  id: number; type: string; response_id: string; status: string; draft_version?: number
  delta?: string; reason?: Failure; response?: Answer; execution?: PythonExecution
  reasoning_status?: string
}
type ProvisionalAnswer = { version: number; text: string }
let csrf = ''

function expireSession() {
  csrf = ''
  window.dispatchEvent(new Event('hwops:unauthorized'))
}

async function api<T>(path: string, method = 'GET', body?: unknown, key?: string): Promise<T> {
  const response = await fetch(path, {
    method, credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf, ...(key ? { 'Idempotency-Key': key } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const value = await response.json()
  if (!response.ok) {
    if (response.status === 401 && path !== '/v1/auth/login') expireSession()
    const failure = value as Failure
    throw new Error(failure.message || `请求失败 (${response.status})`)
  }
  return value as T
}

function uploadFile(path: string, file: globalThis.File, progress: (value: number) => void): Promise<Attachment> {
  return new Promise((resolve, reject) => {
    const request = new XMLHttpRequest()
    request.open('POST', path)
    request.withCredentials = true
    request.setRequestHeader('X-CSRF-Token', csrf)
    request.upload.onprogress = event => {
      if (event.lengthComputable) progress(Math.min(100, Math.round(event.loaded * 100 / event.total)))
    }
    request.onerror = () => reject(new Error('附件上传连接失败。'))
    request.onload = () => {
      let value: Attachment | Failure
      try { value = JSON.parse(request.responseText) }
      catch { reject(new Error(`附件上传失败 (${request.status})`)); return }
      if (request.status < 200 || request.status >= 300) {
        if (request.status === 401) expireSession()
        reject(new Error((value as Failure).message || `附件上传失败 (${request.status})`))
        return
      }
      resolve(value as Attachment)
    }
    const body = new FormData()
    body.append('file', file, file.name)
    request.send(body)
  })
}
const terminal = (r: Answer) => !['QUEUED', 'RUNNING', 'CANCELING'].includes(r.status)
const attachmentStatus: Record<string, string> = {
  UPLOADING: '上传中', UPLOADED: '等待解析', PROCESSING: '解析中', READY: '已就绪',
  PARTIAL: '部分可用', FAILED: '失败', UNSUPPORTED: '不支持',
}

function AttachmentTypeIcon({ mediaType, size = 16 }: { mediaType: string; size?: number }) {
  if (mediaType.startsWith('image/')) return <ImageIcon size={size} aria-hidden="true" />
  if (mediaType === 'application/pdf' || mediaType.startsWith('text/')) return <FileText size={size} aria-hidden="true" />
  return <File size={size} aria-hidden="true" />
}

const executionStatus: Record<string, string> = {
  QUEUED: '排队中', STARTING: '启动中', RUNNING: '运行中', SUCCEEDED: '已完成',
  FAILED: '失败', CANCELING: '停止中', CANCELED: '已停止', INTERRUPTED: '已中断',
}

function SafeMarkdown({ children }: { children: string }) {
  return <Markdown skipHtml remarkPlugins={[remarkGfm]} components={{
    img: ({ alt }) => <span className="blocked-resource">{alt ? `[图片已隐藏：${alt}]` : '[图片已隐藏]'}</span>,
    a: ({ children: label }) => <span className="blocked-resource">{label}</span>,
  }}>{children}</Markdown>
}

function mergeAnswer(previous: Answer | undefined, incoming: Answer): Answer {
  if (!previous) return incoming
  const result = terminal(previous) && !terminal(incoming) ? previous : incoming
  const reasoning = (previous.reasoning?.event_id || 0) > (incoming.reasoning?.event_id || 0) ?
    previous.reasoning : incoming.reasoning
  return { ...result, reasoning }
}

function ReasoningPanel({ answer }: { answer: Answer }) {
  const reasoning = answer.reasoning
  const running = reasoning?.status === 'RUNNING'
  const [expanded, setExpanded] = useState(running)
  useEffect(() => { setExpanded(running) }, [running])
  if (!reasoning?.text) return null
  const label = running ? '正在思考' : reasoning.status === 'COMPLETED' ? '思考已完成' : '思考已中断'
  return <section className={`reasoning-panel ${running ? 'running' : ''}`} aria-label="思考过程">
    <button type="button" className="reasoning-toggle" aria-expanded={expanded}
      aria-controls={`reasoning-${answer.id}`} onClick={() => setExpanded(value => !value)}>
      <ChevronRight size={15} aria-hidden="true" className={expanded ? 'expanded' : ''} />
      <span>{label}</span><small>{expanded ? '收起' : '展开'}</small>
    </button>
    {expanded && <div className="reasoning-content" id={`reasoning-${answer.id}`}>
      <SafeMarkdown>{reasoning.text}</SafeMarkdown>
    </div>}
  </section>
}

function ExecutionList({ values }: { values: PythonExecution[] }) {
  if (!values.length) return null
  return <div className="executions" aria-label="Python 执行记录">{values.map(execution =>
    <section className="execution-record" key={execution.id}>
      <div className="execution-heading">
        <strong>Python</strong><span className={`execution-status ${execution.status.toLowerCase()}`}>
          {executionStatus[execution.status] || execution.status}
        </span><code>{execution.id.slice(0, 10)}</code>
      </div>
      <pre className="execution-code"><code>{execution.code}</code></pre>
      {execution.result.stdout && <div className="execution-output"><span>标准输出</span>
        <pre>{execution.result.stdout}{execution.stdout_truncated ? '\n[输出已截断，请打开执行记录核对完整内容]' : ''}</pre>
      </div>}
      {execution.result.stderr && <div className="execution-output error-output"><span>标准错误</span>
        <pre>{execution.result.stderr}{execution.stderr_truncated ? '\n[输出已截断]' : ''}</pre>
      </div>}
      {execution.result.error && <p className="execution-error">{execution.result.error}</p>}
      {!!execution.result.artifacts?.length && <div className="artifacts">{execution.result.artifacts.map(artifact =>
        <a key={artifact.id} href={`/v1/artifacts/${artifact.id}/content`} target="_blank" rel="noreferrer">
          {artifact.media_type.startsWith('image/') ?
            <img src={`/v1/artifacts/${artifact.id}/content`} alt={artifact.name} /> :
            <FileText size={18} aria-hidden="true" />}
          <span>{artifact.name}</span><small>{(artifact.bytes / 1000).toFixed(1)} KB</small>
        </a>)}</div>}
    </section>)}</div>
}

function App() {
  const [session, setSession] = useState<Session | null>()
  const [error, setError] = useState('')
  useEffect(() => {
    const unauthorized = () => { setError(''); setSession(null) }
    window.addEventListener('hwops:unauthorized', unauthorized)
    api<Session>('/v1/auth/me').then(s => { csrf = s.csrf_token; setSession(s) })
      .catch(() => setSession(null))
    return () => window.removeEventListener('hwops:unauthorized', unauthorized)
  }, [])
  if (session === undefined) return <div className="loading" role="status">正在载入工作台…</div>
  if (session === null) return <Login onLogin={s => { csrf = s.csrf_token; setSession(s) }} />
  return <Workbench session={session} error={error} setError={setError} logout={async () => {
    try { await api('/v1/auth/logout', 'POST', {}); csrf = ''; setSession(null) }
    catch (e) { setError(String(e)) }
  }} />
}

function Login({ onLogin }: { onLogin: (s: Session) => void }) {
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    setBusy(true); setError('')
    try {
      onLogin(await api<Session>('/v1/auth/login', 'POST', {
        username: data.get('username'), password: data.get('password'),
      }))
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  return <main className="login-page">
    <div className="login-intro"><span className="brand-mark">✳</span><h1>知维</h1><p>让每一个判断，都有据可循。</p></div>
    <form className="login-card" onSubmit={submit}>
      <h2>登录工作台</h2><p className="muted">使用管理员为你开通的账号</p>
      <label>用户名<input name="username" autoComplete="username" required maxLength={64} autoFocus /></label>
      <label>密码<input name="password" type="password" autoComplete="current-password" required maxLength={72} /></label>
      {error && <p className="error" role="alert">{error}</p>}
      <button className="primary" disabled={busy}>{busy ? '正在登录…' : '登录'}</button>
      <p className="footnote">你的会话与文件仅对自己可见</p>
    </form>
  </main>
}

function Workbench({ session, logout, error, setError }: {
  session: Session; logout: () => void; error: string; setError: (v: string) => void
}) {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [current, setCurrent] = useState(() => new URL(location.href).searchParams.get('conversation') || '')
  const [answers, setAnswers] = useState<Answer[]>([])
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [provisional, setProvisional] = useState<Record<string, ProvisionalAnswer>>({})
  const [toolExecutions, setToolExecutions] = useState<Record<string, Record<string, PythonExecution>>>({})
  const text = drafts[current] || ''
  const setText = (value: string) => setDrafts(d => ({ ...d, [current]: value }))
  const [query, setQuery] = useState('')
  const [next, setNext] = useState('')
  const [metadata, setMetadata] = useState<Conversation | null>(null)
  const updateMetadata = (value: Conversation) => setMetadata(saved =>
    !saved || saved.id !== value.id || value.state_version >= saved.state_version ? value : saved)
  const [rename, setRename] = useState(false)
  const [title, setTitle] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [loading, setLoading] = useState(false)
  const request = useRef<{ conversation: string; question: string; attachments: string; key: string } | null>(null)
  const [sending, setSending] = useState(false)
  const [admin, setAdmin] = useState(false)
  const [source, setSource] = useState<Record<string, unknown> | null>(null)
  const [attachmentDrafts, setAttachmentDrafts] = useState<Record<string, AttachmentDraft[]>>({})
  const [attachmentPreview, setAttachmentPreview] = useState<Attachment | null>(null)
  const [dragging, setDragging] = useState(false)
  const [sidebar, setSidebar] = useState(false)
  const bottom = useRef<HTMLDivElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const eventCursors = useRef<Record<string, number>>({})
  const active = answers.find(a => a.conversation_id === current && !terminal(a))
  const currentAnswers = answers.filter(a => a.conversation_id === current).sort((a, b) => a.sequence - b.sequence)
  const currentAttachments = attachmentDrafts[current] || []
  const attachmentsReady = currentAttachments.every(a => ['READY', 'PARTIAL'].includes(a.status))
  const canSend = !sending && !loading && !active && attachmentsReady &&
    (!!text.trim() || currentAttachments.length > 0)
  const upsert = (answer: Answer) => setAnswers(rows => {
    const found = rows.some(r => r.id === answer.id)
    return found ? rows.map(r => r.id === answer.id ? mergeAnswer(r, answer) : r) : [...rows, answer]
  })
  async function loadList(search = query, cursor = '') {
    const result = await api<{ conversations: Conversation[]; next: string }>(
      `/v1/conversations/search?q=${encodeURIComponent(search)}&after=${encodeURIComponent(cursor)}`)
    setConversations(rows => cursor ? [...rows, ...result.conversations] : result.conversations)
    setNext(result.next)
  }
  useEffect(() => {
    let valid = true
    const timer = setTimeout(() => {
      api<{ conversations: Conversation[]; next: string }>(`/v1/conversations/search?q=${encodeURIComponent(query)}`)
        .then(result => { if (valid) { setConversations(result.conversations); setNext(result.next) } })
        .catch(e => { if (valid) setError(String(e)) })
    }, 150)
    return () => { valid = false; clearTimeout(timer) }
  }, [query])
  useEffect(() => {
    const url = new URL(location.href)
    if (current) url.searchParams.set('conversation', current)
    else url.searchParams.delete('conversation')
    history.replaceState(null, '', url)
    setMetadata(null); setRename(false); setDeleting(false); setSource(null); setAttachmentPreview(null)
    if (!current) return
    let valid = true
    setLoading(true)
    const load = async () => {
      const c = await api<Conversation>(`/v1/conversations/${current}`)
      const rows: Answer[] = []
      let after = 0
      do {
        const page = await api<{ messages: Answer[]; next: number }>(`/v1/conversations/${current}/messages?after=${after}`)
        rows.push(...page.messages); after = page.next
      } while (after && valid)
      if (valid) {
        updateMetadata(c)
        setAnswers(saved => {
          const merged = new Map(saved.filter(r => r.conversation_id === current).map(r => [r.id, r]))
          for (const r of rows) {
            const old = merged.get(r.id)
            merged.set(r.id, mergeAnswer(old, r))
          }
          return [...saved.filter(r => r.conversation_id !== current), ...merged.values()]
        })
        setLoading(false)
      }
    }
    load().catch(e => { if (valid) { setError(String(e)); setLoading(false) } })
    // A second tab can append or cancel while this tab is idle.
    const focus = () => { load().catch(e => { if (valid) setError(String(e)) }) }
    window.addEventListener('focus', focus)
    return () => { valid = false; window.removeEventListener('focus', focus) }
  }, [current])
  useEffect(() => { bottom.current?.scrollIntoView({ behavior: 'smooth' }) }, [answers, provisional, current])
  useEffect(() => {
    if (!active) return
    const events = new EventSource(`/v1/responses/${active.id}/events`)
    const update = (event: MessageEvent) => {
      const data = JSON.parse(event.data) as ResponseEvent
      const eventID = Number(event.lastEventId || data.id)
      if (!Number.isSafeInteger(eventID) || eventID <= (eventCursors.current[data.response_id] || 0)) return
      eventCursors.current[data.response_id] = eventID
      if (data.type === 'reasoning_delta' || data.type === 'reasoning_done') {
        setAnswers(rows => rows.map(answer => {
          if (answer.id !== data.response_id || eventID <= (answer.reasoning?.event_id || 0)) return answer
          return { ...answer, reasoning: {
            text: (answer.reasoning?.text || '') + (data.delta || ''),
            status: data.reasoning_status || 'RUNNING', event_id: eventID,
          } }
        }))
      }
      if (data.type === 'draft_started' && data.draft_version) {
        setProvisional(saved => ({
          ...saved, [data.response_id]: { version: data.draft_version as number, text: '' },
        }))
      } else if (data.type === 'answer_delta' && data.draft_version && data.delta) {
        setProvisional(saved => {
          const draft = saved[data.response_id]
          if (!draft || draft.version !== data.draft_version) return saved
          return { ...saved, [data.response_id]: { ...draft, text: draft.text + data.delta } }
        })
      } else if (data.type === 'draft_retracted' && data.draft_version) {
        setProvisional(saved => {
          if (saved[data.response_id]?.version !== data.draft_version) return saved
          const next = { ...saved }
          delete next[data.response_id]
          return next
        })
      }
      if (data.type === 'tool_progress' && data.execution) {
        setToolExecutions(saved => {
          const current = saved[data.response_id]?.[data.execution!.id]
          if (current && current.state_version > data.execution!.state_version) return saved
          return {
            ...saved,
            [data.response_id]: { ...(saved[data.response_id] || {}), [data.execution!.id]: data.execution! },
          }
        })
      }
      if (data.response) {
        upsert(data.response)
        if (terminal(data.response)) {
          setProvisional(saved => {
            const next = { ...saved }
            delete next[data.response_id]
            return next
          })
          events.close()
        }
      }
    }
    for (const kind of ['accepted', 'progress', 'draft_started', 'answer_delta', 'draft_retracted', 'tool_progress',
      'reasoning_delta', 'reasoning_done',
      'answer', 'failed', 'canceled', 'interrupted', 'clarification_required']) {
      events.addEventListener(kind, update as EventListener)
    }
    events.onerror = () => {
      api<Answer>(`/v1/responses/${active.id}`).then(answer => {
        upsert(answer)
        if (terminal(answer)) events.close()
      }).catch(e => { setError(String(e)); events.close() })
    }
    return () => events.close()
  }, [active?.id])
  async function newConversation() {
    const c = await api<Conversation>('/v1/conversations', 'POST', {})
    setConversations(rows => [c, ...rows]); setCurrent(c.id); setSidebar(false); setAdmin(false)
    return c.id
  }
  function updateAttachment(conversation: string, localID: string, update: (value: AttachmentDraft) => AttachmentDraft) {
    setAttachmentDrafts(saved => ({
      ...saved,
      [conversation]: (saved[conversation] || []).map(value => value.local_id === localID ? update(value) : value),
    }))
  }
  async function addFiles(files: globalThis.File[]) {
    if (!files.length) return
    const existing = currentAttachments
    if (existing.length + files.length > 5) {
      setError('每条消息最多添加 5 个附件。')
      return
    }
    if (files.some(file => file.size > 50_000_000)) {
      setError('单个附件不能超过 50,000,000 字节。')
      return
    }
    if (existing.reduce((sum, file) => sum + file.bytes, 0) + files.reduce((sum, file) => sum + file.size, 0) > 100_000_000) {
      setError('每条消息的附件总大小不能超过 100,000,000 字节。')
      return
    }
    setError('')
    const conversation = current || await newConversation()
    for (const file of files) {
      const localID = crypto.randomUUID()
      const pending: AttachmentDraft = {
        local_id: localID, id: '', conversation_id: conversation, name: file.name, media_type: file.type,
        sha256: '', bytes: file.size, status: 'UPLOADING', progress: 0,
      }
      setAttachmentDrafts(saved => ({ ...saved, [conversation]: [...(saved[conversation] || []), pending] }))
      void (async () => {
        try {
          let attachment = await uploadFile(`/v1/conversations/${conversation}/attachments`, file,
            progress => updateAttachment(conversation, localID, value => ({ ...value, progress })))
          updateAttachment(conversation, localID, () => ({ ...attachment, local_id: localID, progress: 100 }))
          while (!['READY', 'PARTIAL', 'FAILED', 'UNSUPPORTED'].includes(attachment.status)) {
            await new Promise(resolve => setTimeout(resolve, 250))
            attachment = await api<Attachment>(`/v1/attachments/${attachment.id}`)
            updateAttachment(conversation, localID, () => ({ ...attachment, local_id: localID, progress: 100 }))
          }
        } catch (e) {
          updateAttachment(conversation, localID, value => ({
            ...value, status: 'FAILED', error: e instanceof Error ? e.message : String(e),
          }))
        }
      })()
    }
  }
  function removeAttachment(localID: string) {
    setAttachmentDrafts(saved => ({
      ...saved,
      [current]: (saved[current] || []).filter(value => value.local_id !== localID),
    }))
  }
  function pasteFiles(event: ClipboardEvent<HTMLTextAreaElement>) {
    const files = Array.from(event.clipboardData.files).filter(file => file.type.startsWith('image/'))
    if (files.length) {
      event.preventDefault()
      void addFiles(files)
    }
  }
  function dropFiles(event: DragEvent<HTMLFormElement>) {
    event.preventDefault()
    setDragging(false)
    void addFiles(Array.from(event.dataTransfer.files))
  }
  async function send(event: FormEvent) {
    event.preventDefault()
    if (!canSend) return
    setSending(true); setError('')
    const question = text
    try {
      const id = current || await newConversation()
      const attachmentIDs = currentAttachments.map(value => value.id)
      const attachmentKey = attachmentIDs.join('\x00')
      if (!request.current || request.current.conversation !== id || request.current.question !== question ||
        request.current.attachments !== attachmentKey) {
        request.current = { conversation: id, question, attachments: attachmentKey, key: crypto.randomUUID() }
      }
      const answer = await api<Answer>(`/v1/conversations/${id}/messages`, 'POST',
        { text: question, attachment_ids: attachmentIDs }, request.current.key)
      upsert(answer); setDrafts(d => ({ ...d, [current]: '', [id]: '' })); request.current = null
      setAttachmentDrafts(saved => ({ ...saved, [id]: [] }))
      const c = await api<Conversation>(`/v1/conversations/${id}`)
      updateMetadata(c); await loadList()
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setSending(false) }
  }
  return <div className="workbench">
    <aside className={`sidebar ${sidebar ? 'open' : ''}`}>
      <a className="brand" href="/"><span className="brand-mark">✳</span> 知维<span className="brand-caption">硬件运维助手</span></a>
      <button className="new-chat" onClick={() => newConversation().catch(e => setError(String(e)))}>
        <Plus size={17} aria-hidden="true" />新建会话
      </button>
      <input className="history-search" aria-label="搜索历史" placeholder="搜索标题、问题和回答" value={query} onChange={e => setQuery(e.target.value)} maxLength={256} />
      <p className="nav-label">会话</p>
      <nav aria-label="会话列表">{conversations.map(c => <button key={c.id} className={c.id === current ? 'selected' : ''} onClick={() => {
        setCurrent(c.id); setAdmin(false); setSidebar(false)
      }}>{c.title}</button>)}</nav>
      {next && <button onClick={() => loadList(query, next).catch(e => setError(String(e)))}>加载更多会话</button>}
      <div className="account">
        {session.user.role === 'ADMIN' && <button className="text-icon-button" onClick={() => setAdmin(!admin)}>
          <UserCog size={14} aria-hidden="true" />账号管理
        </button>}
        <div className="account-row"><span className="avatar">{session.user.username[0].toUpperCase()}</span><span>{session.user.username}</span>
          <button className="text-icon-button" onClick={logout}><LogOut size={13} aria-hidden="true" />退出</button>
        </div>
      </div>
    </aside>
    <main className="chat-main">
      <header className="topbar"><button className="menu icon-button" title="切换侧栏" aria-label="切换侧栏" onClick={() => setSidebar(!sidebar)}>
        <Menu size={19} aria-hidden="true" />
      </button><span className="current-title">{admin ? '账号管理' : metadata?.title || '新会话'}</span>
        {!admin && metadata && <><button className="text-icon-button" aria-label="重命名会话" onClick={() => { setTitle(metadata.title); setRename(true) }}>
          <Pencil size={13} aria-hidden="true" />重命名
        </button><button className="text-icon-button" aria-label="删除会话" onClick={() => setDeleting(true)}>
          <Trash2 size={13} aria-hidden="true" />删除
        </button></>}
        <span className="private-tag">私有会话</span></header>
      {error && <div className="global-error" role="alert">{error}<button className="icon-button" title="关闭错误" aria-label="关闭错误" onClick={() => setError('')}>
        <X size={16} aria-hidden="true" />
      </button></div>}
      {admin ? <Accounts /> : <>
        <div className="messages">
          {loading && <p role="status">正在载入会话…</p>}
          {!loading && currentAnswers.length === 0 && <section className="welcome"><span className="brand-mark">✳</span><h1>今天，一起解决什么问题？</h1><p>从一个问题开始，查阅资料、核对依据。</p><div className="suggestions">{['帮我解释设备告警', '查找运维手册中的操作步骤', '核对不同版本的配置要求'].map(q => <button key={q} onClick={() => setText(q)}>{q} <ArrowUpRight size={13} aria-hidden="true" /></button>)}</div></section>}
          {currentAnswers.map(a => <article className="turn" key={a.id}>
            {a.question && <div className="question">{a.question}</div>}
            {!!a.attachments?.length && <div className="message-attachments">{a.attachments.map(file =>
              <button key={file.id} onClick={() => setAttachmentPreview(file)}>
                <AttachmentTypeIcon mediaType={file.media_type} />
                <span>{file.name}</span><small>{file.status === 'PARTIAL' ? '部分可用' : '本会话附件'}</small>
              </button>)}</div>}
            {a.retry_of && <p className="footnote">重试记录 · 保留原尝试</p>}
            <div className="answer"><span className="answer-mark">✳</span><div className="answer-body">
              <ReasoningPanel answer={a} />
              <ExecutionList values={Object.values({
                ...(toolExecutions[a.id] || {}),
                ...Object.fromEntries((a.executions || []).map(execution => [execution.id, execution])),
              })} />
              {!terminal(a) ? provisional[a.id]?.text ? <div className="provisional" role="status">
                <span className="draft-badge">生成中草稿</span>
                <SafeMarkdown>{provisional[a.id].text}</SafeMarkdown>
                <p>正在核对来源与完整性…</p>
              </div> : <p className="processing" role="status">正在查阅资料并核对回答…</p> : <>
                {a.data_mode === 'REPLAY' && <span className="mode-badge">REPLAY · 确定性回放</span>}
                {a.answer && <SafeMarkdown>{a.answer}</SafeMarkdown>}
                {!a.answer && ['UNRESOLVED', 'NEEDS_CLARIFICATION'].includes(a.status) &&
                  <p>{a.status === 'NEEDS_CLARIFICATION' ? '需要补充信息' : '暂时无法回答'}</p>}
                {a.error && <p className="error" role="alert">{a.error.message}</p>}
                {['FAILED', 'CANCELED', 'INTERRUPTED'].includes(a.status) && <button className="text-icon-button" disabled={sending || !!active} onClick={async () => {
                  setSending(true)
                  try {
                    const r = await api<Answer>(`/v1/conversations/${current}/messages`, 'POST', {
                      text: a.question, retry_of: a.id, attachment_ids: (a.attachments || []).map(file => file.id),
                    }, crypto.randomUUID())
                    upsert(r)
                  } catch (e) { setError(String(e)) } finally { setSending(false) }
                }}><RotateCcw size={14} aria-hidden="true" />重试这一轮</button>}
                {!!a.conflicts?.length && <div className="conflicts">{a.conflicts.map((conflict, i) =>
                  <section key={i}><strong>来源存在差异</strong><p>{conflict.subject}</p>
                    <small>{(conflict.fragment_ids?.length || 0) + (conflict.source_ids?.length || 0)} 个实际来源</small>
                  </section>)}</div>}
                {a.gaps?.map((g, i) => <p className="gap" key={i}>{g}</p>)}
                {!!a.citations?.length && <div className="citations">{a.citations.map((c, i) => <button key={c.fragment_id} onClick={() => api<Record<string, unknown>>(c.url).then(setSource).catch(e => setError(String(e)))}><span>{i + 1}</span>{c.title} · {c.section}</button>)}</div>}
              </>}
            </div></div>
          </article>)}
          <div ref={bottom} />
        </div>
        <div className="composer-wrap"><form className={`composer ${dragging ? 'dragging' : ''}`} onSubmit={send}
          onDragEnter={event => { event.preventDefault(); setDragging(true) }}
          onDragOver={event => event.preventDefault()}
          onDragLeave={event => { if (!event.currentTarget.contains(event.relatedTarget as Node)) setDragging(false) }}
          onDrop={dropFiles}>
          {!!currentAttachments.length && <div className="composer-attachments">{currentAttachments.map(file =>
            <div className={`attachment-draft ${file.status.toLowerCase()}`} key={file.local_id}>
              <AttachmentTypeIcon mediaType={file.media_type} />
              <button type="button" className="attachment-name" disabled={!file.id}
                onClick={() => file.id && setAttachmentPreview(file)}>{file.name}</button>
              <span>{file.status === 'UPLOADING' ? `${file.progress}%` : attachmentStatus[file.status] || file.status}</span>
              <button type="button" className="icon-button" title="移除附件" aria-label={`移除 ${file.name}`}
                onClick={() => removeAttachment(file.local_id)}><X size={15} /></button>
              {file.status === 'UPLOADING' && <i style={{ width: `${file.progress}%` }} />}
              {(file.error || file.gaps?.[0]) && <small>{file.error || file.gaps?.[0].text}</small>}
            </div>)}</div>}
          <textarea value={text} onChange={e => setText(e.target.value)} onPaste={pasteFiles}
            placeholder="向知维提问…" aria-label="消息" maxLength={16000} rows={2} onKeyDown={e => {
            if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); void send(e) }
          }} />
          <div className="composer-bottom"><div className="composer-tools">
            <input ref={fileInput} type="file" hidden multiple accept=".png,.jpg,.jpeg,.webp,.pdf,.log,.txt,text/plain,image/png,image/jpeg,image/webp,application/pdf"
              onChange={event => { void addFiles(Array.from(event.target.files || [])); event.currentTarget.value = '' }} />
            <button type="button" className="icon-button" title="添加附件" aria-label="添加附件"
              disabled={!!active || currentAttachments.length >= 5} onClick={() => fileInput.current?.click()}><Paperclip size={18} /></button>
            <span>{active ? '当前会话正在处理，输入内容会保留' :
              currentAttachments.some(file => !['READY', 'PARTIAL', 'FAILED', 'UNSUPPORTED'].includes(file.status)) ? '正在准备附件' : ''}</span>
          </div>
            {active && <button type="button" className="text-icon-button" onClick={() => api<Answer>(`/v1/responses/${active.id}/cancel`, 'POST', {}).then(upsert).catch(e => setError(String(e)))}>
              <Square size={13} aria-hidden="true" />停止
            </button>}
            <button className="send icon-button" title="发送消息" disabled={!canSend} aria-label="发送消息">
              <ArrowUp size={18} aria-hidden="true" />
            </button></div>
        </form><p className="footnote">请结合引用原文核对建议；历史设备数据不代表当前状态。</p></div>
      </>}
    </main>
    {(rename || deleting) && <div className="modal-backdrop"><section className="conversation-modal" role="dialog" aria-modal="true" aria-label={rename ? '重命名会话' : '删除会话'}>
      {rename ? <form onSubmit={async e => {
        e.preventDefault()
        if (!metadata) return
        try {
          const c = await api<Conversation>(`/v1/conversations/${current}`, 'PATCH', { title, state_version: metadata.state_version })
          updateMetadata(c); setRename(false); await loadList()
        } catch (e) {
          setError(String(e))
          api<Conversation>(`/v1/conversations/${current}`).then(updateMetadata).catch(() => {})
        }
      }}><h2>重命名会话</h2><label>会话标题<input value={title} maxLength={100} required autoFocus onChange={e => setTitle(e.target.value)} /></label><div className="modal-actions"><button type="button" onClick={() => setRename(false)}>取消</button><button className="primary">保存标题</button></div></form> : <>
        <h2>删除这个会话？</h2><p>会话将立即无法访问，并停止正在处理的任务。历史消息和相关文件随后清理，此操作不可恢复。</p><div className="modal-actions"><button onClick={() => setDeleting(false)}>保留会话</button><button className="primary" onClick={async () => {
          try {
            await api(`/v1/conversations/${current}`, 'DELETE')
            setAnswers(rows => rows.filter(r => r.conversation_id !== current)); setCurrent(''); setDeleting(false); await loadList()
          } catch (e) { setError(String(e)) }
        }}>确认删除</button></div>
      </>}
    </section></div>}
    {source && <div className="drawer-backdrop" onClick={() => setSource(null)}><section className="source-drawer" role="dialog" aria-modal="true" aria-label="引用原文" onClick={e => e.stopPropagation()}><button className="close icon-button" title="关闭引用" onClick={() => setSource(null)} aria-label="关闭引用">
      <X size={20} aria-hidden="true" />
    </button><p className="eyebrow">来源原文</p><h2>{String(source.title)}</h2><p className="muted">{String(source.section)} · 第 {String(source.start_line)}–{String(source.end_line)} 行</p><pre>{String(source.content)}</pre><p className="footnote">{String(source.publication_status)}</p></section></div>}
    {attachmentPreview && <AttachmentDrawer attachment={attachmentPreview} close={() => setAttachmentPreview(null)} onError={setError} />}
  </div>
}

function AttachmentDrawer({ attachment, close, onError }: {
  attachment: Attachment; close: () => void; onError: (message: string) => void
}) {
  const [pageNumber, setPageNumber] = useState(1)
  const [result, setResult] = useState<AttachmentPageResult | null>(null)
  const [loading, setLoading] = useState(true)
  const count = Math.max(1, attachment.page_count || 1)
  useEffect(() => {
    let valid = true
    setLoading(true)
    api<AttachmentPageResult>(`/v1/attachments/${attachment.id}/pages/${pageNumber}`)
      .then(value => { if (valid) { setResult(value); setLoading(false) } })
      .catch(error => { if (valid) { setLoading(false); onError(String(error)) } })
    return () => { valid = false }
  }, [attachment.id, pageNumber])
  const page = result?.page
  const originalURL = `/v1/attachments/${attachment.id}/content`
  return <div className="drawer-backdrop" onClick={close}>
    <section className="source-drawer attachment-drawer" role="dialog" aria-modal="true"
      aria-label={`附件 ${attachment.name}`} onClick={event => event.stopPropagation()}>
      <button className="close icon-button" onClick={close} aria-label="关闭附件"><X size={20} /></button>
      <p className="eyebrow">本会话附件</p>
      <div className="attachment-heading"><AttachmentTypeIcon mediaType={attachment.media_type} size={22} />
        <div><h2>{attachment.name}</h2><p>{attachmentStatus[attachment.status] || attachment.status} · {(attachment.bytes / 1_000_000).toFixed(2)} MB</p></div>
        <a className="icon-button" href={originalURL} download={attachment.name} title="下载原文件" aria-label="下载原文件"><Download size={18} /></a>
      </div>
      {attachment.status === 'PARTIAL' && attachment.gaps?.map((gap, index) =>
        <p className="gap" key={`${gap.code}-${index}`}>{gap.page ? `第 ${gap.page} 页：` : ''}{gap.text}</p>)}
      {count > 1 && <div className="page-controls">
        <button className="icon-button" aria-label="上一页" disabled={pageNumber <= 1}
          onClick={() => setPageNumber(value => value - 1)}><ChevronLeft size={18} /></button>
        <span>{pageNumber} / {count}</span>
        <button className="icon-button" aria-label="下一页" disabled={pageNumber >= count}
          onClick={() => setPageNumber(value => value + 1)}><ChevronRight size={18} /></button>
      </div>}
      {loading && <p role="status" className="muted">正在读取附件来源…</p>}
      {!loading && attachment.media_type.startsWith('image/') &&
        <img className="attachment-original" src={originalURL} alt={attachment.name} />}
      {!loading && page?.preview &&
        <img className="attachment-preview" src={`/v1/attachments/${attachment.id}/assets/${page.preview.id}`}
          alt={`${attachment.name} 第 ${page.page} 页预览`} />}
      {!loading && page?.error && <p className="error">{page.error.message}</p>}
      {!loading && page?.text && <>
        <p className="source-location">{page.start_line ? `第 ${page.start_line}–${page.end_line} 行` : `第 ${page.page} 页`} · SHA-256 {page.text_sha256?.slice(0, 12)}…</p>
        <pre>{page.text}</pre>
      </>}
      {!!page?.tables?.length && <div className="attachment-tables">{page.tables.map(table =>
        <div key={table.index}><p className="source-location">表格 {table.index}</p><div className="table-scroll"><table><tbody>
          {table.rows.map((row, rowIndex) => <tr key={rowIndex}>{row.map((cell, cellIndex) =>
            <td key={cellIndex}>{cell}</td>)}</tr>)}
        </tbody></table></div></div>)}</div>}
      {!!page?.images?.length && <div className="embedded-images">{page.images.map(image =>
        <a key={image.id} href={`/v1/attachments/${attachment.id}/assets/${image.id}`} target="_blank" rel="noreferrer">
          <img src={`/v1/attachments/${attachment.id}/assets/${image.id}`} alt={image.name} />
        </a>)}</div>}
      {result?.source && <p className="hash-line">原件 SHA-256 {result.source.attachment_hash}</p>}
    </section>
  </div>
}

function Accounts() {
  const [users, setUsers] = useState<User[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const load = () => api<{ users: User[] }>('/v1/admin/users').then(r => setUsers(r.users))
  useEffect(() => { load().catch(e => setError(String(e))) }, [])
  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(''); setBusy(true)
    const form = event.currentTarget, data = new FormData(form)
    try {
      await api('/v1/admin/users', 'POST', { username: data.get('username'), password: data.get('password'), role: data.get('role') })
      form.reset(); await load()
    } catch (e) { setError(String(e)) } finally { setBusy(false) }
  }
  return <section className="admin-page"><h1>账号管理</h1><p className="muted">开通账号、停用访问或重置登录凭据。</p>
    {error && <p className="error" role="alert">{error}</p>}
    <form className="create-account" onSubmit={create}>
      <label>用户名<input name="username" required minLength={3} maxLength={64} autoComplete="off" /></label>
      <label>初始密码<input name="password" type="password" required minLength={12} maxLength={72} autoComplete="new-password" /></label>
      <label>角色<select name="role"><option value="USER">普通用户</option><option value="ADMIN">管理员</option></select></label>
      <button className="primary" disabled={busy}>创建账号</button>
    </form>
    <div className="users">{users.map(u => <div className="user-row" key={u.id}><div><strong>{u.username}</strong><span className="muted">{u.role === 'ADMIN' ? '管理员' : '普通用户'} · {u.active ? '已启用' : '已停用'}</span></div><button onClick={async () => {
      try { await api(`/v1/admin/users/${u.id}`, 'PATCH', { active: !u.active }); await load() }
      catch (e) { setError(String(e)) }
    }}>{u.active ? '停用' : '启用'}</button><details><summary>重置密码</summary><form onSubmit={async e => {
      e.preventDefault()
      const form = e.currentTarget, password = new FormData(form).get('password')
      try { await api(`/v1/admin/users/${u.id}/password-reset`, 'POST', { password }); form.reset(); setError('密码已重置，该账号原有登录状态已撤销。') }
      catch (e) { setError(String(e)) }
    }}><input aria-label={`${u.username}的新密码`} name="password" type="password" required minLength={12} maxLength={72} autoComplete="new-password" /><button>保存</button></form></details></div>)}</div>
  </section>
}

createRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>)
