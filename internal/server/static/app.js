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
    autoOn:'Auto',autoOff:'Auto',refreshIn:'refresh in',
    input:'Input',output:'Output',cacheRead:'Cache Read',cacheCreate:'Cache Create',
    gran_1m:'1min',gran_30m:'30min',gran_1h:'1h',gran_6h:'6h',gran_12h:'12h',gran_1d:'1d',gran_1w:'1w',gran_1M:'1mo',
  },
  zh: {
    title:'AI 使用仪表盘',to:'至',totalCost:'总费用',totalTokens:'总 Token',
    sessions:'会话数',prompts:'提示数',costByModel:'模型费用分布',costOverTime:'费用趋势',
    tokenUsage:'Token 使用趋势',dailySessions:'每日会话数',source:'来源',project:'项目',
    branch:'分支',time:'时间',tokens:'Token',cost:'费用',
    today:'今天',thisWeek:'本周',thisMonth:'本月',thisYear:'今年',
    last3d:'近3天',last7d:'近7天',last30d:'近30天',custom:'自定义',
    light:'浅色',dark:'深色',system:'跟随系统',
    autoOn:'自动',autoOff:'自动',refreshIn:'刷新倒计时',
    input:'输入',output:'输出',cacheRead:'缓存读取',cacheCreate:'缓存创建',
    gran_1m:'1分钟',gran_30m:'30分钟',gran_1h:'1小时',gran_6h:'6小时',gran_12h:'12小时',gran_1d:'1天',gran_1w:'1周',gran_1M:'1月',
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
let countdownTimer = null;
let countdown = 0;
let charts = {};

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
  const tb = $('session-table');
  tb.innerHTML = (sessions || []).map(s => `<tr>
    <td><span class="badge ${s.source}">${s.source}</span></td>
    <td title="${s.cwd}">${s.project || s.cwd || '-'}</td>
    <td>${s.git_branch || '-'}</td>
    <td>${s.start_time ? s.start_time.replace('T', ' ').slice(0, 16) : '-'}</td>
    <td>${s.prompts}</td><td>${fmt(s.tokens || 0)}</td><td>${fmtCost(s.total_cost || 0)}</td></tr>`).join('');
}

// ── Auto Refresh ──
function startAutoRefresh() {
  stopAutoRefresh();
  if (!state.autoRefresh) return;
  countdown = state.refreshInterval;
  updateCountdown();
  countdownTimer = setInterval(() => {
    countdown--;
    if (countdown <= 0) {
      refresh();
      countdown = state.refreshInterval;
    }
    updateCountdown();
  }, 1000);
}

function stopAutoRefresh() {
  if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null; }
  $('refresh-indicator').textContent = '';
}

function updateCountdown() {
  if (!state.autoRefresh) { $('refresh-indicator').textContent = ''; return; }
  const m = Math.floor(countdown / 60), s = countdown % 60;
  $('refresh-indicator').textContent = t('refreshIn') + ' ' + (m > 0 ? m + 'm' : '') + s + 's';
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
refresh();
startAutoRefresh();
