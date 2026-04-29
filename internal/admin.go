package internal

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
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
    <div id="viewSettings" class="view">
      <section>
        <h2>系统设置</h2>
        <pre id="settings"></pre>
      </section>
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
  for (const v of ['account','history','settings']) {
    document.getElementById('view'+cap(v)).classList.toggle('active', v === name);
    document.getElementById('nav'+cap(v)).classList.toggle('active', v === name);
  }
  if (name === 'account') loadAccount();
  if (name === 'history') loadHistory();
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
async function loadSettings(){ settings.textContent = JSON.stringify(await api('/admin/api/settings'), null, 2); }
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
	endpoints := GetAPIEndpoints()
	writeJSON(w, map[string]any{
		"port":            Cfg.Port,
		"config_path":     Cfg.ConfigPath,
		"api_endpoint":    endpoints[0],
		"api_endpoints":   endpoints,
		"auth_tokens":     len(Cfg.AuthTokens),
		"backup_tokens":   len(Cfg.BackupTokens),
		"debug_logging":   Cfg.DebugLogging,
		"tool_support":    Cfg.ToolSupport,
		"retry_count":     Cfg.RetryCount,
		"skip_auth_token": Cfg.SkipAuthToken,
		"scan_limit":      Cfg.ScanLimit,
		"log_level":       Cfg.LogLevel,
		"spoof_client_ip": Cfg.SpoofClientIP,
		"admin_token_set": Cfg.AdminToken != "",
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
