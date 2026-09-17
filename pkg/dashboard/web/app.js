'use strict';
/* WebSearch Control Center - dependency-free frontend. */

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

const REFRESH_MS = 60000;
const TZ = { hour12: false, timeZone: 'Asia/Shanghai' };
const PAGES = { overview: '运行总览', sources: '搜索源', usage: '调用记录', settings: '设置' };
const HEALTH = { healthy: '正常', degraded: '不稳定', down: '异常', unknown: '未知' };
const SYSTEM = { healthy: '运行正常', degraded: '部分降级', down: '部分异常', waiting: '等待真实调用' };
const DOT = { healthy: 'healthy', degraded: 'degraded', down: 'down', unknown: 'unknown' };
const DISPLAY = {
  smartsearch: '智能搜索', academicsearch: '学术检索', cleanfetch: '网页抓取', pdf_parser: 'PDF 解析',
  anysearch: 'AnySearch', baidu: '百度千帆', tavily: 'Tavily', exa: 'Exa', doubao: '豆包搜索',
  baidu_web: '百度网页', bing: 'Bing', google: 'Google', duckduckgo: 'DuckDuckGo',
  arxiv: 'arXiv', crossref: 'Crossref', openalex: 'OpenAlex', pubmed: 'PubMed', europepmc: 'Europe PMC',
  dblp: 'DBLP', doaj: 'DOAJ', semantic_scholar: 'Semantic Scholar', google_scholar: 'Google Scholar',
  webfetch: '网页抓取器', jina: 'Jina Reader', pdf_pipeline: 'PDF 流水线',
};
const SECRETS = {
  BAIDU_SK: '百度', TAVILY_SK: 'Tavily', EXA_API_KEY: 'Exa', ANYSEARCH_API_KEY: 'AnySearch',
  DOUBAO_SEARCH_API_KEY: '豆包', JINA_API_KEY: 'Jina Reader', MINERU_TOKEN: 'MinerU',
};

const state = {
  overview: null, overviewError: null, overviewStale: false,
  quotas: null, quotaError: null,
  settings: null, settingsError: null, settingsDirty: false, settingsLoaded: false,
  events: { rows: null, error: null, serverFiltered: true },
  providerEvents: { rows: null, error: null },
  filters: { kind: 'provider', status: '', tool: '', provider: '', limit: 50 },
  lastLoad: 0,
};

/* ---------- helpers ---------- */

const ESCAPES = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };
function esc(value) {
  if (value === null || value === undefined) return '';
  return String(value).replace(/[&<>"']/g, ch => ESCAPES[ch]);
}
function num(value) {
  return typeof value === 'number' && isFinite(value) ? value : null;
}
function fmtInt(value) {
  const n = num(value);
  return n === null ? '—' : new Intl.NumberFormat('zh-CN').format(n);
}
function fmtMS(value) {
  const n = num(value);
  return n === null ? '—' : n + ' ms';
}
function fmtPct(rate, sample) {
  const n = num(rate);
  return n === null || !sample ? '—' : Math.round(n * 100) + '%';
}
function fmtAt(value) {
  if (!value) return '—';
  const d = new Date(value);
  return isNaN(d.getTime()) ? '—' : d.toLocaleString('zh-CN', TZ);
}
function fmtClock() {
  return new Date().toLocaleTimeString('zh-CN', TZ);
}
function displayName(id) {
  return DISPLAY[id] || id || '';
}
function dot(status) {
  return `<i class="dot ${DOT[status] || 'unknown'}"></i>`;
}
function setText(sel, text) {
  const el = $(sel);
  if (el) el.textContent = text;
}
function setNote(el, text, cls) {
  if (!el) return;
  el.textContent = text || '';
  el.className = 'panel-note' + (cls ? ' ' + cls : '');
}
function emptyRow(cols, text) {
  return `<tr><td class="empty" colspan="${cols}">${esc(text)}</td></tr>`;
}
function hasFilter() {
  const f = state.filters;
  return Boolean(f.kind || f.status || f.tool || f.provider);
}
function matchesFilters(e) {
  const f = state.filters;
  if (f.status === 'success' && !e.success) return false;
  if (f.status === 'failure' && e.success) return false;
  if (f.tool && (e.kind !== 'tool' || e.tool !== f.tool)) return false;
  if (f.provider && (e.kind !== 'provider' || e.provider !== f.provider)) return false;
  if (f.tool && f.provider) return false;
  if (!f.tool && !f.provider && f.kind && e.kind !== f.kind) return false;
  return true;
}
function activationOf(s) {
  if (s.configured === false) return { key: 'not_configured', label: '未配置' };
  if (s.active === true) return { key: 'active', label: '当前启用' };
  if (s.active === false) return { key: 'inactive', label: '已配置未使用' };
  return { key: 'unknown', label: '未知' };
}
function healthOf(s) {
  if (s.active === false || s.configured === false) return { dot: 'unknown', label: '—' };
  if (!s.last_seen_at) return { dot: 'unknown', label: '尚无调用' };
  return { dot: DOT[s.status] || 'unknown', label: HEALTH[s.status] || '未知' };
}

/* ---------- transport ---------- */

async function fetchJSON(path, options = {}) {
  const headers = Object.assign({ Accept: 'application/json' }, options.headers || {});
  const res = await fetch(path, Object.assign({ cache: 'no-store' }, options, { headers }));
  let body = null;
  try { body = await res.json(); } catch (e) { body = null; }
  if (!res.ok) {
    const msg = body && body.error ? body.error : 'HTTP ' + res.status;
    throw new Error(msg);
  }
  if (body === null) throw new Error('响应不是有效的 JSON');
  return body;
}

/* ---------- loaders (each panel fails on its own) ---------- */

async function loadOverview() {
  try {
    const data = await fetchJSON('/__admin/api/overview');
    state.overview = data;
    // The source catalog is the schema this page is built around; older
    // payloads are shown as-is but flagged so they are never read as current.
    state.overviewStale = !(data && data.system && Array.isArray(data.sources));
    state.overviewError = null;
  } catch (e) {
    state.overviewError = e.message;
  }
  renderSystem();
  renderToolKpis();
  renderMCPTools();
  renderWebSources();
  renderSourcePages();
  renderUsageSourceOptions();
  renderConfig();
}

async function loadQuotas() {
  try {
    const data = await fetchJSON('/__admin/api/quotas');
    state.quotas = Array.isArray(data) ? data : [];
    state.quotaError = null;
  } catch (e) {
    state.quotaError = e.message;
  }
  renderTavily();
  renderWebSources();
  renderSourcePages();
}

async function loadSettings() {
  try {
    const data = await fetchJSON('/__admin/api/settings');
    if (!data || !data.values) throw new Error('设置响应缺少 values 字段');
    state.settings = data;
    state.settingsError = null;
  } catch (e) {
    state.settingsError = e.message;
  }
  renderConfig();
  if (!state.settingsLoaded || !state.settingsDirty) renderSettingsForm();
}

async function loadEvents() {
  const requestId = (state.events.requestId || 0) + 1;
  state.events.requestId = requestId;
  const f = state.filters;
  const params = new URLSearchParams();
  params.set('limit', String(f.limit));
  if (f.kind) params.set('kind', f.kind);
  if (f.status) params.set('status', f.status);
  const tool = f.tool || '';
  const provider = f.provider || '';
  if (tool && provider) {
    params.set('kind', 'none');
    params.set('source', '');
  } else if (tool) {
    params.set('kind', 'tool');
    params.set('source', tool);
  } else if (provider) {
    params.set('kind', 'provider');
    params.set('source', provider);
  }
  try {
    const rows = await fetchJSON('/__admin/api/events?' + params.toString());
    if (state.events.requestId !== requestId) return;
    state.events.rows = Array.isArray(rows) ? rows : [];
    state.events.error = null;
    state.events.serverFiltered = !hasFilter() || state.events.rows.every(matchesFilters);
  } catch (e) {
    if (state.events.requestId !== requestId) return;
    state.events.error = e.message;
  }
  renderUsage();
  renderUsageSourceOptions();
}

async function loadProviderEvents() {
  try {
    const rows = await fetchJSON('/__admin/api/events?limit=20&kind=provider');
    state.providerEvents.rows = (Array.isArray(rows) ? rows : []).filter(e => e && e.kind === 'provider');
    state.providerEvents.error = null;
  } catch (e) {
    state.providerEvents.error = e.message;
  }
  renderOverviewEvents();
}

async function loadAll() {
  await Promise.allSettled([loadOverview(), loadQuotas(), loadSettings(), loadEvents(), loadProviderEvents()]);
  state.lastLoad = Date.now();
  renderUpdated();
}

function anyFailure() {
  return Boolean(state.overviewError || state.quotaError || state.settingsError || state.events.error || state.providerEvents.error || state.overviewStale);
}

function renderUpdated() {
  const el = $('#updated');
  if (!el) return;
  el.textContent = '更新于 ' + fmtClock() + (anyFailure() ? ' · 部分数据可能已过期' : '');
  el.classList.toggle('warn', anyFailure());
}

/* ---------- overview: system + tool KPIs ---------- */

function renderSystem() {
  const o = state.overview;
  const sys = o && o.system ? o.system : null;
  const label = $('#system-status');
  const d = $('#system-dot');
  const note = $('#system-note');
  if (!o) {
    label.textContent = '—';
    d.className = 'dot unknown';
    setNote(note, state.overviewError ? '读取失败：' + state.overviewError : '正在读取…', state.overviewError ? 'bad' : '');
  } else if (!sys) {
    label.textContent = '未知';
    d.className = 'dot unknown';
    setNote(note, '数据可能已过期：概览接口未返回系统摘要（system）', 'warn');
  } else {
    label.textContent = SYSTEM[sys.status] || '未知';
    d.className = 'dot ' + (sys.status === 'waiting' ? 'unknown' : (DOT[sys.status] || 'unknown'));
    const summary = `已启用 ${fmtInt(sys.enabled)} 个 · 已观察 ${fmtInt(sys.observed)} 个 · 异常 ${fmtInt(sys.down)} 个`;
    if (state.overviewError) setNote(note, summary + ' · 数据可能已过期：' + state.overviewError, 'warn');
    else if (sys.status === 'waiting') setNote(note, summary + ' · 尚未观察到近期真实调用', '');
    else setNote(note, summary, '');
  }
  const has = o && sys;
  setText('#system-enabled', has ? fmtInt(sys.enabled) : '—');
  setText('#system-observed', has ? fmtInt(sys.observed) : '—');
  setText('#system-degraded', has ? fmtInt(sys.degraded) : '—');
  setText('#system-down', has ? fmtInt(sys.down) : '—');
}

function renderToolKpis() {
  const raw = state.overview && state.overview.today;
  const today = raw && raw.day ? raw : null;
  const avg = today && num(today.requests) > 0 ? Math.round(today.duration_ms / today.requests) : null;
  const cards = [
    ['今日调用总数', today ? fmtInt(today.requests) : '—', today ? `缓存命中 ${fmtInt(today.cache_hits)} 次` : '暂无工具层调用'],
    ['今日成功次数', today ? fmtInt(today.successes) : '—', '仅统计 MCP 工具层事件'],
    ['今日失败次数', today ? fmtInt(today.failures) : '—', '仅统计 MCP 工具层事件'],
    ['今日平均响应时间', avg === null ? '—' : fmtMS(avg), today && num(today.requests) > 0 ? '工具层总耗时 ÷ 调用数' : '无调用时不计算'],
  ];
  $('#tool-kpis').innerHTML = cards.map(([label, value, detail]) =>
    `<article class="kpi"><header><b>${esc(label)}</b></header><strong>${esc(value)}</strong><small class="dim">${esc(detail)}</small></article>`
  ).join('');
}

// renderMCPTools 展示公开 MCP 工具的固定清单（不依赖是否已经产生调用），
// 并用遥测健康数据补充“已观测 / 尚无调用”的观测状态。
function renderMCPTools() {
  const list = $('#mcp-tools-list');
  if (!list) return;
  const o = state.overview;
  const configured = o && Array.isArray(o.configured_tools) ? o.configured_tools : [];
  const note = $('#mcp-tools-note');
  const help = $('#mcp-tools-help');
  if (!configured.length) {
    list.innerHTML = `<p class="empty">${esc(o ? '概览接口未返回工具清单' : '正在读取…')}</p>`;
    if (note) note.textContent = '—';
    if (help) setNote(help, state.overviewError ? '读取失败：' + state.overviewError : '', state.overviewError ? 'bad' : '');
    return;
  }
  const observed = new Map();
  (Array.isArray(o.tools) ? o.tools : []).forEach(h => h && observed.set(h.name, h));
  const enabledCount = configured.filter(t => t.enabled).length;
  if (note) note.textContent = enabledCount + ' / ' + configured.length + ' 已启用';
  if (help) setNote(help, '开关状态来自运行配置；观测状态来自真实调用，尚无调用不会被当成异常', '');
  list.innerHTML = configured.map(tool => {
    const health = observed.get(tool.name);
    let stateLabel;
    let dotClass;
    if (!tool.enabled) {
      stateLabel = '配置未启用';
      dotClass = 'unknown';
    } else if (health && health.last_seen_at) {
      stateLabel = HEALTH[health.status] || '未知';
      dotClass = DOT[health.status] || 'unknown';
    } else {
      stateLabel = '可调用 · 尚无观测';
      dotClass = 'unknown';
    }
    const stats = health && health.today
      ? `<small>今日 ${fmtInt(health.today.requests)} 次 · 成功 ${fmtInt(health.today.successes)} · 失败 ${fmtInt(health.today.failures)}</small>`
      : '<small>尚未产生工具层事件</small>';
    return `<div class="tool-row">${dot(dotClass)}` +
      `<div class="tool-main"><div class="tool-line"><b>${esc(tool.label || displayName(tool.name))}</b><span class="tool-state">${esc(stateLabel)}</span></div>` +
      `<small class="mono">${esc(tool.name)}</small>${stats}</div></div>`;
  }).join('');
}

/* ---------- sources ---------- */

function webSources() {
  const o = state.overview;
  if (!o) return null;
  if (Array.isArray(o.sources)) return o.sources;
  // Legacy payload: show observed health rows without inventing a catalog.
  return (Array.isArray(o.providers) ? o.providers : []).map(h => Object.assign({}, h, {
    id: h.name, name: displayName(h.name), group: 'web', configured: null, active: null,
  }));
}

function academicSources() {
  const o = state.overview;
  if (!o) return null;
  return Array.isArray(o.academic_sources) ? o.academic_sources : null;
}

function tavilyItem() {
  if (!Array.isArray(state.quotas)) return null;
  return state.quotas.find(q => q && q.provider === 'tavily') || null;
}

function quotaCell(s) {
  if (!s || s.quota_queryable !== true) return '<span class="dim">不可查询</span>';
  const q = tavilyItem();
  if (!q) return state.quotaError ? '<span class="dim">查询失败</span>' : '<span class="dim">尚未返回</span>';
  if (q.status === 'available') return `${esc(fmtInt(q.remaining))} / ${esc(fmtInt(q.limit))} ${esc(q.unit || '')}`.trim();
  if (q.status === 'not_configured') return '未配置';
  if (q.status === 'query_failed') return '<span class="bad-text">查询失败</span>';
  return '未知';
}

function healthStrip(s) {
  const outcomes = Array.isArray(s.recent_outcomes) ? s.recent_outcomes.slice(-20) : [];
  const cells = Array(Math.max(0, 20 - outcomes.length)).fill(null).concat(outcomes);
  const title = outcomes.length ? `最近 ${outcomes.length} 次真实调用` : '暂无真实调用';
  return `<span class="health-strip" title="${esc(title)}" aria-label="${esc(title)}">` +
    cells.map(ok => `<i class="${ok === null ? 'empty' : ok ? 'ok' : 'fail'}"></i>`).join('') + '</span>';
}

function sourceRowHtml(s, detailed) {
  const act = activationOf(s);
  const health = healthOf(s);
  const today = s.today && s.today.day ? s.today : null;
  const name = s.name || displayName(s.id);
  const id = s.id || '';
  const cells = [
    `<td class="src-name"><b>${esc(name)}</b>${id ? `<small class="dim">${esc(id)}</small>` : ''}</td>`,
    `<td><span class="tag ${esc(act.key)}">${esc(act.label)}</span></td>`,
    `<td class="nowrap">${s.active === false || s.configured === false ? '<span class="dim">—</span>' : dot(health.dot) + ' ' + esc(health.label)}</td>`,
    `<td>${healthStrip(s)}</td>`,
    `<td class="num">${today ? fmtInt(today.requests) : '<span class="dim">—</span>'}</td>`,
  ];
  if (detailed) {
    cells.push(`<td class="num">${today ? fmtInt(today.successes) + ' / ' + fmtInt(today.failures) : '<span class="dim">—</span>'}</td>`);
  }
  const successRate = today && num(today.requests) > 0 ? today.successes / today.requests : null;
  cells.push(`<td class="num">${successRate === null ? '<span class="dim">—</span>' : esc(Math.round(successRate * 100) + '%')}</td>`);
  cells.push(`<td class="num">${today && num(today.requests) > 0 ? esc(fmtMS(Math.round(today.duration_ms / today.requests))) : '<span class="dim">—</span>'}</td>`);
  if (detailed) {
    cells.push(`<td class="nowrap">${esc(fmtAt(s.last_seen_at))}</td>`);
  }
  cells.push(`<td>${quotaCell(s)}</td>`);
  if (detailed) {
    cells.push(`<td class="wrap-any">${s.last_error ? '<span class="bad-text">' + esc(s.last_error) + '</span>' : '<span class="dim">—</span>'}</td>`);
  }
  return `<tr class="${s.active === true && s.status === 'down' ? 'row-down' : ''}">${cells.join('')}</tr>`;
}

function countsLabel(list) {
  let active = 0, inactive = 0, unconfigured = 0;
  list.forEach(s => {
    if (s.configured === false) unconfigured++;
    else if (s.active === true) active++;
    else if (s.active === false) inactive++;
    else unconfigured++;
  });
  return `启用 ${active} · 已配置未用 ${inactive} · 未配置 ${unconfigured}`;
}

function renderWebSources() {
  const body = $('#web-sources-body');
  const note = $('#web-sources-note');
  const list = webSources();
  if (list === null) {
    body.innerHTML = emptyRow(6, state.overviewError ? '读取失败' : '正在读取…');
    setNote(note, state.overviewError ? '读取失败：' + state.overviewError : '', state.overviewError ? 'bad' : '');
    return;
  }
  const active = list.filter(s => s.active === true);
  const shown = active;
  body.innerHTML = shown.length ? shown.map(s => sourceRowHtml(s, false)).join('') : emptyRow(8, '当前模式没有已启用的 Web 来源');
  if (state.overviewError) setNote(note, '数据可能已过期：' + state.overviewError, 'warn');
  else if (!Array.isArray(state.overview.sources)) setNote(note, '数据可能已过期：接口未返回来源清单（sources），按已观测 Provider 展示', 'warn');
  else setNote(note, '', '');
}

function renderSourcePages() {
  const webList = webSources();
  const acadList = academicSources();

  const webBody = $('#sources-web-body');
  if (webList === null) {
    webBody.innerHTML = emptyRow(11, state.overviewError ? '读取失败' : '正在读取…');
    setNote($('#sources-web-note'), state.overviewError ? '读取失败：' + state.overviewError : '', state.overviewError ? 'bad' : '');
    setText('#sources-web-counts', '—');
  } else {
    webBody.innerHTML = webList.length ? webList.map(s => sourceRowHtml(s, true)).join('') : emptyRow(11, '暂无 Web 来源');
    setText('#sources-web-counts', countsLabel(webList));
    if (state.overviewError) setNote($('#sources-web-note'), '数据可能已过期：' + state.overviewError, 'warn');
    else if (!Array.isArray(state.overview.sources)) setNote($('#sources-web-note'), '数据可能已过期：接口未返回来源清单（sources）', 'warn');
    else setNote($('#sources-web-note'), '', '');
  }

  const acadBody = $('#sources-academic-body');
  if (acadList === null) {
    acadBody.innerHTML = emptyRow(11, state.overviewError ? '读取失败' : '正在读取…');
    setText('#sources-academic-counts', '—');
    setNote($('#sources-academic-note'), state.overviewError
      ? '读取失败：' + state.overviewError
      : '数据可能已过期：概览接口未返回学术来源清单（academic_sources）', state.overviewError ? 'bad' : 'warn');
  } else {
    acadBody.innerHTML = acadList.length ? acadList.map(s => sourceRowHtml(s, true)).join('') : emptyRow(11, '暂无学术来源');
    setText('#sources-academic-counts', countsLabel(acadList));
    setNote($('#sources-academic-note'), state.overviewError ? '数据可能已过期：' + state.overviewError : '', state.overviewError ? 'warn' : '');
  }
}

/* ---------- Tavily quota card ---------- */

function renderTavily() {
  const value = $('#tavily-value');
  const note = $('#tavily-note');
  const dotEl = $('#tavily-dot');
  const details = $('#tavily-details');
  const q = tavilyItem();
  const kv = (pairs) => pairs.filter(p => p[1]).map(p => `<div><dt>${esc(p[0])}</dt><dd>${esc(p[1])}</dd></div>`).join('');
  if (state.quotaError && !q) {
    value.textContent = '查询失败';
    dotEl.className = 'dot down';
    setNote(note, '数据可能已过期：' + state.quotaError, 'warn');
    details.innerHTML = '';
    return;
  }
  if (!q) {
    value.textContent = '—';
    dotEl.className = 'dot unknown';
    setNote(note, state.quotas ? '官方额度接口未返回 Tavily 条目' : '正在读取…', state.quotas ? 'bad' : '');
    details.innerHTML = '';
    return;
  }
  details.innerHTML = kv([
    ['已用', num(q.used) === null ? '' : fmtInt(q.used) + ' ' + (q.unit || '')],
    ['上限', num(q.limit) === null ? '' : fmtInt(q.limit) + ' ' + (q.unit || '')],
    ['套餐', q.plan || ''],
    ['更新时间', q.updated_at ? fmtAt(q.updated_at) : ''],
    ['官方接口', q.source || ''],
  ]);
  switch (q.status) {
    case 'available':
      value.textContent = `${fmtInt(q.remaining)} / ${fmtInt(q.limit)} ${q.unit || ''}`.trim();
      dotEl.className = 'dot healthy';
      setNote(note, state.quotaError ? '数据可能已过期：' + state.quotaError : '来自官方额度接口，非本地估算', state.quotaError ? 'warn' : '');
      break;
    case 'not_configured':
      value.textContent = '未配置';
      dotEl.className = 'dot unknown';
      setNote(note, '未配置 Tavily Key，无法查询官方额度', '');
      break;
    case 'query_failed':
      value.textContent = '查询失败';
      dotEl.className = 'dot down';
      setNote(note, q.error || '官方额度接口未返回可用数据', 'bad');
      break;
    default:
      value.textContent = '未知';
      dotEl.className = 'dot unknown';
      setNote(note, q.error || '官方额度接口返回了未识别的状态', 'warn');
  }
}

/* ---------- effective config summary ---------- */

function renderConfig() {
  const list = $('#config-list');
  const note = $('#config-note');
  const secrets = $('#config-secrets');
  const s = state.settings;
  if (!s) {
    list.innerHTML = '';
    secrets.textContent = '';
    setNote(note, state.settingsError ? '读取失败：' + state.settingsError : '正在读取…', state.settingsError ? 'bad' : '');
    return;
  }
  const v = s.values || {};
  const yn = value => value === true ? '启用' : value === false ? '关闭' : '未知';
  const rows = [
    ['搜索模式', typeof v.mode === 'string' ? v.mode : ''],
    ['网络区域', v.network === 'china' ? '中国' : v.network === 'international' ? '国际' : ''],
    ['上游超时', num(v.upstream_timeout_sec) === null ? '' : String(v.upstream_timeout_sec) + ' 秒'],
    ['缓存', yn(v['cache.enabled'])],
    ['已启用来源', state.overview && state.overview.system ? fmtInt(state.overview.system.enabled) + ' 个' : ''],
  ];
  list.innerHTML = rows.filter(r => r[1]).map(r => `<div><dt>${esc(r[0])}</dt><dd>${esc(r[1])}</dd></div>`).join('');
  secrets.textContent = '';
  if (state.settingsError) setNote(note, '数据可能已过期：' + state.settingsError, 'warn');
  else setNote(note, '重启后生效的当前配置', '');
}

/* ---------- events ---------- */

function eventCells(e, cols) {
  const out = [`<td class="nowrap">${esc(fmtAt(e.occurred_at))}</td>`];
  if (cols !== 'overview') {
    out.push(`<td>${esc(e.kind === 'provider' ? '来源' : '工具')}</td>`);
  }
  const detail = e.detail ? `<small class="source-detail">${esc(e.detail)}</small>` : '';
  const toolLabel = e.kind === 'tool'
    ? `<span class="source-name">${esc(displayName(e.tool))}</span>`
    : '<span class="dim">—</span>';
  const providerLabel = e.provider
    ? `<span class="source-name">${esc(displayName(e.provider))}</span>${detail}`
    : '<span class="dim">—</span>';
  out.push(
    `<td>${toolLabel}</td>`,
    `<td>${providerLabel}</td>`,
    `<td class="${e.success ? 'ok-text' : 'bad-text'}">${e.success ? '成功' : '失败'}</td>`,
    `<td class="num">${esc(fmtMS(e.duration_ms))}</td>`,
    `<td class="num">${fmtInt(e.result_count)}</td>`,
  );
  if (cols === 'overview') {
    out.push(`<td>${esc([e.query_topic, e.query_language].filter(Boolean).join(' / ') || '—')}</td>`);
    out.push(`<td class="wrap-any">${esc(e.query_keywords || '—')}</td>`);
    out.push(`<td class="wrap-any">${e.error_summary ? '<span class="bad-text">' + esc(e.error_summary) + '</span>' : '<span class="dim">—</span>'}</td>`);
  } else {
    out.push(`<td>${e.cache_hit ? '是' : '否'}</td>`);
    out.push(`<td>${esc([e.query_topic, e.query_language].filter(Boolean).join(' / ') || '—')}</td>`);
    out.push(`<td class="wrap-any">${esc(e.query_keywords || '—')}</td>`);
    out.push(`<td class="mono">${esc(e.query_hash || '—')}</td>`);
    out.push(`<td class="wrap-any">${e.error_summary ? '<span class="bad-text">' + esc(e.error_summary) + '</span>' : '<span class="dim">—</span>'}</td>`);
  }
  return out;
}

function renderOverviewEvents() {
  const body = $('#overview-events-body');
  const note = $('#overview-events-note');
  const rows = state.providerEvents.rows;
  if (rows === null) {
    body.innerHTML = emptyRow(9, state.providerEvents.error ? '读取失败' : '正在读取…');
    setNote(note, state.providerEvents.error ? '读取失败：' + state.providerEvents.error : '', state.providerEvents.error ? 'bad' : '');
    return;
  }
  const top = rows.slice(0, 6);
  body.innerHTML = top.length
    ? top.map(e => `<tr>${eventCells(e, 'overview').join('')}</tr>`).join('')
    : emptyRow(9, '暂无 Provider 事件（明细保留 30 天）');
  if (state.providerEvents.error) setNote(note, '数据可能已过期：' + state.providerEvents.error, 'warn');
  else if (!top.length) setNote(note, '来源层调用后才会出现真实事件，不做模拟', '');
  else setNote(note, '', '');
}

function renderUsage() {
  const body = $('#usage-body');
  const note = $('#events-note');
  const rows = state.events.rows;
  const exportBtn = $('#export-csv');
  if (rows === null) {
    body.innerHTML = emptyRow(12, state.events.error ? '读取失败' : '正在读取…');
    exportBtn.disabled = true;
    setNote(note, state.events.error ? '读取失败：' + state.events.error : '', state.events.error ? 'bad' : '');
    return;
  }
  const filtered = rows.filter(matchesFilters);
  body.innerHTML = filtered.length
    ? filtered.map(e => `<tr>${eventCells(e, 'usage').join('')}</tr>`).join('')
    : emptyRow(12, rows.length ? '当前筛选条件下没有匹配记录' : '暂无调用记录');
  exportBtn.disabled = filtered.length === 0;
  const parts = [];
  if (state.events.error) parts.push(['数据可能已过期：' + state.events.error, 'warn']);
  if (hasFilter() && !state.events.serverFiltered) parts.push(['后端未应用筛选参数，当前按已获取的 ' + rows.length + ' 条在本地筛选', 'warn']);
  parts.push(['匹配 ' + filtered.length + ' 条 / 已获取 ' + rows.length + ' 条 · 查询全文不保存', '']);
  note.innerHTML = parts.filter(p => p[0]).map(p => `<span${p[1] ? ' class="' + p[1] + '"' : ''}>${esc(p[0])}</span>`).join(' · ');
}

/* ---------- CSV export (UTF-8 BOM) ---------- */

function csvCell(value) {
  const s = value === null || value === undefined ? '' : String(value);
  return '"' + s.replace(/"/g, '""').replace(/[\r\n]+/g, ' ') + '"';
}
function stamp() {
  const d = new Date();
  const p = n => String(n).padStart(2, '0');
  return d.getFullYear() + p(d.getMonth() + 1) + p(d.getDate()) + '-' + p(d.getHours()) + p(d.getMinutes()) + p(d.getSeconds());
}
function exportCSV() {
  const rows = (state.events.rows || []).filter(matchesFilters);
  if (!rows.length) {
    toast('当前没有可导出的记录', true);
    return;
  }
  const head = ['时间', '层级', '工具', '来源', '解析来源', '状态', '耗时(ms)', '结果数', '缓存命中', '主题', '语言', '关键词', '查询哈希', '查询字符数', '错误摘要'];
  const lines = [head.map(csvCell).join(',')];
  rows.forEach(e => {
    lines.push([
      fmtAt(e.occurred_at),
      e.kind === 'provider' ? '来源' : '工具',
      e.kind === 'tool' ? displayName(e.tool) : '',
      e.provider ? displayName(e.provider) : '',
      e.detail || '',
      e.success ? '成功' : '失败',
      num(e.duration_ms) === null ? '' : e.duration_ms,
      num(e.result_count) === null ? '' : e.result_count,
      e.cache_hit ? '是' : '否',
      e.query_topic || '',
      e.query_language || '',
      e.query_keywords || '',
      e.query_hash || '',
      num(e.query_chars) === null ? '' : e.query_chars,
      e.error_summary || '',
    ].map(csvCell).join(','));
  });
  const blob = new Blob(['\uFEFF' + lines.join('\r\n')], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'websearch-events-' + stamp() + '.csv';
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 2000);
  toast('已导出 ' + rows.length + ' 条记录（UTF-8 BOM）');
}

/* ---------- usage filters ---------- */

function knownFilterEntries() {
  const tools = new Map();
  const providers = new Map();
  const o = state.overview;
  const addTool = (id, label) => { if (id) tools.set(id, label || displayName(id)); };
  const addProvider = (id, label) => { if (id) providers.set(id, label || displayName(id)); };
  if (o) {
    (Array.isArray(o.configured_tools) ? o.configured_tools : []).forEach(t => t && addTool(t.name, t.label));
    (Array.isArray(o.providers) ? o.providers : []).forEach(h => h && addProvider(h.name));
    (Array.isArray(o.tools) ? o.tools : []).forEach(h => h && addTool(h.name));
    (Array.isArray(o.sources) ? o.sources : []).forEach(s => s && addProvider(s.id, s.name));
    (Array.isArray(o.academic_sources) ? o.academic_sources : []).forEach(s => s && addProvider(s.id, s.name));
  }
  [state.events.rows, state.providerEvents.rows].forEach(list => {
    (list || []).forEach(e => {
      if (!e) return;
      if (e.kind === 'provider') addProvider(e.provider);
      else addTool(e.tool);
    });
  });
  const sort = map => Array.from(map.entries()).sort((a, b) => String(a[1]).localeCompare(String(b[1]), 'zh-CN'));
  return { tools: sort(tools), providers: sort(providers) };
}

function renderFilterSelect(selector, entries, current) {
  const sel = $(selector);
  if (!sel) return;
  const known = new Set(entries.map(e => e[0]));
  const options = ['<option value="">全部</option>'];
  if (current && !known.has(current)) {
    options.push(`<option value="${esc(current)}">${esc(displayName(current))}</option>`);
  }
  entries.forEach(([id, label]) => {
    const text = label && label !== id ? label + '（' + id + '）' : id;
    options.push(`<option value="${esc(id)}">${esc(text)}</option>`);
  });
  sel.innerHTML = options.join('');
  sel.value = current;
}

function renderUsageSourceOptions() {
  const entries = knownFilterEntries();
  renderFilterSelect('#filter-tool', entries.tools, state.filters.tool);
  renderFilterSelect('#filter-provider', entries.providers, state.filters.provider);
}

/* ---------- settings & advanced actions ---------- */

function renderSettingsForm() {
  const s = state.settings;
  if (!s || !s.values) return;
  $$('[data-key]').forEach(el => {
    const value = s.values[el.dataset.key];
    if (el.type === 'checkbox') el.checked = value === true;
    else el.value = value === null || value === undefined ? '' : value;
  });
  $('#settings-dirty').classList.add('hidden');
  state.settingsDirty = false;

  const flags = s.secrets || {};
  $('#secret-list').innerHTML = Object.keys(flags).map(key => {
    const on = flags[key] === true;
    return `<div class="secret-row">${dot(on ? 'healthy' : 'unknown')}` +
      `<div><b>${esc(SECRETS[key] || key)}</b><small>${on ? '已配置' : '未配置'}</small></div>` +
      `<button type="button" data-secret="${esc(key)}">${on ? '替换' : '设置'}</button></div>`;
  }).join('');
  $$('[data-secret]').forEach(b => { b.onclick = () => editSecret(b.dataset.secret); });
  state.settingsLoaded = true;
}

function changes() {
  if (!state.settings) return {};
  const out = {};
  $$('[data-key]').forEach(el => {
    const value = el.type === 'checkbox' ? el.checked : el.type === 'number' ? Number(el.value) : el.value;
    const before = state.settings.values[el.dataset.key];
    if (JSON.stringify(value) !== JSON.stringify(before)) out[el.dataset.key] = value;
  });
  return out;
}

function markDirty() {
  const dirty = Object.keys(changes()).length > 0;
  state.settingsDirty = dirty;
  $('#settings-dirty').classList.toggle('hidden', !dirty);
}

async function confirmDialog(title, copy, diff) {
  const d = $('#confirm-dialog');
  $('#dialog-title').textContent = title;
  $('#dialog-copy').textContent = copy;
  const pre = $('#dialog-diff');
  pre.textContent = diff || '';
  pre.style.display = diff ? 'block' : 'none';
  d.showModal();
  return new Promise(resolve => {
    d.addEventListener('close', () => resolve(d.returnValue === 'confirm'), { once: true });
  });
}

function toast(message, bad) {
  const el = $('#toast');
  el.textContent = message;
  el.style.background = bad ? '#9e342f' : '#17231c';
  el.classList.add('show');
  setTimeout(() => el.classList.remove('show'), 2600);
}

async function postJSON(path, payload) {
  return fetchJSON(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify(payload),
  });
}

async function saveSettings() {
  const diff = changes();
  if (!Object.keys(diff).length) {
    toast('没有需要保存的更改');
    return;
  }
  try {
    await postJSON('/__admin/api/settings', { changes: diff, confirm: false });
    if (!(await confirmDialog('保存设置', '将备份当前 YAML，再写入以下更改。重启后生效。', JSON.stringify(diff, null, 2)))) return;
    const res = await postJSON('/__admin/api/settings', { changes: diff, confirm: true });
    toast(res.backup ? '已保存，备份：' + res.backup : '已保存');
    await loadSettings();
  } catch (e) {
    toast(e.message, true);
  }
}

async function editSecret(name) {
  const label = SECRETS[name] || name;
  const value = prompt('输入 ' + label + ' 的新值。内容不会回显，也不会写入 YAML；留空表示删除。');
  if (value === null) return;
  if (!(await confirmDialog('更新私密凭据', '新值将保存到 Docker 持久卷的私密覆盖文件，重启后生效。'))) return;
  try {
    await postJSON('/__admin/api/secrets', { name: name, value: value, confirm: true });
    toast(label + ' 已保存，需要重启服务');
    await loadSettings();
  } catch (e) {
    toast(e.message, true);
  }
}

/* ---------- navigation & wiring ---------- */

function nav(page, push) {
  if (!PAGES[page]) page = 'overview';
  $$('.page').forEach(el => el.classList.toggle('active', el.id === 'page-' + page));
  $$('.nav').forEach(el => el.classList.toggle('active', el.dataset.page === page));
  $('#page-title').textContent = PAGES[page];
  if (push !== false) history.replaceState(null, '', '#' + page);
}

function wire() {
  $$('.nav').forEach(btn => btn.addEventListener('click', () => nav(btn.dataset.page)));
  $$('[data-jump]').forEach(link => link.addEventListener('click', e => { e.preventDefault(); nav(link.dataset.jump); }));
  $('#refresh').addEventListener('click', () => loadAll());
  $('#reload-events').addEventListener('click', () => loadEvents());
  $('#export-csv').addEventListener('click', exportCSV);
  $('#save-settings').addEventListener('click', saveSettings);
  $('#restart-service').addEventListener('click', async () => {
    if (!(await confirmDialog('重启 WebSearch', '当前请求会短暂中断，Docker 将自动重新启动服务。'))) return;
    try {
      await postJSON('/__admin/api/restart', { confirm: true });
      toast('正在重启…');
      setTimeout(() => location.reload(), 3500);
    } catch (e) {
      toast(e.message, true);
    }
  });
  $('#clear-cache').addEventListener('click', async () => {
    if (!(await confirmDialog('清空搜索缓存', '此操作不可撤销，但不会删除监控历史。'))) return;
    try {
      const res = await postJSON('/__admin/api/cache/clear', { confirm: true });
      toast('已清理 ' + fmtInt(res.cleared) + ' 条缓存');
    } catch (e) {
      toast(e.message, true);
    }
  });
  $$('[data-key]').forEach(el => el.addEventListener('change', markDirty));
  const applyFilter = () => {
    state.filters.kind = '';
    state.filters.status = $('#filter-status').value;
    state.filters.tool = $('#filter-tool').value;
    state.filters.provider = $('#filter-provider').value;
    state.filters.limit = Number($('#filter-limit').value) || 20;
    loadEvents();
  };
  ['#filter-kind', '#filter-status', '#filter-tool', '#filter-provider', '#filter-limit'].forEach(sel => {
    $(sel).addEventListener('change', applyFilter);
  });
  $('#filter-limit').value = String(state.filters.limit);
  $('#filter-kind').value = '';
  $('#filter-status').value = state.filters.status;
  document.addEventListener('visibilitychange', () => {
    if (!document.hidden && Date.now() - state.lastLoad > REFRESH_MS) loadAll();
  });
}

wire();
nav(location.hash.slice(1) || 'overview', false);
loadAll();
setInterval(() => { if (!document.hidden) loadAll(); }, REFRESH_MS);
