// ── i18n ──
const I18N = {
  en: {
    title:'AI Usage Dashboard',to:'to',totalCost:'Total Cost',totalTokens:'Total Tokens',
    sessions:'Sessions',prompts:'Prompts',costByModel:'Cost by Model',costOverTime:'Cost Over Time',
    tokenUsage:'Token Usage Over Time',dailySessions:'Daily Sessions',source:'Source',project:'Project',
    branch:'Branch',time:'Time',tokens:'Tokens',cost:'Cost',
    today:'Today',thisWeek:'This Week',thisMonth:'This Month',thisYear:'This Year',
    last3d:'Last 3 Days',last7d:'Last 7 Days',last30d:'Last 30 Days',custom:'Custom',
    light:'Light',dark:'Dark',system:'System',
    autoOn:'Auto',autoOff:'Auto',
    input:'Input',output:'Output',cacheRead:'Cache Read',cacheCreate:'Cache Create',
    gran_1m:'1min',gran_30m:'30min',gran_1h:'1h',gran_6h:'6h',gran_12h:'12h',gran_1d:'1d',gran_1w:'1w',gran_1M:'1mo',
    model:'Model',calls:'Calls',allSources:'All Sources',claudeCode:'Claude Code',codex:'Codex',
    filterProject:'Filter by project...',justNow:'just now',mAgo:'m ago',hAgo:'h ago',dAgo:'d ago',
    noSessions:'No sessions found',
  },
  zh: {
    title:'AI 使用仪表盘',to:'至',totalCost:'总费用',totalTokens:'总 Token',
    sessions:'会话数',prompts:'提示数',costByModel:'模型费用分布',costOverTime:'费用趋势',
    tokenUsage:'Token 使用趋势',dailySessions:'每日会话数',source:'来源',project:'项目',
    branch:'分支',time:'时间',tokens:'Token',cost:'费用',
    today:'今天',thisWeek:'本周',thisMonth:'本月',thisYear:'今年',
    last3d:'近3天',last7d:'近7天',last30d:'近30天',custom:'自定义',
    light:'浅色',dark:'深色',system:'跟随系统',
    autoOn:'自动',autoOff:'自动',
    input:'输入',output:'输出',cacheRead:'缓存读取',cacheCreate:'缓存创建',
    gran_1m:'1分钟',gran_30m:'30分钟',gran_1h:'1小时',gran_6h:'6小时',gran_12h:'12小时',gran_1d:'1天',gran_1w:'1周',gran_1M:'1月',
    model:'模型',calls:'调用次数',allSources:'全部来源',claudeCode:'Claude Code',codex:'Codex',
    filterProject:'按项目筛选...',justNow:'刚刚',mAgo:'分钟前',hAgo:'小时前',dAgo:'天前',
    noSessions:'暂无会话',
  }
};

// ── State ──
const $ = id => document.getElementById(id);
const fmt = n => n >= 1e6 ? (n/1e6).toFixed(1)+'M' : n >= 1e3 ? (n/1e3).toFixed(1)+'K' : String(n);
const fmtCost = n => n >= 1 ? '$'+n.toFixed(2) : '$'+n.toFixed(4);
const colors = ['#6366f1','#3b82f6','#22c55e','#f59e0b','#ec4899','#8b5cf6','#14b8a6','#f43f5e'];

const PRESETS = ['today','thisWeek','thisMonth','thisYear','last3d','last7d','last30d','custom'];
const GRANULARITIES = ['1m','30m','1h','6h','12h','1d','1w','1M'];
const REFRESH_INTERVALS = [30,60,300,1800,3600]; // seconds

let state = {
  lang: localStorage.getItem('au-lang') || (navigator.language.includes('zh') ? 'zh' : 'en'),
  theme: localStorage.getItem('au-theme') || 'system',
  preset: localStorage.getItem('au-preset') || 'today',
  granularity: localStorage.getItem('au-granularity') || '1h',
  autoRefresh: localStorage.getItem('au-autoRefresh') !== 'false',
  refreshInterval: parseInt(localStorage.getItem('au-refreshInterval')) || 300,
  customFrom: localStorage.getItem('au-customFrom') || '',
  customTo: localStorage.getItem('au-customTo') || '',
};

let autoTimer = null;
let charts = {};

// Session table state
let allSessions = [];
let sessionSort = { key: 'start_time', dir: 'desc' };
let sessionPage = 1;
const PAGE_SIZE = 20;
let expandedSessions = new Set();

// ── Helpers ──
function t(key) { return (I18N[state.lang] || I18N.en)[key] || key; }

function persist(key, val) {
  state[key] = val;
  localStorage.setItem('au-' + key, val);
}

function applyTheme() {
  const th = state.theme === 'system'
    ? (window.matchMedia('(prefers-color-scheme:dark)').matches ? 'dark' : 'light')
    : state.theme;
  document.documentElement.setAttribute('data-theme', th);
  // re-render charts with new theme colors
  Object.values(charts).forEach(c => c && c.resize());
}

function applyI18n() {
  document.querySelectorAll('[data-i18n]').forEach(el => {
    el.textContent = t(el.dataset.i18n);
  });
}

function getThemeColors() {
  const cs = getComputedStyle(document.documentElement);
  return {
    bg: cs.getPropertyValue('--chart-bg').trim() || 'transparent',
    text: cs.getPropertyValue('--chart-text').trim() || '#e1e4ed',
    muted: cs.getPropertyValue('--chart-muted').trim() || '#8b8fa3',
    grid: cs.getPropertyValue('--chart-grid').trim() || '#2a2d3a',
    tooltipBg: cs.getPropertyValue('--tooltip-bg').trim() || '#1a1d27',
    tooltipBorder: cs.getPropertyValue('--tooltip-border').trim() || '#2a2d3a',
  };
}

function baseOpt() {
  const tc = getThemeColors();
  return {
    backgroundColor: tc.bg,
    textStyle: { color: tc.text },
    grid: { left: 60, right: 20, top: 30, bottom: 30 },
    tooltip: { trigger: 'axis', backgroundColor: tc.tooltipBg, borderColor: tc.tooltipBorder, textStyle: { color: tc.text } }
  };
}

// ── Time range ──
function getTimeRange() {
  const now = new Date();
  const todayStr = now.toISOString().slice(0, 10);
  switch (state.preset) {
    case 'today': return { from: todayStr, to: todayStr };
    case 'thisWeek': {
      const d = new Date(now); d.setDate(d.getDate() - ((d.getDay() + 6) % 7));
      return { from: d.toISOString().slice(0, 10), to: todayStr };
    }
    case 'thisMonth': return { from: todayStr.slice(0, 8) + '01', to: todayStr };
    case 'thisYear': return { from: todayStr.slice(0, 5) + '01-01', to: todayStr };
    case 'last3d': { const d = new Date(now); d.setDate(d.getDate() - 2); return { from: d.toISOString().slice(0, 10), to: todayStr }; }
    case 'last7d': { const d = new Date(now); d.setDate(d.getDate() - 6); return { from: d.toISOString().slice(0, 10), to: todayStr }; }
    case 'last30d': { const d = new Date(now); d.setDate(d.getDate() - 29); return { from: d.toISOString().slice(0, 10), to: todayStr }; }
    case 'custom': return { from: state.customFrom || todayStr, to: state.customTo || todayStr };
    default: return { from: todayStr, to: todayStr };
  }
}

function params() {
  const r = getTimeRange();
  let q = ['from=' + r.from, 'to=' + r.to];
  if (state.granularity) q.push('granularity=' + state.granularity);
  return '?' + q.join('&');
}

async function api(path) { const r = await fetch('/api/' + path + params()); return r.json(); }

// ── Charts ──
function initCharts() {
  charts.pie = echarts.init($('chart-pie'));
  charts.cost = echarts.init($('chart-cost'));
  charts.tokens = echarts.init($('chart-tokens'));
  charts.sessions = echarts.init($('chart-sessions'));
  window.addEventListener('resize', () => Object.values(charts).forEach(c => c && c.resize()));
}

async function refresh() {
  const [stats, costModel, costTime, tokensTime, sessions] = await Promise.all([
    api('stats'), api('cost-by-model'), api('cost-over-time'), api('tokens-over-time'), api('sessions')
  ]);

  $('s-cost').textContent = fmtCost(stats.total_cost || 0);
  $('s-tokens').textContent = fmt(stats.total_tokens || 0);
  $('s-sessions').textContent = stats.total_sessions || 0;
  $('s-prompts').textContent = stats.total_prompts || 0;

  const tc = getThemeColors();

  // Pie
  const pieData = (costModel || []).filter(d => d.cost > 0).map(d => ({ name: d.model, value: +d.cost.toFixed(4) }));
  charts.pie.setOption({
    ...baseOpt(), tooltip: { trigger: 'item', formatter: p => p.name + '<br/>' + fmtCost(p.value), backgroundColor: tc.tooltipBg, borderColor: tc.tooltipBorder, textStyle: { color: tc.text } },
    series: [{ type: 'pie', radius: ['40%', '70%'], itemStyle: { borderRadius: 4, borderColor: tc.bg, borderWidth: 2 },
      label: { color: tc.text, fontSize: 11 }, data: pieData }], color: colors
  }, true);

  // Cost over time
  const costDates = [...new Set((costTime || []).map(d => d.date))].sort();
  const costModels = [...new Set((costTime || []).map(d => d.model))];
  const costSeries = costModels.map(m => {
    const map = Object.fromEntries((costTime || []).filter(d => d.model === m).map(d => [d.date, d.value]));
    return { name: m, type: 'line', stack: 'cost', areaStyle: { opacity: 0.3 }, data: costDates.map(d => +(map[d] || 0).toFixed(4)), smooth: true };
  });
  charts.cost.setOption({
    ...baseOpt(),
    xAxis: { type: 'category', data: costDates, axisLine: { lineStyle: { color: tc.grid } }, axisLabel: { color: tc.muted } },
    yAxis: { type: 'value', axisLabel: { color: tc.muted, formatter: v => fmtCost(v) }, splitLine: { lineStyle: { color: tc.grid } } },
    legend: { textStyle: { color: tc.muted }, bottom: 0 }, series: costSeries, color: colors
  }, true);

  // Tokens
  const tokenDates = (tokensTime || []).map(d => d.date);
  charts.tokens.setOption({
    ...baseOpt(),
    xAxis: { type: 'category', data: tokenDates, axisLine: { lineStyle: { color: tc.grid } }, axisLabel: { color: tc.muted } },
    yAxis: { type: 'value', axisLabel: { color: tc.muted, formatter: v => fmt(v) }, splitLine: { lineStyle: { color: tc.grid } } },
    legend: { textStyle: { color: tc.muted }, bottom: 0 },
    series: [
      { name: t('input'), type: 'bar', stack: 't', data: (tokensTime || []).map(d => d.input_tokens), color: '#3b82f6' },
      { name: t('output'), type: 'bar', stack: 't', data: (tokensTime || []).map(d => d.output_tokens), color: '#22c55e' },
      { name: t('cacheRead'), type: 'bar', stack: 't', data: (tokensTime || []).map(d => d.cache_read), color: '#f59e0b' },
      { name: t('cacheCreate'), type: 'bar', stack: 't', data: (tokensTime || []).map(d => d.cache_create), color: '#ec4899' }
    ]
  }, true);

  // Sessions per day
  const sessByDate = {};
  (sessions || []).forEach(s => { if (s.start_time) { const d = s.start_time.slice(0, 10); sessByDate[d] = (sessByDate[d] || 0) + 1; } });
  const sesDates = Object.keys(sessByDate).sort();
  charts.sessions.setOption({
    ...baseOpt(),
    xAxis: { type: 'category', data: sesDates, axisLine: { lineStyle: { color: tc.grid } }, axisLabel: { color: tc.muted } },
    yAxis: { type: 'value', axisLabel: { color: tc.muted }, splitLine: { lineStyle: { color: tc.grid } } },
    series: [{ type: 'bar', data: sesDates.map(d => sessByDate[d]), color: '#6366f1', barMaxWidth: 30 }]
  }, true);

  // Session table
  allSessions = sessions || [];
  renderSessionTable();
}

// ── Relative time ──
function relTime(ts) {
  if (!ts) return '-';
  const d = new Date(ts.replace(' ', 'T').replace(' +0000 UTC', 'Z'));
  if (isNaN(d)) return ts.replace('T', ' ').slice(0, 16);
  const diff = Math.floor((Date.now() - d.getTime()) / 1000);
  if (diff < 60) return t('justNow');
  if (diff < 3600) return Math.floor(diff / 60) + t('mAgo');
  if (diff < 86400) return Math.floor(diff / 3600) + t('hAgo');
  if (diff < 604800) return Math.floor(diff / 86400) + t('dAgo');
  return d.toLocaleDateString();
}

function relTimeTitle(ts) {
  if (!ts) return '';
  return ts.replace('T', ' ').slice(0, 19);
}

// ── Session table ──
function getFilteredSessions() {
  const srcFilter = $('filter-source').value;
  const projFilter = ($('filter-project').value || '').toLowerCase();
  return allSessions.filter(s => {
    if (srcFilter && s.source !== srcFilter) return false;
    if (projFilter) {
      const proj = (s.project || s.cwd || '').toLowerCase();
      if (!proj.includes(projFilter)) return false;
    }
    return true;
  });
}

function getSortedSessions(filtered) {
  const k = sessionSort.key;
  const dir = sessionSort.dir === 'asc' ? 1 : -1;
  return [...filtered].sort((a, b) => {
    let va = a[k], vb = b[k];
    if (k === 'start_time') {
      va = va || ''; vb = vb || '';
      return va < vb ? -dir : va > vb ? dir : 0;
    }
    if (typeof va === 'number' || typeof vb === 'number') {
      return ((va || 0) - (vb || 0)) * dir;
    }
    va = (va || '').toLowerCase(); vb = (vb || '').toLowerCase();
    return va < vb ? -dir : va > vb ? dir : 0;
  });
}

function renderSessionTable() {
  const filtered = getFilteredSessions();
  const sorted = getSortedSessions(filtered);
  const totalPages = Math.max(1, Math.ceil(sorted.length / PAGE_SIZE));
  if (sessionPage > totalPages) sessionPage = totalPages;
  const start = (sessionPage - 1) * PAGE_SIZE;
  const page = sorted.slice(start, start + PAGE_SIZE);

  // Update sort arrows
  document.querySelectorAll('.sortable').forEach(th => {
    const k = th.dataset.sort;
    th.classList.remove('asc', 'desc');
    let arrow = th.querySelector('.sort-arrow');
    if (!arrow) { arrow = document.createElement('span'); arrow.className = 'sort-arrow'; th.appendChild(arrow); }
    if (k === sessionSort.key) {
      th.classList.add(sessionSort.dir);
      arrow.textContent = sessionSort.dir === 'asc' ? '\u25B2' : '\u25BC';
    } else {
      arrow.textContent = '\u25B4';
    }
  });

  const tb = $('session-table');
  if (page.length === 0) {
    tb.innerHTML = `<tr><td colspan="8" style="text-align:center;color:var(--muted);padding:24px">${t('noSessions')}</td></tr>`;
  } else {
    tb.innerHTML = page.map(s => {
      const expanded = expandedSessions.has(s.session_id);
      return `<tr class="session-row" data-sid="${s.session_id}">
        <td><span class="badge ${s.source}">${s.source}</span></td>
        <td title="${s.cwd || ''}">${s.project || s.cwd || '-'}</td>
        <td>${s.git_branch || '-'}</td>
        <td title="${relTimeTitle(s.start_time)}">${relTime(s.start_time)}</td>
        <td>${s.prompts}</td><td>${fmt(s.tokens || 0)}</td><td>${fmtCost(s.total_cost || 0)}</td>
        <td><button class="expand-btn${expanded ? ' open' : ''}" data-sid="${s.session_id}">\u25B6</button></td>
      </tr>${expanded ? `<tr class="detail-row" data-sid="${s.session_id}"><td colspan="8"><div class="detail-content" id="detail-${s.session_id}">Loading...</div></td></tr>` : ''}`;
    }).join('');
  }

  // Pagination
  const pag = $('pagination');
  if (totalPages <= 1) {
    pag.innerHTML = filtered.length > 0 ? `<span class="page-info">${filtered.length} sessions</span>` : '';
  } else {
    let html = `<span class="page-info">${start + 1}-${Math.min(start + PAGE_SIZE, sorted.length)} of ${sorted.length}</span>`;
    if (sessionPage > 1) html += `<button class="page-btn" data-page="${sessionPage - 1}">\u2190</button>`;
    const maxBtns = 7;
    let pStart = Math.max(1, sessionPage - 3);
    let pEnd = Math.min(totalPages, pStart + maxBtns - 1);
    if (pEnd - pStart < maxBtns - 1) pStart = Math.max(1, pEnd - maxBtns + 1);
    for (let i = pStart; i <= pEnd; i++) {
      html += `<button class="page-btn${i === sessionPage ? ' active' : ''}" data-page="${i}">${i}</button>`;
    }
    if (sessionPage < totalPages) html += `<button class="page-btn" data-page="${sessionPage + 1}">\u2192</button>`;
    pag.innerHTML = html;
  }

  // Bind events
  tb.querySelectorAll('.expand-btn').forEach(btn => {
    btn.onclick = (e) => {
      e.stopPropagation();
      toggleSessionDetail(btn.dataset.sid);
    };
  });
  pag.querySelectorAll('.page-btn').forEach(btn => {
    btn.onclick = () => { sessionPage = parseInt(btn.dataset.page); renderSessionTable(); };
  });
}

async function toggleSessionDetail(sid) {
  if (expandedSessions.has(sid)) {
    expandedSessions.delete(sid);
    renderSessionTable();
    return;
  }
  expandedSessions.add(sid);
  renderSessionTable();
  // Fetch detail
  try {
    const res = await fetch('/api/session-detail?session_id=' + encodeURIComponent(sid));
    const data = await res.json();
    const el = document.getElementById('detail-' + sid);
    if (!el) return;
    if (!data || data.length === 0) {
      el.textContent = 'No usage data';
      return;
    }
    el.innerHTML = `<table class="detail-table"><thead><tr>
      <th>${t('model')}</th><th>${t('calls')}</th><th>${t('input')}</th><th>${t('output')}</th>
      <th>${t('cacheRead')}</th><th>${t('cacheCreate')}</th><th>${t('cost')}</th>
    </tr></thead><tbody>${data.map(d => `<tr>
      <td>${d.model}</td><td>${d.calls}</td><td>${fmt(d.input_tokens)}</td><td>${fmt(d.output_tokens)}</td>
      <td>${fmt(d.cache_read)}</td><td>${fmt(d.cache_create)}</td><td>${fmtCost(d.cost_usd)}</td>
    </tr>`).join('')}</tbody></table>`;
  } catch (e) {
    const el = document.getElementById('detail-' + sid);
    if (el) el.textContent = 'Error loading details';
  }
}

function initSessionControls() {
  // Sort headers
  document.querySelectorAll('.sortable').forEach(th => {
    th.onclick = () => {
      const k = th.dataset.sort;
      if (sessionSort.key === k) {
        sessionSort.dir = sessionSort.dir === 'asc' ? 'desc' : 'asc';
      } else {
        sessionSort.key = k;
        sessionSort.dir = (k === 'start_time' || k === 'total_cost' || k === 'tokens' || k === 'prompts') ? 'desc' : 'asc';
      }
      renderSessionTable();
    };
  });
  // Filters
  $('filter-source').onchange = () => { sessionPage = 1; renderSessionTable(); };
  $('filter-project').oninput = () => { sessionPage = 1; renderSessionTable(); };
}

// ── Auto Refresh ──
function startAutoRefresh() {
  stopAutoRefresh();
  if (!state.autoRefresh) return;
  autoTimer = setInterval(() => refresh(), state.refreshInterval * 1000);
}

function stopAutoRefresh() {
  if (autoTimer) { clearInterval(autoTimer); autoTimer = null; }
}

// ── UI Setup ──
function buildControls() {
  // Theme selector
  const selTheme = $('sel-theme');
  selTheme.innerHTML = ['system', 'light', 'dark'].map(v =>
    `<option value="${v}" ${state.theme === v ? 'selected' : ''}>${t(v)}</option>`
  ).join('');
  selTheme.onchange = () => { persist('theme', selTheme.value); applyTheme(); };

  // Language selector
  const selLang = $('sel-lang');
  selLang.innerHTML = `<option value="en" ${state.lang === 'en' ? 'selected' : ''}>English</option>
    <option value="zh" ${state.lang === 'zh' ? 'selected' : ''}>中文</option>`;
  selLang.onchange = () => { persist('lang', selLang.value); applyI18n(); buildControls(); refresh(); };

  // Presets
  const bar = $('preset-bar');
  bar.innerHTML = PRESETS.map(p =>
    `<button class="preset-btn ${state.preset === p ? 'active' : ''}" data-preset="${p}">${t(p)}</button>`
  ).join('');
  bar.querySelectorAll('.preset-btn').forEach(btn => {
    btn.onclick = () => {
      persist('preset', btn.dataset.preset);
      buildControls();
      refresh();
      startAutoRefresh();
    };
  });

  // Custom date inputs visibility
  const fromEl = $('from'), toEl = $('to');
  const customVisible = state.preset === 'custom';
  fromEl.parentElement.style.display = customVisible ? 'flex' : 'none';
  if (customVisible) {
    fromEl.value = state.customFrom || new Date().toISOString().slice(0, 10);
    toEl.value = state.customTo || new Date().toISOString().slice(0, 10);
  }
  fromEl.onchange = () => { persist('customFrom', fromEl.value); refresh(); startAutoRefresh(); };
  toEl.onchange = () => { persist('customTo', toEl.value); refresh(); startAutoRefresh(); };

  // Granularity
  const selGran = $('sel-granularity');
  selGran.innerHTML = GRANULARITIES.map(g =>
    `<option value="${g}" ${state.granularity === g ? 'selected' : ''}>${t('gran_' + g)}</option>`
  ).join('');
  selGran.onchange = () => { persist('granularity', selGran.value); refresh(); };

  // Refresh button
  $('btn-refresh').onclick = () => { refresh(); startAutoRefresh(); };

  // Auto refresh toggle
  const btnAuto = $('btn-auto-refresh');
  btnAuto.textContent = state.autoRefresh ? t('autoOn') + ' ✓' : t('autoOff');
  btnAuto.className = 'ctrl-btn' + (state.autoRefresh ? ' active' : '');
  btnAuto.onclick = () => {
    persist('autoRefresh', !state.autoRefresh);
    if (state.autoRefresh) startAutoRefresh(); else stopAutoRefresh();
    buildControls();
  };

  // Refresh interval
  const selInt = $('sel-refresh-interval');
  const intLabels = { 30: '30s', 60: '1m', 300: '5m', 1800: '30m', 3600: '1h' };
  selInt.innerHTML = REFRESH_INTERVALS.map(v =>
    `<option value="${v}" ${state.refreshInterval === v ? 'selected' : ''}>${intLabels[v]}</option>`
  ).join('');
  selInt.onchange = () => { persist('refreshInterval', parseInt(selInt.value)); startAutoRefresh(); };
}

// ── Init ──
applyTheme();
window.matchMedia('(prefers-color-scheme:dark)').addEventListener('change', () => {
  if (state.theme === 'system') applyTheme();
});
initCharts();
buildControls();
applyI18n();
initSessionControls();
refresh();
startAutoRefresh();
