package internal

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const adminHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>zai2api Admin</title>
  <style>
    *{box-sizing:border-box}
    body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;margin:0;background:#f5f6f8;color:#172033}
    button,input,textarea{font:inherit}
    button{border:1px solid #ccd3df;background:#fff;border-radius:6px;padding:7px 10px;cursor:pointer;color:#172033}
    button.primary{background:#1f6feb;color:#fff;border-color:#1f6feb}
    textarea{width:100%;min-height:82px;resize:vertical}
    input,textarea{border:1px solid #ccd3df;border-radius:6px;padding:8px;background:#fff;color:#172033}
    .shell{display:grid;grid-template-columns:220px 1fr;min-height:100vh}
    .nav{background:#101828;color:#fff;padding:18px 14px}
    .brand{font-weight:700;margin:4px 8px 20px}
    .nav button{width:100%;text-align:left;margin:4px 0;background:transparent;color:#d9e2f1;border-color:transparent}
    .nav button.active{background:#24324a;color:#fff;border-color:#33435d}
    .content{padding:20px;min-width:0}
    .view{display:none}
    .view.active{display:block}
    section{background:#fff;border:1px solid #e0e5ee;border-radius:8px;padding:16px;margin-bottom:16px}
    h2{font-size:16px;margin:0 0 12px}
    .grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:12px}
    .metric{background:#f8fafc;border:1px solid #ebeff5;border-radius:6px;padding:12px;color:#586174}
    .metric b{display:block;font-size:22px;margin-top:4px;color:#172033}
    .row{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
    table{width:100%;border-collapse:collapse;font-size:13px}
    th,td{border-bottom:1px solid #edf1f6;padding:8px;text-align:left;vertical-align:top}
    th{color:#586174;font-weight:600;background:#fbfcfe}
    .token{word-break:break-all;max-width:560px}
    .reason{word-break:break-word;max-width:360px;color:#667085}
    .ok{color:#057a55;font-weight:700}
    .bad{color:#b42318;font-weight:700}
    .muted{color:#667085}
    .testing{opacity:.65;pointer-events:none}
    pre{background:#101828;color:#e5e7eb;padding:12px;border-radius:6px;overflow:auto;white-space:pre-wrap}
    .history-layout{display:grid;grid-template-columns:minmax(300px,390px) 1fr;gap:16px;height:calc(100vh - 40px)}
    .history-list,.history-detail{background:#fff;border:1px solid #e0e5ee;border-radius:8px;min-height:0;overflow:auto}
    .history-head{position:sticky;top:0;background:#fff;border-bottom:1px solid #edf1f6;padding:12px;z-index:1}
    .history-pager{position:sticky;bottom:0;background:#fff;border-top:1px solid #edf1f6;padding:10px 12px;display:flex;align-items:center;justify-content:space-between;gap:8px}
    .history-pager button:disabled{opacity:.45;cursor:not-allowed}
    .history-page-info{font-size:12px;color:#667085}
    .history-item{display:block;width:100%;border:0;border-bottom:1px solid #edf1f6;border-radius:0;background:#fff;text-align:left;padding:12px}
    .history-item.active{background:#eef4ff}
    .history-title{display:flex;justify-content:space-between;gap:8px;font-weight:700}
    .history-meta{margin-top:6px;color:#667085;font-size:12px;line-height:1.6}
    .badge{display:inline-flex;align-items:center;border-radius:999px;padding:2px 8px;font-size:12px;font-weight:700}
    .badge.success{background:#dcfae6;color:#067647}
    .badge.failed{background:#fee4e2;color:#b42318}
    .badge.running{background:#fef0c7;color:#b54708}
    .badge.healthy{background:#d1fae5;color:#047857}
    .badge.disabled{background:#e5e7eb;color:#4b5563}
    .badge.error{background:#fee2e2;color:#b91c1c}
    .proxy-head{display:flex;justify-content:space-between;align-items:center;gap:12px;margin-bottom:14px;flex-wrap:wrap}
    .proxy-head h2{margin:0}
    .proxy-stats{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin-bottom:14px}
    .proxy-stat{background:#f8fafc;border:1px solid #ebeff5;border-radius:10px;padding:14px}
    .proxy-stat span{color:#667085;font-size:13px}
    .proxy-stat b{display:block;font-size:28px;margin-top:6px}
    .dot{display:inline-block;width:8px;height:8px;border-radius:50%;background:#12b76a;margin-right:6px}
    .btn-danger{color:#b42318;border-color:#fecdca}
    .btn-danger:hover{background:#fef3f2}
    .modal-mask{position:fixed;inset:0;background:rgba(16,24,40,.45);display:none;align-items:center;justify-content:center;z-index:20}
    .modal-mask.show{display:flex}
    .modal-card{background:#fff;border-radius:12px;padding:18px;width:min(560px,92vw);box-shadow:0 20px 50px rgba(16,24,40,.18)}
    .ops button{margin-right:4px}
    .proxy-url{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;word-break:break-all}
    @media (max-width:900px){.proxy-stats{grid-template-columns:1fr 1fr}}
    .detail-empty{height:100%;display:flex;align-items:center;justify-content:center;color:#667085}
    .detail-body{padding:14px}
    .message{border:1px solid #e0e5ee;border-radius:8px;margin:0 0 12px;background:#fff}
    .message-head{display:flex;justify-content:space-between;gap:12px;padding:8px 10px;border-bottom:1px solid #edf1f6;background:#fbfcfe;font-weight:700}
    .message-content{padding:10px;white-space:pre-wrap;word-break:break-word;font-size:13px;line-height:1.55}
    .message-content.reasoning{border-top:1px dashed #d0d5dd;color:#475467;background:#fcfcfd}
    @media (max-width:900px){
      .shell{grid-template-columns:1fr}
      .nav{position:sticky;top:0;z-index:2;display:flex;gap:8px;align-items:center;overflow:auto}
      .brand{margin:0 8px 0 0;white-space:nowrap}
      .nav button{width:auto;white-space:nowrap}
      .history-layout{grid-template-columns:1fr;height:auto}
      .history-list,.history-detail{max-height:none}
    }
  </style>
</head>
<body>
<div class="shell">
  <aside class="nav">
    <div class="brand">zai2api Admin</div>
    <button id="navAccount" class="active" onclick="showView('account')">账号管理</button>
    <button id="navHistory" onclick="showView('history')">聊天记录</button>
    <button id="navProxies" onclick="showView('proxies')">代理池</button>
    <button id="navSettings" onclick="showView('settings')">系统设置</button>
  </aside>
  <main class="content">
    <div id="viewAccount" class="view active">
      <section>
        <h2>统计</h2>
        <div class="grid" id="metrics"></div>
      </section>
      <section>
        <h2>账号 / Token</h2>
        <div class="row">
          <button class="primary" onclick="validateTokens()">立即验证</button>
          <button onclick="loadAccount()">刷新</button>
        </div>
        <p><textarea id="newToken" placeholder="粘贴 Z.AI token"></textarea></p>
        <button onclick="addToken()">添加 Token</button>
        <table><thead><tr><th>Email</th><th>User ID</th><th>Token</th><th>状态</th><th>来源</th><th>失败原因</th><th>使用次数</th><th>操作</th></tr></thead><tbody id="tokens"></tbody></table>
      </section>
      <section>
        <h2>上游 Endpoints</h2>
        <div class="row">
          <input id="newEndpoint" style="min-width:360px;flex:1" placeholder="https://example.workers.dev/api/v2/chat/completions">
          <button class="primary" onclick="addEndpoint()">添加 Endpoint</button>
        </div>
        <table><thead><tr><th>Endpoint</th><th>状态</th><th>操作</th></tr></thead><tbody id="endpoints"></tbody></table>
      </section>
    </div>
    <div id="viewHistory" class="view">
      <div class="history-layout">
        <div class="history-list">
          <div class="history-head row">
            <h2 style="margin:0;flex:1">聊天记录</h2>
            <button onclick="loadHistory()">刷新</button>
          </div>
          <div id="historyList"></div>
          <div class="history-pager">
            <button id="historyPrev" onclick="changeHistoryPage(-1)">上一页</button>
            <span id="historyPageInfo" class="history-page-info"></span>
            <button id="historyNext" onclick="changeHistoryPage(1)">下一页</button>
          </div>
        </div>
        <div class="history-detail" id="historyDetail"><div class="detail-empty">选择一条聊天记录</div></div>
      </div>
    </div>
    <div id="viewProxies" class="view">
      <div class="proxy-head">
        <h2>代理池</h2>
        <div class="row">
          <button id="btnPoolToggle" onclick="toggleProxyPool()">停用代理池</button>
          <button onclick="checkAllProxies()">检测全部</button>
          <button class="primary" onclick="openProxyModal()">+ 添加代理</button>
        </div>
      </div>
      <p class="muted" style="margin-top:0">开启后整条链路直连 chat.z.ai 并走对应代理，不再走 Cloudflare Worker。可一次粘贴多个地址。</p>
      <div class="proxy-stats">
        <div class="proxy-stat"><span>代理总数</span><b id="proxyTotal">0</b></div>
        <div class="proxy-stat"><span>已启用</span><b id="proxyActive">0</b></div>
        <div class="proxy-stat"><span>健康</span><b id="proxyHealthy">0</b></div>
        <div class="proxy-stat"><span>代理池轮换</span><b id="proxyRotate"><span class="dot"></span>关闭</b></div>
      </div>
      <section style="padding:0;overflow:auto">
        <table>
          <thead><tr><th>代理地址</th><th>状态</th><th>出口IP</th><th>地区</th><th>延迟</th><th>成功/失败</th><th>最近检测</th><th>操作</th></tr></thead>
          <tbody id="proxyRows"></tbody>
        </table>
      </section>
    </div>
    <div id="viewSettings" class="view">
      <section>
        <h2>系统设置</h2>
        <div style="display:flex;gap:16px;flex-wrap:wrap;align-items:flex-end;margin-bottom:12px">
          <label>Token 重试次数（默认 0）<br><input id="setRetryCount" type="number" min="0" style="width:120px"></label>
          <label>模型降级链（逗号分隔，撞容量满/无权限时依次降级）<br><input id="setModelFallbacks" style="min-width:340px" placeholder="GLM-5-Turbo,GLM-4.6"></label>
          <button onclick="saveSettings()">保存</button>
          <span id="settingsMsg"></span>
        </div>
        <pre id="settings"></pre>
      </section>
    </div>
    <div id="proxyModal" class="modal-mask" onclick="if(event.target===this)closeProxyModal()">
      <div class="modal-card">
        <h2>添加代理</h2>
        <p class="muted">每行一个，或用逗号分隔，可一次添加多个。</p>
        <p><textarea id="proxyAddText" placeholder="http://user:pass@host:port"></textarea></p>
        <div class="row">
          <button class="primary" onclick="submitAddProxies()">添加</button>
          <button onclick="closeProxyModal()">取消</button>
        </div>
      </div>
    </div>
  </main>
</div>
<script>
const adminToken = new URLSearchParams(location.search).get('token') || '';
let currentView = 'account';
let selectedHistoryId = '';
let historyPage = 1;
const historyPageSize = 20;
function adminPath(path) {
  if (!adminToken) return path;
  const u = new URL(path, location.origin);
  u.searchParams.set('token', adminToken);
  return u.pathname + u.search;
}
async function api(path, opts={}) {
  const r = await fetch(adminPath(path), Object.assign({headers:{'Content-Type':'application/json'}}, opts));
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}
function showView(name){
  currentView = name;
  for (const v of ['account','history','proxies','settings']) {
    document.getElementById('view'+cap(v)).classList.toggle('active', v === name);
    document.getElementById('nav'+cap(v)).classList.toggle('active', v === name);
  }
  if (name === 'account') loadAccount();
  if (name === 'history') loadHistory();
  if (name === 'proxies') loadProxies();
  if (name === 'settings') loadSettings();
}
function cap(s){return s.charAt(0).toUpperCase()+s.slice(1)}
function metric(label, value){return '<div class="metric">'+label+'<b>'+value+'</b></div>'}
async function loadStats(){
  const s = await api('/admin/api/stats');
  const t = s.telemetry;
  metrics.innerHTML = [
    metric('请求数', t.total_requests),
    metric('RPM', t.rpm),
    metric('有效账号', t.valid_tokens),
    metric('成功率', (t.success_rate||0).toFixed(1)+'%'),
    metric('输入 Tokens', t.total_input_tokens),
    metric('输出 Tokens', t.total_output_tokens)
  ].join('');
}
async function loadTokens(){
  const data = await api('/admin/api/tokens');
  tokens.innerHTML = data.tokens.map(t => {
    const raw = t.token || t.token_preview || '';
    const arg = JSON.stringify(raw).replace(/"/g,'&quot;');
    const status = t.valid ? '<span class="ok">有效</span>' : '<span class="bad">失效</span>';
    const restore = t.valid ? '' : '<button onclick="restoreToken('+arg+')">重新启用</button> ';
    return '<tr><td>'+escapeHtml(t.email||'')+'</td><td>'+escapeHtml(t.user_id||'')+'</td><td class="token">'+escapeHtml(raw)+'</td><td>'+status+'</td><td>'+escapeHtml(t.source||'')+'</td><td class="reason">'+escapeHtml(t.invalid_reason||'')+'</td><td>'+t.use_count+'</td><td><button onclick="testToken(this,'+arg+')">测试</button> '+restore+'<button onclick="deleteToken('+arg+')">删除</button></td></tr>';
  }).join('');
}
async function loadEndpoints(){
  const data = await api('/admin/api/endpoints');
  endpoints.innerHTML = data.endpoints.map((endpoint, i) => {
    const arg = JSON.stringify(endpoint).replace(/"/g,'&quot;');
    const status = i === 0 ? '<span class="ok">默认</span>' : '轮询';
    const del = data.endpoints.length <= 1 ? '' : '<button onclick="deleteEndpoint('+arg+')">删除</button>';
    return '<tr><td class="token">'+escapeHtml(endpoint)+'</td><td>'+status+'</td><td>'+del+'</td></tr>';
  }).join('');
}
async function loadSettings(){
  const s = await api('/admin/api/settings');
  document.getElementById('setRetryCount').value = s.retry_count ?? 0;
  document.getElementById('setModelFallbacks').value = (s.model_fallbacks || []).join(', ');
  settings.textContent = JSON.stringify(s, null, 2);
}
async function saveSettings(){
  const msg = document.getElementById('settingsMsg');
  const rc = parseInt(document.getElementById('setRetryCount').value, 10);
  const fb = document.getElementById('setModelFallbacks').value.split(',').map(x=>x.trim()).filter(Boolean);
  try {
    await api('/admin/api/settings', {method:'POST', body:JSON.stringify({retry_count: isNaN(rc)?0:rc, model_fallbacks: fb})});
    msg.textContent = '已保存 ✓';
  } catch(e) { msg.textContent = '保存失败: ' + e.message; }
  loadSettings();
}
function fmtTime(ts){
  if(!ts) return '-';
  const d = new Date(ts*1000);
  return d.getFullYear()+'/'+(d.getMonth()+1)+'/'+d.getDate()+' '+String(d.getHours()).padStart(2,'0')+':'+String(d.getMinutes()).padStart(2,'0')+':'+String(d.getSeconds()).padStart(2,'0');
}
function proxyStatusHtml(p){
  let html = '';
  if (p.exit_ip && !p.last_error) html += '<span class="badge healthy">健康</span> ';
  else if (p.last_error) html += '<span class="badge error" title="'+escapeHtml(p.last_error)+'">异常</span> ';
  if (!p.enabled) html += '<span class="badge disabled">禁用</span>';
  return html || '<span class="muted">未检测</span>';
}
async function loadProxies(){
  const data = await api('/admin/api/proxies');
  proxyTotal.textContent = data.stats.total;
  proxyActive.textContent = data.stats.active;
  proxyHealthy.textContent = data.stats.healthy;
  proxyRotate.innerHTML = data.enabled ? '<span class="dot"></span>开启' : '<span class="dot" style="background:#98a2b3"></span>关闭';
  const btn = document.getElementById('btnPoolToggle');
  btn.textContent = data.enabled ? '停用代理池' : '启用代理池';
  proxyRows.innerHTML = (data.items||[]).map(p => {
    const arg = JSON.stringify(p.url).replace(/"/g,'&quot;');
    const lat = p.latency_ms ? p.latency_ms+' ms' : '-';
    const toggle = p.enabled ? '<button onclick="toggleProxy('+arg+',false)">停用</button>' : '<button onclick="toggleProxy('+arg+',true)">启用</button>';
    return '<tr><td class="proxy-url">'+escapeHtml(p.url)+'</td><td>'+proxyStatusHtml(p)+'</td><td>'+escapeHtml(p.exit_ip||'-')+'</td><td>'+escapeHtml(p.region||'-')+'</td><td>'+lat+'</td><td>'+(p.success||0)+' / '+(p.fail||0)+'</td><td>'+fmtTime(p.last_check)+'</td><td class="ops"><button onclick="checkProxy(this,'+arg+')">检测</button> '+toggle+' <button class="btn-danger" onclick="deleteProxy('+arg+')">删除</button></td></tr>';
  }).join('');
}
function openProxyModal(){ proxyModal.classList.add('show'); proxyAddText.focus(); }
function closeProxyModal(){ proxyModal.classList.remove('show'); }
async function submitAddProxies(){
  const urls = proxyAddText.value.split(/[\n,;]+/).map(x=>x.trim()).filter(Boolean);
  if(!urls.length) return;
  await api('/admin/api/proxies',{method:'POST',body:JSON.stringify({urls})});
  proxyAddText.value='';
  closeProxyModal();
  loadProxies();
}
async function deleteProxy(url){ if(!confirm('删除 '+url+' ?')) return; await api('/admin/api/proxies/delete',{method:'POST',body:JSON.stringify({url})}); loadProxies(); }
async function toggleProxy(url, enabled){ await api('/admin/api/proxies/toggle',{method:'POST',body:JSON.stringify({url,enabled})}); loadProxies(); }
async function toggleProxyPool(){
  const on = document.getElementById('btnPoolToggle').textContent.indexOf('启用')>=0;
  await api('/admin/api/proxies/toggle',{method:'POST',body:JSON.stringify({pool:on})});
  loadProxies();
}
async function checkProxy(btn, url){
  const old = btn.textContent;
  btn.textContent='检测中';
  btn.classList.add('testing');
  try { await api('/admin/api/proxies/check',{method:'POST',body:JSON.stringify({url})}); }
  finally { btn.textContent=old; btn.classList.remove('testing'); loadProxies(); }
}
async function checkAllProxies(){
  const btn = event && event.target;
  if(btn){ btn.classList.add('testing'); btn.textContent='检测中'; }
  try { await api('/admin/api/proxies/check',{method:'POST',body:JSON.stringify({all:true})}); }
  finally { if(btn){ btn.classList.remove('testing'); btn.textContent='检测全部'; } loadProxies(); }
}
async function addToken(){ await api('/admin/api/tokens',{method:'POST',body:JSON.stringify({token:newToken.value})}); newToken.value=''; loadAccount(); }
async function addEndpoint(){ const endpoint = newEndpoint.value.trim(); if(!endpoint) return; await api('/admin/api/endpoints',{method:'POST',body:JSON.stringify({endpoint})}); newEndpoint.value=''; loadAccount(); }
async function deleteEndpoint(endpoint){ if(!confirm('删除 '+endpoint+' ?')) return; await api('/admin/api/endpoints',{method:'DELETE',body:JSON.stringify({endpoint})}); loadAccount(); }
async function validateTokens(){ await api('/admin/api/tokens/validate',{method:'POST',body:'{}'}); loadAccount(); }
async function testToken(btn, token){
  btn.textContent='测试中';
  btn.classList.add('testing');
  try {
    const r = await api('/admin/api/tokens/test',{method:'POST',body:JSON.stringify({token:token})});
    alert(r.token && r.token.valid ? '测试通过' : '测试失败：'+((r.token && r.token.invalid_reason)||'未知原因'));
  } finally {
    loadAccount();
  }
}
async function deleteToken(token){ if(!confirm('删除 '+token+' ?')) return; await api('/admin/api/tokens/delete',{method:'POST',body:JSON.stringify({token:token})}); loadAccount(); }
async function restoreToken(token){ await api('/admin/api/tokens/restore',{method:'POST',body:JSON.stringify({token:token})}); loadAccount(); }
async function loadHistory(){
  const data = await api('/admin/api/history?page='+historyPage+'&page_size='+historyPageSize);
  const items = data.history || [];
  const total = data.total || 0;
  const totalPages = data.total_pages || 1;
  if (historyPage > totalPages) {
    historyPage = totalPages;
    return loadHistory();
  }
  historyPageInfo.textContent = total ? '第 '+historyPage+' / '+totalPages+' 页，共 '+total+' 条' : '共 0 条';
  historyPrev.disabled = historyPage <= 1;
  historyNext.disabled = historyPage >= totalPages;
  historyList.innerHTML = items.length ? items.map(item => historyListItem(item)).join('') : '<div class="detail-empty" style="height:140px">暂无聊天记录</div>';
  if (selectedHistoryId && items.some(i => i.id === selectedHistoryId)) {
    await loadHistoryDetail(selectedHistoryId);
  } else if (items.length) {
    await loadHistoryDetail(items[0].id);
  } else {
    selectedHistoryId = '';
    historyDetail.innerHTML = '<div class="detail-empty">暂无聊天记录</div>';
  }
}
function changeHistoryPage(delta){
  const next = historyPage + delta;
  if (next < 1) return;
  historyPage = next;
  selectedHistoryId = '';
  loadHistory();
}
function historyListItem(item){
  const active = item.id === selectedHistoryId ? ' active' : '';
  return '<button class="history-item'+active+'" onclick="loadHistoryDetail(\''+escapeAttr(item.id)+'\')">'+
    '<div class="history-title"><span>'+escapeHtml(item.model||'unknown')+'</span>'+statusBadge(item.status)+'</div>'+
    '<div class="history-meta">'+formatTime(item.request_time)+' · '+formatDurationMs(item.duration_ms)+'<br>输入 '+(item.input_tokens||0)+' / 输出 '+(item.output_tokens||0)+' / 总计 '+(item.total_tokens||0)+'</div>'+
  '</button>';
}
async function loadHistoryDetail(id){
  selectedHistoryId = id;
  for (const el of document.querySelectorAll('.history-item')) el.classList.remove('active');
  const data = await api('/admin/api/history?id='+encodeURIComponent(id));
  const h = data.history;
  historyDetail.innerHTML = renderHistoryDetail(h);
  for (const el of document.querySelectorAll('.history-item')) {
    if (el.getAttribute('onclick') && el.getAttribute('onclick').includes(id)) el.classList.add('active');
  }
}
function renderHistoryDetail(h){
  const err = h.error ? '<section><h2>错误</h2><pre>'+escapeHtml(h.error)+'</pre></section>' : '';
  const messages = (h.messages||[]).map((m, i) => {
    const reasoning = m.reasoning_content ? '<div class="message-content reasoning">'+escapeHtml(m.reasoning_content)+'</div>' : '';
    const toolCalls = m.tool_calls && m.tool_calls.length ? '\n\nTool Calls:\n'+JSON.stringify(m.tool_calls, null, 2) : '';
    return '<div class="message"><div class="message-head"><span>'+(i+1)+'. '+escapeHtml(m.role||'unknown')+'</span></div><div class="message-content">'+escapeHtml((m.content||'')+toolCalls)+'</div>'+reasoning+'</div>';
  }).join('');
  return '<div class="detail-body">'+
    '<section><h2>请求信息</h2><div class="grid">'+
    metric('请求模型', escapeHtml(h.model||'unknown'))+
    metric('上游模型', escapeHtml(h.upstream_model||'-'))+
    metric('状态', statusBadge(h.status))+
    metric('请求时间', formatTime(h.request_time))+
    metric('耗时', formatDurationMs(h.duration_ms))+
    metric('Tokens', (h.input_tokens||0)+' / '+(h.output_tokens||0)+' / '+(h.total_tokens||0))+
    '</div></section>'+err+
    '<section><h2>对话内容</h2>'+(messages || '<div class="muted">无消息内容</div>')+'</section>'+
  '</div>';
}
function statusBadge(status){
  const label = status === 'success' ? '成功' : status === 'failed' ? '失败' : '进行中';
  const cls = status === 'success' ? 'success' : status === 'failed' ? 'failed' : 'running';
  return '<span class="badge '+cls+'">'+label+'</span>';
}
function formatTime(value){
  if (!value) return '-';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '-';
  return d.toLocaleString();
}
function formatDurationMs(ms){
  if (ms === null || ms === undefined) return '-';
  if (ms < 1000) return ms+'ms';
  return (ms/1000).toFixed(2)+'s';
}
function escapeHtml(s){return String(s).replace(/[&<>"']/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]))}
function escapeAttr(s){return String(s).replace(/['\\]/g,'\\$&')}
function loadAccount(){ loadStats(); loadTokens(); loadEndpoints(); }
loadAccount(); loadSettings(); setInterval(() => { if (currentView === 'account') loadStats(); if (currentView === 'history') loadHistory(); }, 5000);
</script>
</body></html>`

func HandleAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(adminHTML))
}

func RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if Cfg.AdminToken == "" {
			next(w, r)
			return
		}
		token := r.Header.Get("X-Admin-Token")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token != Cfg.AdminToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func HandleAdminStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"telemetry": GetTelemetryData()})
}

func HandleAdminTokens(w http.ResponseWriter, r *http.Request) {
	tm := GetTokenManager()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"tokens": tm.ListTokens(true), "stats": tm.GetStats()})
	case http.MethodPost:
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := tm.AddToken(req.Token); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func HandleAdminTokenDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token        string `json:"token"`
		TokenPreview string `json:"token_preview"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var err error
	if req.Token != "" {
		err = GetTokenManager().DeleteToken(req.Token)
	} else {
		err = GetTokenManager().DeleteTokenByPreview(req.TokenPreview)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func HandleAdminTokenRestore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token        string `json:"token"`
		TokenPreview string `json:"token_preview"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var err error
	if req.Token != "" {
		err = GetTokenManager().RestoreToken(req.Token)
	} else {
		err = GetTokenManager().RestoreTokenByPreview(req.TokenPreview)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func HandleAdminTokenTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token        string `json:"token"`
		TokenPreview string `json:"token_preview"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var (
		info PublicTokenInfo
		err  error
	)
	if req.Token != "" {
		info, err = GetTokenManager().TestToken(req.Token)
	} else {
		info, err = GetTokenManager().TestTokenByPreview(req.TokenPreview)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "token": info})
}

func HandleAdminTokenValidate(w http.ResponseWriter, r *http.Request) {
	GetTokenManager().ValidateNow()
	writeJSON(w, map[string]any{"ok": true})
}

func HandleAdminEndpoints(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		endpoints := GetAPIEndpoints()
		writeJSON(w, map[string]any{
			"endpoints": endpoints,
			"default":   endpoints[0],
		})
	case http.MethodPost:
		var req struct {
			Endpoint  string   `json:"endpoint"`
			Endpoints []string `json:"endpoints"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var err error
		if len(req.Endpoints) > 0 {
			err = SetAPIEndpoints(req.Endpoints)
		} else {
			err = AddAPIEndpoint(req.Endpoint)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "endpoints": GetAPIEndpoints()})
	case http.MethodDelete:
		var req struct {
			Endpoint string `json:"endpoint"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := DeleteAPIEndpoint(req.Endpoint); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "endpoints": GetAPIEndpoints()})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func HandleAdminHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id != "" {
		record, err := GetChatHistory(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"history": record})
		return
	}
	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	pageSize := parsePositiveInt(r.URL.Query().Get("page_size"), 20)
	if pageSize > 100 {
		pageSize = 100
	}
	history, total, err := ListChatHistoryPage(page, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	writeJSON(w, map[string]any{
		"history":     history,
		"page":        page,
		"page_size":   pageSize,
		"total":       total,
		"total_pages": totalPages,
		"has_prev":    page > 1,
		"has_next":    page < totalPages,
	})
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func HandleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			RetryCount              *int      `json:"retry_count"`
			ModelFallbacks          *[]string `json:"model_fallbacks"`
			CaptchaProxyPoolEnabled *bool     `json:"captcha_proxy_pool_enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		patch := RuntimeSettingsPatch{
			RetryCount:              req.RetryCount,
			CaptchaProxyPoolEnabled: req.CaptchaProxyPoolEnabled,
		}
		if req.ModelFallbacks != nil {
			var fallbacks []string
			for _, m := range *req.ModelFallbacks {
				m = strings.TrimSpace(m)
				if m == "" {
					continue
				}
				if !IsValidModel(m) {
					http.Error(w, "无效的备用模型: "+m, http.StatusBadRequest)
					return
				}
				fallbacks = append(fallbacks, m)
			}
			patch.ModelFallbacks = &fallbacks
		}
		if err := UpdateRuntimeSettings(patch); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// 落回 GET 返回最新设置
	}

	endpoints := GetAPIEndpoints()
	writeJSON(w, map[string]any{
		"port":                       Cfg.Port,
		"config_path":                Cfg.ConfigPath,
		"api_endpoint":               endpoints[0],
		"api_endpoints":              endpoints,
		"auth_tokens":                len(Cfg.AuthTokens),
		"backup_tokens":              len(Cfg.BackupTokens),
		"debug_logging":              Cfg.DebugLogging,
		"tool_support":               Cfg.ToolSupport,
		"retry_count":                GetRetryCount(),
		"model_fallbacks":            GetModelFallbacks(),
		"captcha_proxy_pool_enabled": GetCaptchaProxyPoolEnabled(),
		"captcha_auto_gen":           Cfg.CaptchaAutoGen,
		"skip_auth_token":            Cfg.SkipAuthToken,
		"scan_limit":                 Cfg.ScanLimit,
		"log_level":                  Cfg.LogLevel,
		"spoof_client_ip":            Cfg.SpoofClientIP,
		"admin_token_set":            Cfg.AdminToken != "",
		"env": map[string]string{
			"ZAI_BROWSER":        os.Getenv("ZAI_BROWSER"),
			"ADSPOWER_API_URL":   os.Getenv("ADSPOWER_API_URL"),
			"ADSPOWER_GROUP_ID":  os.Getenv("ADSPOWER_GROUP_ID"),
			"ZAI_VISION_MODEL":   os.Getenv("ZAI_VISION_MODEL"),
			"ZAI_MAIL_PROVIDER":  os.Getenv("ZAI_MAIL_PROVIDER"),
			"ZAI_REGISTER_PROXY": os.Getenv("ZAI_REGISTER_PROXY"),
		},
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
