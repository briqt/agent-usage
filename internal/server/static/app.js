const $ = id => document.getElementById(id);
const fmt = n => n >= 1e6 ? (n/1e6).toFixed(1)+'M' : n >= 1e3 ? (n/1e3).toFixed(1)+'K' : String(n);
const fmtCost = n => n >= 1 ? '$'+n.toFixed(2) : '$'+n.toFixed(4);

const dark = {bg:'transparent',text:'#e1e4ed',muted:'#8b8fa3',grid:'#2a2d3a'};
const colors = ['#6366f1','#3b82f6','#22c55e','#f59e0b','#ec4899','#8b5cf6','#14b8a6','#f43f5e'];

function baseOpt(){return{backgroundColor:dark.bg,textStyle:{color:dark.text},grid:{left:60,right:20,top:30,bottom:30},tooltip:{trigger:'axis',backgroundColor:'#1a1d27',borderColor:'#2a2d3a',textStyle:{color:'#e1e4ed'}}}}

function params(){
  const f=$('from').value, t=$('to').value;
  let q=[];
  if(f) q.push('from='+f);
  if(t) q.push('to='+t);
  return q.length ? '?'+q.join('&') : '';
}

async function api(path){const r=await fetch('/api/'+path+params());return r.json()}

let charts={};
function initCharts(){
  charts.pie=echarts.init($('chart-pie'));
  charts.cost=echarts.init($('chart-cost'));
  charts.tokens=echarts.init($('chart-tokens'));
  charts.sessions=echarts.init($('chart-sessions'));
  window.addEventListener('resize',()=>Object.values(charts).forEach(c=>c.resize()));
}

async function refresh(){
  const [stats,costModel,costTime,tokensTime,sessions]=await Promise.all([
    api('stats'),api('cost-by-model'),api('cost-over-time'),api('tokens-over-time'),api('sessions')
  ]);

  $('s-cost').textContent=fmtCost(stats.total_cost||0);
  $('s-tokens').textContent=fmt(stats.total_tokens||0);
  $('s-sessions').textContent=stats.total_sessions||0;
  $('s-prompts').textContent=stats.total_prompts||0;

  // Pie chart
  const pieData=(costModel||[]).filter(d=>d.cost>0).map(d=>({name:d.model,value:+d.cost.toFixed(4)}));
  charts.pie.setOption({...baseOpt(),tooltip:{trigger:'item',formatter:p=>p.name+'<br/>'+fmtCost(p.value)},
    series:[{type:'pie',radius:['40%','70%'],itemStyle:{borderRadius:4,borderColor:dark.bg,borderWidth:2},
    label:{color:dark.text,fontSize:11},data:pieData}],color:colors});

  // Cost over time - stacked by model
  const costDates=[...new Set((costTime||[]).map(d=>d.date))].sort();
  const costModels=[...new Set((costTime||[]).map(d=>d.model))];
  const costSeries=costModels.map((m,i)=>{
    const map=Object.fromEntries((costTime||[]).filter(d=>d.model===m).map(d=>[d.date,d.value]));
    return{name:m,type:'line',stack:'cost',areaStyle:{opacity:0.3},data:costDates.map(d=>+(map[d]||0).toFixed(4)),smooth:true};
  });
  charts.cost.setOption({...baseOpt(),xAxis:{type:'category',data:costDates,axisLine:{lineStyle:{color:dark.grid}},axisLabel:{color:dark.muted}},
    yAxis:{type:'value',axisLabel:{color:dark.muted,formatter:v=>fmtCost(v)},splitLine:{lineStyle:{color:dark.grid}}},
    legend:{textStyle:{color:dark.muted},bottom:0},series:costSeries,color:colors});

  // Token usage
  const tokenDates=(tokensTime||[]).map(d=>d.date);
  charts.tokens.setOption({...baseOpt(),xAxis:{type:'category',data:tokenDates,axisLine:{lineStyle:{color:dark.grid}},axisLabel:{color:dark.muted}},
    yAxis:{type:'value',axisLabel:{color:dark.muted,formatter:v=>fmt(v)},splitLine:{lineStyle:{color:dark.grid}}},
    legend:{textStyle:{color:dark.muted},bottom:0},
    series:[
      {name:'Input',type:'bar',stack:'t',data:(tokensTime||[]).map(d=>d.input_tokens),color:'#3b82f6'},
      {name:'Output',type:'bar',stack:'t',data:(tokensTime||[]).map(d=>d.output_tokens),color:'#22c55e'},
      {name:'Cache Read',type:'bar',stack:'t',data:(tokensTime||[]).map(d=>d.cache_read),color:'#f59e0b'},
      {name:'Cache Create',type:'bar',stack:'t',data:(tokensTime||[]).map(d=>d.cache_create),color:'#ec4899'}
    ]});

  // Sessions per day (derived from sessions list)
  const sessByDate={};
  (sessions||[]).forEach(s=>{if(s.start_time){const d=s.start_time.slice(0,10);sessByDate[d]=(sessByDate[d]||0)+1}});
  const sesDates=Object.keys(sessByDate).sort();
  charts.sessions.setOption({...baseOpt(),xAxis:{type:'category',data:sesDates,axisLine:{lineStyle:{color:dark.grid}},axisLabel:{color:dark.muted}},
    yAxis:{type:'value',axisLabel:{color:dark.muted},splitLine:{lineStyle:{color:dark.grid}}},
    series:[{type:'bar',data:sesDates.map(d=>sessByDate[d]),color:'#6366f1',barMaxWidth:30}]});

  // Session table
  const tb=$('session-table');
  tb.innerHTML=(sessions||[]).map(s=>`<tr>
    <td><span class="badge ${s.source}">${s.source}</span></td>
    <td title="${s.cwd}">${s.project||s.cwd||'-'}</td>
    <td>${s.git_branch||'-'}</td>
    <td>${s.start_time?s.start_time.replace('T',' ').slice(0,16):'-'}</td>
    <td>${s.prompts}</td><td>${fmt(s.tokens||0)}</td><td>${fmtCost(s.total_cost||0)}</td></tr>`).join('');
}

// Init
const now=new Date(), monthAgo=new Date(now);
monthAgo.setMonth(monthAgo.getMonth()-1);
$('from').value=monthAgo.toISOString().slice(0,10);
$('to').value=now.toISOString().slice(0,10);
initCharts();
refresh();
