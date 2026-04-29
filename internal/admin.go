package internal

import (
	"encoding/json"
	"net/http"
	"os"
)

const adminHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>zai2api Admin</title>
  <style>
    body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;margin:0;background:#f6f7f9;color:#1f2937}
    header{height:56px;background:#111827;color:#fff;display:flex;align-items:center;padding:0 24px;font-weight:700}
    main{padding:20px;display:grid;gap:16px}
    section{background:#fff;border:1px solid #e5e7eb;border-radius:8px;padding:16px}
    h2{margin:0 0 12px;font-size:16px}
    .grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:12px}
    .metric{background:#f9fafb;border:1px solid #eef0f3;border-radius:6px;padding:12px}
    .metric b{display:block;font-size:22px;margin-top:4px}
    table{width:100%;border-collapse:collapse;font-size:13px}
    th,td{border-bottom:1px solid #eef0f3;padding:8px;text-align:left}
    .token{word-break:break-all;max-width:520px}
    .reason{word-break:break-word;max-width:360px;color:#6b7280}
    .ok{color:#047857;font-weight:700}
    .bad{color:#b91c1c;font-weight:700}
    .testing{opacity:.65;pointer-events:none}
    input,textarea,button{font:inherit}
    textarea{width:100%;min-height:80px}
    button{border:1px solid #d1d5db;background:#fff;border-radius:6px;padding:7px 10px;cursor:pointer}
    button.primary{background:#2563eb;color:#fff;border-color:#2563eb}
    .row{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
    pre{background:#111827;color:#e5e7eb;padding:12px;border-radius:6px;overflow:auto}
  </style>
</head>
<body>
<header>zai2api Admin</header>
<main>
  <section>
    <h2>统计</h2>
    <div class="grid" id="metrics"></div>
  </section>
  <section>
    <h2>账号 / Token</h2>
    <div class="row">
      <button class="primary" onclick="validateTokens()">立即验证</button>
      <button onclick="loadAll()">刷新</button>
    </div>
    <p><textarea id="newToken" placeholder="粘贴 Z.AI token"></textarea></p>
    <button onclick="addToken()">添加 Token</button>
    <table><thead><tr><th>Email</th><th>User ID</th><th>Token</th><th>状态</th><th>来源</th><th>失败原因</th><th>使用次数</th><th>操作</th></tr></thead><tbody id="tokens"></tbody></table>
  </section>
  <section>
    <h2>系统设置</h2>
    <pre id="settings"></pre>
  </section>
</main>
<script>
async function api(path, opts={}) {
  const r = await fetch(path, Object.assign({headers:{'Content-Type':'application/json'}}, opts));
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}
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
async function loadSettings(){ settings.textContent = JSON.stringify(await api('/admin/api/settings'), null, 2); }
async function addToken(){ await api('/admin/api/tokens',{method:'POST',body:JSON.stringify({token:newToken.value})}); newToken.value=''; loadAll(); }
async function validateTokens(){ await api('/admin/api/tokens/validate',{method:'POST',body:'{}'}); loadAll(); }
async function testToken(btn, token){
  btn.textContent='测试中';
  btn.classList.add('testing');
  try {
    const r = await api('/admin/api/tokens/test',{method:'POST',body:JSON.stringify({token:token})});
    alert(r.token && r.token.valid ? '测试通过' : '测试失败：'+((r.token && r.token.invalid_reason)||'未知原因'));
  } finally {
    loadAll();
  }
}
async function deleteToken(token){ if(!confirm('删除 '+token+' ?')) return; await api('/admin/api/tokens/delete',{method:'POST',body:JSON.stringify({token:token})}); loadAll(); }
async function restoreToken(token){ await api('/admin/api/tokens/restore',{method:'POST',body:JSON.stringify({token:token})}); loadAll(); }
function escapeHtml(s){return String(s).replace(/[&<>"']/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]))}
function loadAll(){ loadStats(); loadTokens(); loadSettings(); }
loadAll(); setInterval(loadStats, 5000);
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

func HandleAdminSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"port":            Cfg.Port,
		"api_endpoint":    Cfg.APIEndpoint,
		"auth_tokens":     len(Cfg.AuthTokens),
		"backup_tokens":   len(Cfg.BackupTokens),
		"debug_logging":   Cfg.DebugLogging,
		"tool_support":    Cfg.ToolSupport,
		"retry_count":     Cfg.RetryCount,
		"skip_auth_token": Cfg.SkipAuthToken,
		"scan_limit":      Cfg.ScanLimit,
		"log_level":       Cfg.LogLevel,
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
