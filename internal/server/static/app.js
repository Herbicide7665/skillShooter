/* Skill Hub SPA */
const $app = document.getElementById('app');
const $nav = document.querySelectorAll('#navlinks a');

const state = { machines: [], glm: null };

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}
function toast(msg, isErr) {
  const t = document.getElementById('toast');
  t.textContent = msg;
  t.className = 'toast' + (isErr ? ' err' : '');
  clearTimeout(t._timer);
  t._timer = setTimeout(() => t.classList.add('hidden'), 4000);
}
function getToken() { return localStorage.getItem('skhub_token') || ''; }

/* ---- 纯 JS SHA-256/HMAC：hub 是 http 非 secure context，crypto.subtle 不可用 ---- */
const _K = new Uint32Array([0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2]);
function rotr(x, n) { return (x >>> n) | (x << (32 - n)); }
function sha256Bytes(bytes) {
  const H = new Uint32Array([0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19]);
  const l = bytes.length, total = Math.ceil((l + 9) / 64) * 64;
  const buf = new Uint8Array(total);
  buf.set(bytes); buf[l] = 0x80;
  const dv = new DataView(buf.buffer);
  dv.setUint32(total - 8, Math.floor(l * 8 / 4294967296));
  dv.setUint32(total - 4, (l * 8) >>> 0);
  const w = new Uint32Array(64);
  for (let i = 0; i < total; i += 64) {
    for (let j = 0; j < 16; j++) w[j] = dv.getUint32(i + j * 4);
    for (let j = 16; j < 64; j++) {
      const s0 = rotr(w[j-15],7) ^ rotr(w[j-15],18) ^ (w[j-15] >>> 3);
      const s1 = rotr(w[j-2],17) ^ rotr(w[j-2],19) ^ (w[j-2] >>> 10);
      w[j] = (w[j-16] + s0 + w[j-7] + s1) >>> 0;
    }
    let a=H[0],b=H[1],c=H[2],d=H[3],e=H[4],f=H[5],g=H[6],h=H[7];
    for (let j = 0; j < 64; j++) {
      const S1 = rotr(e,6) ^ rotr(e,11) ^ rotr(e,25);
      const ch = (e & f) ^ (~e & g);
      const t1 = (h + S1 + ch + _K[j] + w[j]) >>> 0;
      const S0 = rotr(a,2) ^ rotr(a,13) ^ rotr(a,22);
      const maj = (a & b) ^ (a & c) ^ (b & c);
      const t2 = (S0 + maj) >>> 0;
      h=g; g=f; f=e; e=(d+t1)>>>0; d=c; c=b; b=a; a=(t1+t2)>>>0;
    }
    H[0]=(H[0]+a)>>>0; H[1]=(H[1]+b)>>>0; H[2]=(H[2]+c)>>>0; H[3]=(H[3]+d)>>>0;
    H[4]=(H[4]+e)>>>0; H[5]=(H[5]+f)>>>0; H[6]=(H[6]+g)>>>0; H[7]=(H[7]+h)>>>0;
  }
  const out = new Uint8Array(32), odv = new DataView(out.buffer);
  for (let i = 0; i < 8; i++) odv.setUint32(i * 4, H[i]);
  return out;
}
function concatBytes(a, b) { const o = new Uint8Array(a.length + b.length); o.set(a); o.set(b, a.length); return o; }
function hmacSha256(keyBytes, msgBytes) {
  let k = keyBytes;
  if (k.length > 64) k = sha256Bytes(k);
  const kp = new Uint8Array(64); kp.set(k);
  const oKey = new Uint8Array(64), iKey = new Uint8Array(64);
  for (let i = 0; i < 64; i++) { oKey[i] = kp[i] ^ 0x5c; iKey[i] = kp[i] ^ 0x36; }
  return sha256Bytes(concatBytes(oKey, sha256Bytes(concatBytes(iKey, msgBytes))));
}
function bytesToHex(b) { return Array.from(b).map(x => x.toString(16).padStart(2, '0')).join(''); }
const utf8Bytes = s => new TextEncoder().encode(s);

/* ---- HMAC 请求签名（token 不上 wire，见 PROTOCOL.md）---- */
function signHeaders(method, pathWithQuery, bodyStr) {
  const token = getToken();
  if (!token) return {};
  const ts = Math.floor(Date.now() / 1000).toString();
  const nonce = bytesToHex(crypto.getRandomValues(new Uint8Array(16)));
  const bodySHA = bytesToHex(sha256Bytes(utf8Bytes(bodyStr || '')));
  const signStr = [method, pathWithQuery, ts, nonce, bodySHA].join('\n');
  const sign = bytesToHex(hmacSha256(utf8Bytes(token), utf8Bytes(signStr)));
  return {
    'X-Key-ID': bytesToHex(sha256Bytes(utf8Bytes(token))).slice(0, 16),
    'X-Timestamp': ts, 'X-Nonce': nonce, 'X-Body-SHA256': bodySHA, 'X-Sign': sign,
  };
}

async function askToken() {
  const t = prompt('访问 Skill Hub 需要接入 Token（见服务端启动日志 serve.log 或 skillhub.db 所在机器）');
  if (t && t.trim()) {
    localStorage.setItem('skhub_token', t.trim());
    return t.trim();
  }
  return '';
}
function authHeaders(extra) {
  const h = Object.assign({}, extra || {});
  if (getToken()) h['X-Token'] = getToken();
  return h;
}
async function api(path, opts) {
  opts = opts || {};
  const method = (opts.method || 'GET').toUpperCase();
  opts.headers = Object.assign({}, opts.headers || {}, signHeaders(method, '/api' + path, opts.body || ''));
  // 502/网络抖动自动重试（指数退避）：上游代理/网络瞬断时不至于整页卡死
  let r;
  for (let attempt = 0; ; attempt++) {
    try {
      r = await fetch('/api' + path, opts);
    } catch (e) {
      if (attempt < 2) { await new Promise(ok => setTimeout(ok, 800 * (attempt + 1))); continue; }
      throw new Error('网络错误（已重试）: ' + e.message);
    }
    if ((r.status === 502 || r.status === 503 || r.status === 504) && attempt < 2) {
      await new Promise(ok => setTimeout(ok, 800 * (attempt + 1)));
      continue;
    }
    break;
  }
  if (r.status === 401 && !opts._retried) {
    const t = await askToken();
    if (t) return api(path, Object.assign({}, opts, {_retried: true}));
  }
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(data.error || ('HTTP ' + r.status));
  return data;
}
async function apiPost(path, body) {
  return api(path, {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: body ? JSON.stringify(body) : undefined,
  });
}
async function apiPut(path, body) {
  return api(path, {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    body: body ? JSON.stringify(body) : undefined,
  });
}
// 带 token 的原始文件请求（正文/下载）
async function authFetch(url) {
  const path = url.startsWith('/api') ? url : '/api' + url;
  let r = await fetch(url, {headers: signHeaders('GET', path, '')});
  if (r.status === 401) {
    const t = await askToken();
    if (t) r = await fetch(url, {headers: signHeaders('GET', path, '')});
  }
  return r;
}
function fmtBytes(n) {
  if (!n && n !== 0) return '-';
  if (n < 1024) return n + ' B';
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
  return (n / 1048576).toFixed(2) + ' MB';
}
function fmtTime(s) { return (s || '').replace('T', ' ').replace('Z', '').slice(0, 19); }

function renderDiff(diffText) {
  if (!diffText) return '<p class="muted">两个版本的 SKILL.md 内容一致，无 diff。</p>';
  return '<pre class="diff">' + diffText.split('\n').map(l => {
    if (l.startsWith('+++') || l.startsWith('---')) return `<span class="diff-head">${esc(l)}</span>`;
    if (l.startsWith('@@')) return `<span class="diff-hunk">${esc(l)}</span>`;
    if (l.startsWith('+')) return `<span class="diff-add">${esc(l)}</span>`;
    if (l.startsWith('-')) return `<span class="diff-del">${esc(l)}</span>`;
    return esc(l);
  }).join('\n') + '</pre>';
}

/* ---------- 路由 ---------- */
function route() {
  if ($app._jobsTimer) { clearInterval($app._jobsTimer); $app._jobsTimer = null; }
  const h = location.hash.slice(2) || 'dashboard';
  const [page, ...rest] = h.split('/');
  $nav.forEach(a => a.classList.toggle('active', a.dataset.r === page || (page === 'skill' && a.dataset.r === 'skills')));
  const routes = {
    dashboard: pageDashboard, skills: pageSkills, skill: pageSkillDetail,
    graph: pageGraph, dedup: pageDedup, devices: pageDevices, jobs: pageJobs, settings: pageSettings,
  };
  (routes[page] || pageDashboard)(rest).catch(e => {
    $app.innerHTML = `<div class="panel" style="color:var(--danger)">加载失败: ${esc(e.message)}
      <button style="margin-left:12px" onclick="route()">重试</button></div>`;
  });
}
window.addEventListener('hashchange', route);

/* ---------- Dashboard ---------- */
async function pageDashboard() {
  $app.innerHTML = '<div class="loading">加载中…</div>';
  const st = await api('/stats');
  const machines = await api('/machines');
  $app.innerHTML = `
    <h1>总览</h1>
    <div class="cards">
      <div class="card"><div class="num">${st.skill_count}</div><div class="lbl">Skill 总数</div></div>
      <div class="card"><div class="num">${st.version_count}</div><div class="lbl">版本总数</div></div>
      <div class="card"><div class="num">${st.machine_count}</div><div class="lbl">接入机器</div></div>
      <div class="card"><div class="num">${st.open_dup_groups}</div><div class="lbl">待处理重复组</div></div>
      <div class="card"><div class="num">${st.edge_count}</div><div class="lbl">关联边</div></div>
      <div class="card"><div class="num">${fmtBytes(st.blob_bytes)}</div><div class="lbl">内容存储</div></div>
    </div>
    <h2>skill 家族（按命名聚类）</h2>
    <div class="panel" style="display:flex;gap:8px;flex-wrap:wrap" id="famchips">
      ${(await api('/families')).slice(0, 10).map(f => `<a href="#" class="famchip" onclick="sessionStorage.setItem('famPreselect','${esc(f.kind + ':' + esc(f.key))}');location.hash='#/skills';return false"><b>${esc(f.key)}</b> <span class="muted">${f.names.length} 个</span></a>`).join('') || '<span class="muted small">尚未生成。到「关系图」页点「更新关系」，GLM 会按内容聚类并固化。</span>'}
    </div>
    <h2>设备在线状态（30min 心跳探活，超 35min 判离线）</h2>
    <div id="devbox"><div class="loading">加载中…</div></div>
    <h2>最近变更流</h2>
    <table class="tbl"><thead><tr><th>时间</th><th>Skill</th><th>来源机器</th><th>类型</th><th>版本</th></tr></thead><tbody>
    ${st.recent.map(r => `<tr>
      <td class="muted small">${esc(fmtTime(r.created_at))}</td>
      <td><a href="#/skill/${r.skill_id}">${esc(r.name)}</a></td>
      <td><span class="badge b-machine">${esc(r.machine)}</span></td>
      <td><span class="badge b-${r.kind === 'merged' ? 'merged' : 'push'}">${r.kind === 'merged' ? '合并' : '推送'}</span></td>
      <td class="mono small">v${r.version_id}</td>
    </tr>`).join('') || '<tr><td colspan="5" class="muted">暂无记录，先用 skill-hub push 推送数据</td></tr>'}
    </tbody></table>`;
  document.getElementById('devbox').innerHTML = `
    <table class="tbl"><thead><tr><th>设备</th><th>状态</th><th>最后活跃</th><th>skill 数</th><th>推送次数</th></tr></thead><tbody>
    ${machines.map(m => `<tr>
      <td><b>${esc(m.hostname)}</b></td>
      <td><span class="badge ${m.online ? 'b-push' : 'b-exact'}">${m.online ? '● 在线' : '○ 离线'}</span></td>
      <td class="muted small">${esc(fmtTime(m.last_seen))}</td>
      <td>${m.skill_count}</td>
      <td>${m.push_count}</td>
    </tr>`).join('') || '<tr><td colspan="5" class="muted">尚无设备接入</td></tr>'}
    </tbody></table>
    <p class="muted small">管理分配与主动下发请到 <a href="#/devices">设备页</a>。</p>`;
}

/* ---------- Skills 列表 ---------- */
async function pageSkills() {
  if (!state.machines.length) state.machines = await api('/machines');
  const families = await api('/families');
  state.families = families;
  const pre = sessionStorage.getItem('famPreselect');
  $app.innerHTML = `
    <h1>Skills</h1>
    <div class="toolbar">
      <input type="text" id="q" placeholder="搜索名称或描述…" style="min-width:260px">
      <select id="mfilter"><option value="">全部机器</option>
        ${state.machines.map(m => `<option value="${esc(m.hostname)}">${esc(m.hostname)} (${m.skill_count})</option>`).join('')}
      </select>
      <select id="ffilter"><option value="">全部分类</option>
        ${families.map(f => `<option value="${esc(f.kind + ':' + f.key)}">${esc(f.key)}（${f.names.length}）</option>`).join('')}
      </select>
      <label class="small" style="display:inline-flex;align-items:center;gap:4px"><input type="checkbox" id="hidesys"> 隐藏系统自带</label>
      <label class="small" style="display:inline-flex;align-items:center;gap:4px"><input type="checkbox" id="hidedup"> 隐藏重复</label>
      <span class="muted small" id="cnt"></span>
    </div>
    <div id="tblbox"><div class="loading">加载中…</div></div>`;
  if (pre) { document.getElementById('ffilter').value = pre; sessionStorage.removeItem('famPreselect'); }
  const famColor = window._famColorOf || (() => null);
  const load = async () => {
    const q = document.getElementById('q').value.trim();
    const m = document.getElementById('mfilter').value;
    const fkey = document.getElementById('ffilter').value;
    let list = await api(`/skills?q=${encodeURIComponent(q)}&machine=${encodeURIComponent(m)}`);
    if (fkey) {
      const fam = families.find(f => f.kind + ':' + f.key === fkey);
      const names = new Set(fam ? fam.names : []);
      list = list.filter(s => names.has(s.name));
    }
    if (document.getElementById('hidesys').checked) list = list.filter(s => !s.is_system);
    if (document.getElementById('hidedup').checked) {
      const dupNames = new Set();
      try {
        const groups = await api('/dup/groups?status=open');
        groups.forEach(g => (g.members || []).forEach(mm => dupNames.add(mm.name)));
      } catch (e) {}
      list = list.filter(s => !dupNames.has(s.name));
    }
    document.getElementById('cnt').textContent = `${list.length} 个 skill`;
    document.getElementById('tblbox').innerHTML = `
      <table class="tbl"><thead><tr><th>名称</th><th>描述</th><th>机器</th><th>版本</th><th>更新时间</th><th>哈希</th></tr></thead><tbody>
      ${list.map(s => {
        const fam = families.find(f => f.names.includes(s.name));
        return `<tr>
        <td><a href="#/skill/${s.id}"><b>${esc(s.name)}</b></a>${fam ? ` <a class="badge b-related" style="margin-left:6px" title="家族筛选" href="#" onclick="sessionStorage.setItem('famPreselect','${esc(fam.kind + ':' + esc(fam.key))}');location.hash='#/skills';location.reload();return false">${esc(fam.key)}</a>` : ''}</td>
        <td class="desc-cell">${esc(s.description.slice(0, 120))}${s.description.length > 120 ? '…' : ''}</td>
        <td><span class="badge b-machine">${esc(s.machine)}</span></td>
        <td>${s.version_count}</td>
        <td class="muted small">${esc(fmtTime(s.updated_at))}</td>
        <td class="mono small muted">${esc(s.tree_hash.slice(0, 10))}</td>
      </tr>`; }).join('') || '<tr><td colspan="6" class="muted">无匹配结果</td></tr>'}
      </tbody></table>`;
  };
  const hs = document.getElementById('hidesys'), hd = document.getElementById('hidedup');
  hs.checked = localStorage.getItem('skhub_hidesys') !== '0'; // 默认隐藏系统 skill
  hd.checked = localStorage.getItem('skhub_hidedup') === '1';
  hs.addEventListener('change', () => { localStorage.setItem('skhub_hidesys', hs.checked ? '1' : '0'); load(); });
  hd.addEventListener('change', () => { localStorage.setItem('skhub_hidedup', hd.checked ? '1' : '0'); load(); });
  document.getElementById('q').addEventListener('input', debounce(load, 300));
  document.getElementById('mfilter').addEventListener('change', load);
  document.getElementById('ffilter').addEventListener('change', load);
  await load();
}
function debounce(f, ms) { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => f(...a), ms); }; }

/* ---------- Skill 详情 ---------- */
async function pageSkillDetail(rest) {
  const id = rest[0];
  $app.innerHTML = '<div class="loading">加载中…</div>';
  const d = await api(`/skills/${id}`);
  const s = d.skill;
  $app.innerHTML = `
    <h1>${esc(s.name)} <span class="badge b-machine">${esc(s.machine)}</span></h1>
    <div class="panel kv">
      <div class="k">描述</div><div>${esc(s.description) || '<span class="muted">无</span>'}</div>
      <div class="k">当前树哈希</div><div class="mono">${esc(s.tree_hash)}</div>
      <div class="k">版本数</div><div>${s.version_count}</div>
      <div class="k">收录 / 更新</div><div>${esc(fmtTime(s.created_at))} → ${esc(fmtTime(s.updated_at))}</div>
      <div class="k">系统 skill</div><div><span id="sysmark" class="badge ${s.is_system ? 'b-push' : 'b-open'}">${s.is_system ? '是' : '否'}</span>
        <button style="margin-left:8px" onclick="window._toggleSys(${s.id}, ${s.is_system ? 'false' : 'true'})">${s.is_system ? '取消系统标记' : '标记为系统 skill'}</button></div>
    </div>

    <h2>版本时间线（演变路线）</h2>
    <div class="panel">
      <ul class="timeline" id="vline"></ul>
      <div class="toolbar" style="margin-top:12px">
        <select id="dfrom"></select><span class="muted">→</span><select id="dto"></select>
        <button class="primary" id="dobtn">查看 SKILL.md Diff</button>
      </div>
      <div id="diffbox"></div>
    </div>

    <h2>文件浏览（最新版本）</h2>
    <div class="panel" id="filebox"><div class="loading">加载中…</div></div>

    <h2>关联关系</h2>
    <div class="panel">
      <b>出边（本 skill 指向）</b>
      <div class="edge-list">${edgeHTML(d.edges_out, '引用 →') || '<span class="muted small">无</span>'}</div>
      <b>入边（指向本 skill）</b>
      <div class="edge-list">${edgeHTML(d.edges_in, '→ 引用') || '<span class="muted small">无</span>'}</div>
      <span class="muted small">边类型：<span class="badge b-reference">reference 引用</span> <span class="badge b-related">related 相关</span> <span class="badge b-chain">chain 调用链</span>（关系图页「更新关系」可重扫）</span>
    </div>
    <div class="panel" id="fampanel"><span class="muted small">家族信息加载中…</span></div>`;

  // 版本时间线
  const vl = document.getElementById('vline');
  vl.innerHTML = d.versions.map(v => `
    <li><span class="ver">v${v.id}</span>
      <span class="when">${esc(fmtTime(v.created_at))}</span>
      <span class="badge ${v.kind === 'merged' ? 'b-merged' : 'b-push'}">${v.kind === 'merged' ? '合并产生' : '推送'}</span>
      <span class="mono small muted">${esc(v.tree_hash.slice(0, 12))}</span>
      <span class="small muted">${v.file_count} 文件 · ${fmtBytes(v.total_size)}</span>
      ${v.kind === 'merged' ? `<span class="small muted">来源版本: ${(v.source_version_ids || []).join(', ')}</span>` : ''}
    </li>`).join('');

  // diff 选择器
  const dfrom = document.getElementById('dfrom'), dto = document.getElementById('dto');
  d.versions.forEach(v => {
    dfrom.add(new Option('v' + v.id, v.id));
    dto.add(new Option('v' + v.id, v.id));
  });
  dfrom.selectedIndex = 0;
  dto.selectedIndex = d.versions.length - 1;
  document.getElementById('dobtn').onclick = async () => {
    const box = document.getElementById('diffbox');
    box.innerHTML = '<div class="loading">计算 diff…</div>';
    const r = await api(`/skills/${id}/diff?from=${dfrom.value}&to=${dto.value}`);
    box.innerHTML = renderDiff(r.diff);
  };

  // 文件浏览
  const fb = document.getElementById('filebox');
  if (s.latest_version_id) {
    const tree = await api(`/skills/${id}/tree?version=${s.latest_version_id}`);
    const paths = Object.keys(tree);
    fb.innerHTML = `
      <div class="toolbar"><select id="fpick">${paths.map(p => `<option value="${esc(p)}">${esc(p)}</option>`).join('')}</select>
      <span class="muted small" id="fmeta"></span></div>
      <pre class="code" id="fview">选择左侧文件查看内容</pre>`;
    const loadFile = async () => {
      const p = document.getElementById('fpick').value;
      const r = await authFetch(`/api/skills/${id}/file?version=${s.latest_version_id}&path=${encodeURIComponent(p)}`);
      document.getElementById('fview').textContent = await r.text();
      document.getElementById('fmeta').textContent = 'blob ' + tree[p].slice(0, 12);
    };
    document.getElementById('fpick').addEventListener('change', loadFile);
    await loadFile();
  } else {
    fb.innerHTML = '<span class="muted">无版本数据</span>';
  }

  api('/families').then(families => {
    const panel = document.getElementById('fampanel');
    if (!panel) return;
    const fam = families.find(f => f.names.includes(s.name));
    if (!fam) { panel.innerHTML = '<span class="muted small">此 skill 暂无所属家族（GLM 按内容聚类，关系图页「更新关系」可重建）。</span>'; return; }
    const siblings = {};
    (fam.Members || []).forEach(m => { if (m.Name !== s.name && !siblings[m.Name]) siblings[m.Name] = m; });
    panel.innerHTML = `<b>同族「${esc(fam.key)}」</b> <span class="badge b-related" title="${esc(fam.note || '')}">GLM 家族 · ${fam.names.length} 个</span>
      <div class="edge-list">${Object.values(siblings).map(m => `<div><a href="#/skill/${m.id}">${esc(m.Name)}</a> <span class="badge b-machine">${esc(m.Machine)}</span></div>`).join('')}</div>`;
  }).catch(() => {});

  function edgeHTML(edges) {
    return (edges || []).map(e => {
      const isOut = e.src_id === +id;
      return `<div><span class="badge b-${esc(e.kind)}">${esc(e.kind)}</span>
       <a href="#/skill/${isOut ? e.dst_id : e.src_id}">${esc(isOut ? e.dst : e.src)} ${isOut ? '→' : '←'}</a>
       ${e.note ? `<span class="muted small">— ${esc(e.note)}</span>` : ''}</div>`;
    }).join('');
  }
}

window._toggleSys = async (id, val) => {
  try {
    await apiPost(`/skills/${id}/system`, { is_system: val });
    toast(val ? '已标记为系统 skill' : '已取消系统标记');
    location.reload();
  } catch (e) { toast(e.message, true); }
};

/* ---------- 关系图 ---------- */
async function pageGraph() {
  const [g0, families] = await Promise.all([api('/graph'), api('/families')]);
  const g = g0;
  const palette = ['#3565e0','#8b5cf6','#148a4c','#e07b39','#c23b3b','#0e9aa7','#b8860b','#6d3fc0','#2f8f6b','#a349a4','#4a7fb5','#c76b98'];
  const famColorMap = {};
  families.forEach((f, i) => { famColorMap[f.kind + ':' + f.key] = palette[i % palette.length]; });
  const nameFam = {};
  families.forEach(f => { const c = famColorMap[f.kind + ':' + f.key]; f.names.forEach(n => nameFam[n] = c); });
  $app.innerHTML = `
    <h1>关系图</h1>
    <div class="toolbar">
      <button class="primary" id="rescan">更新关系（引用边 + GLM 关系/家族重构，固化）</button>
      <label class="small" style="display:inline-flex;align-items:center;gap:4px"><input type="checkbox" id="ghidesys"> 隐藏系统 skill</label>
      <span class="muted small" id="gstat"></span>
    </div>
    <div class="legend">
      <span><i style="background:#148a4c"></i>reference 引用</span>
      <span><i style="background:#3565e0"></i>related 相关</span>
      <span><i style="background:#8b5cf6"></i>chain 调用链</span>
      <span class="muted">节点颜色=家族：</span>
      ${families.map(f => `<a href="#" class="famchip" data-fk="${esc(f.kind + ':' + f.key)}" style="--fc:${famColorMap[f.kind + ':' + f.key]}" title="${esc(f.note || '')}"><b>${esc(f.key)}</b> ${f.names.length}</a>`).join('') || '<span class="muted small">尚未生成，点上方按钮让 GLM 按内容聚类并固化</span>'}
    </div>
    <div id="graphbox"></div>
    <p class="muted small">节点 = skill（颜色=所属家族，大小随版本数），点图例聚焦家族，双击节点跳详情，可拖拽缩放。展示的为上次固化结果。</p>`;
  document.getElementById('gstat').textContent = `${g.skills.length} 节点 / ${g.edges.length} 边`;

  const gh = document.getElementById('ghidesys');
  gh.checked = localStorage.getItem('skhub_graph_hidesys') !== '0'; // 默认隐藏系统 skill
  const viewData = () => {
    if (!gh.checked) return g;
    const sysIDs = new Set(g.skills.filter(s => s.is_system).map(s => s.id));
    if (!sysIDs.size) return g;
    return {
      skills: g.skills.filter(s => !s.is_system),
      edges: g.edges.filter(e => !sysIDs.has(e.src_id) && !sysIDs.has(e.dst_id)),
    };
  };

  const draw = () => {
    if (typeof vis === 'undefined') {
      document.getElementById('graphbox').innerHTML = '<div class="loading" style="padding-top:280px">vis-network CDN 加载失败，检查网络后刷新</div>';
      return;
    }
    const gv = viewData();
    document.getElementById('gstat').textContent = `${gv.skills.length} 节点 / ${gv.edges.length} 边${gh.checked && g.skills.length !== gv.skills.length ? `（已隐藏 ${g.skills.length - gv.skills.length} 个系统 skill）` : ''}`;
    const nodes = new vis.DataSet(gv.skills.map(s => {
      const fc = nameFam[s.name] || '#7ba7e0';
      return ({
      id: s.id, label: s.name, title: esc(`${s.name}\n${s.machine} · ${s.version_count} 版本\n${(s.description || '').slice(0, 100)}`), // vis title 按 HTML 渲染必须转义
      value: Math.max(1, s.version_count),
      color: { background: fc, border: fc, highlight: { background: fc, border: '#1f2430' } },
      font: { color: '#3d4658' },
    });
    }));
    const colorMap = { reference: '#148a4c', related: '#3565e0', chain: '#8b5cf6' };
    const dashMap = { reference: false, related: true, chain: false };
    const edges = new vis.DataSet(gv.edges.map(e => ({
      from: e.src_id, to: e.dst_id,
      color: { color: colorMap[e.kind] || '#999' },
      dashes: dashMap[e.kind] || false,
      title: esc(`${e.kind}${e.note ? ': ' + e.note : ''}`),
      arrows: 'to', font: { size: 10 },
    })));
    const net = new vis.Network(document.getElementById('graphbox'),
      { nodes, edges },
      {
        physics: { solver: 'forceAtlas2Based', forceAtlas2Based: { gravitationalConstant: -60, springLength: 110 }, stabilization: { iterations: 220 } },
        nodes: { shape: 'dot', scaling: { min: 10, max: 34 }, font: { size: 13 } },
        edges: { smooth: { type: 'continuous' } },
        interaction: { hover: true, tooltipDelay: 120 },
      });
    net.on('doubleClick', p => { if (p.nodes.length) location.hash = `#/skill/${p.nodes[0]}`; });
    window._net = net;
  };
  draw();
  gh.addEventListener('change', () => {
    localStorage.setItem('skhub_graph_hidesys', gh.checked ? '1' : '0');
    draw();
  });
  document.querySelectorAll('.famchip[data-fk]').forEach(chip => {
    chip.onclick = ev => {
      ev.preventDefault();
      const fam = families.find(f => f.kind + ':' + f.key === chip.dataset.fk);
      if (!fam || typeof vis === 'undefined' || !window._net) return;
      const names = new Set(fam.names);
      const gv = viewData();
      const ids = gv.skills.filter(s => names.has(s.name)).map(s => s.id);
      const net = window._net;
      if (chip.classList.contains('on')) {
        net.unselectAll(); chip.classList.remove('on'); net.fit({ animation: true });
      } else {
        document.querySelectorAll('.famchip.on').forEach(c => c.classList.remove('on'));
        chip.classList.add('on');
        net.selectNodes(ids, { highlightEdges: false });
        if (ids.length) net.fit({ nodes: ids });
      }
    };
  });

  document.getElementById('rescan').onclick = async () => {
    const btn = document.getElementById('rescan');
    btn.disabled = true; btn.textContent = '重构中（引用边秒级；GLM 关系+家族约 1~3 分钟）…';
    try {
      const r = await apiPost('/edges/scan');
      const parts = [`引用边 ${r.reference_edges} 条`];
      if (r.llm_edges != null) parts.push(`关系边 ${r.llm_edges} 条`);
      if (r.families != null) parts.push(`家族 ${r.families} 个`);
      let err = '';
      if (r.llm_error) err += ` 关系: ${r.llm_error};`;
      if (r.families_error) err += ` 家族: ${r.families_error};`;
      toast(parts.join('，') + (err ? '（' + err.trim() + '）' : ''));
      pageGraph();
    } catch (e) { toast(e.message, true); }
    btn.disabled = false; btn.textContent = '更新关系（引用边 + GLM 关系/家族重构，固化）';
  };
}

/* ---------- 设备管理 ---------- */
async function pageDevices() {
  $app.innerHTML = '<div class="loading">加载中…</div>';
  const [machines, allSkills, dispatches] = await Promise.all([
    api('/machines'), api('/skills'), api('/dispatches/list').catch(() => []),
  ]);
  // 按 (name, machine) 去重出可选 skill 实体
  const uniqueSkills = [];
  const seen = new Set();
  for (const s of allSkills) {
    const k = s.name + '@' + s.machine;
    if (seen.has(k)) continue;
    seen.add(k);
    uniqueSkills.push(s);
  }
  $app.innerHTML = `
    <h1>设备管理</h1>
    <div class="panel" style="border-left:4px solid var(--accent)">
      <b class="small">说明</b>
      <p class="small muted" style="margin:6px 0">
        设备每 30 分钟心跳一次（daemon 客户端），超 35 分钟无心跳/推送判离线。
        为设备分配 skill 后，可点「下发部署」让设备在下次心跳（或客户端手动同步）时自动把分配的 skill 拉取落盘到本地 skill 目录。
        客户端托盘菜单也可主动「立即同步 / 上传本机全部 / 下载云端分配」。命令行：<code class="inline">skill-hub pull --assigned --dest ~/.agents/skills</code>。
      </p>
    </div>
    <div id="devlist"><div class="loading">加载中…</div></div>
    <h2>下发指令历史（最近 50 条）</h2>
    <table class="tbl"><thead><tr><th>#</th><th>设备</th><th>动作</th><th>状态</th><th>结果</th><th>创建</th><th>完成</th></tr></thead>
    <tbody>
    ${(dispatches || []).map(d => `<tr>
      <td>#${d.id}</td><td><span class="badge b-machine">${esc(d.machine)}</span></td>
      <td>${d.action === 'sync' ? '同步（推送+合并+部署）' : '拉取部署'}</td>
      <td><span class="badge b-${d.status === 'done' ? 'push' : d.status === 'pending' ? 'open' : d.status === 'running' ? 'machine' : 'exact'}">${esc(d.status)}</span></td>
      <td class="small muted" style="max-width:280px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="${esc(d.result)}">${esc(d.result || '—')}</td>
      <td class="muted small">${esc(fmtTime(d.created_at))}</td>
      <td class="muted small">${esc(fmtTime(d.finished_at))}</td>
    </tr>`).join('') || '<tr><td colspan="7" class="muted">暂无指令。在设备卡片上点「下发部署」开始。</td></tr>'}
    </tbody></table>`;
  const box = document.getElementById('devlist');
  box.innerHTML = machines.map(m => `
    <div class="panel" data-host="${esc(m.hostname)}">
      <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap">
        <b>${esc(m.hostname)}</b>
        <span class="badge ${m.online ? 'b-push' : 'b-exact'}">${m.online ? '● 在线' : '○ 离线'}</span>
        <span class="muted small">最后活跃 ${esc(fmtTime(m.last_seen))} · ${m.skill_count} 个 skill · ${m.push_count} 次推送</span>
        <span style="flex:1"></span>
        <button class="primary" onclick="window._dispatch('${esc(m.hostname)}','sync')">⬇ 下发部署（同步+部署分配 skill）</button>
        <button onclick="window._dispatch('${esc(m.hostname)}','pull')">仅部署分配 skill</button>
        <button onclick="window._editAssign('${esc(m.hostname)}')">分配 skill…</button>
      </div>
      <div class="assign-zone" id="az-${esc(m.hostname)}"></div>
    </div>`).join('') || '<div class="panel muted">尚无设备接入。新机器接入命令见「设置」页。</div>';

  window._dispatch = async (host, action) => {
    try {
      const r = await apiPost('/dispatches', {machine: host, action});
      toast(`已下发指令 #${r.id}（${action}）——设备将在下次心跳（≤30min）执行；客户端也可点托盘「立即同步」即时执行`);
      setTimeout(() => pageDevices(), 800);
    } catch (e) { toast(e.message, true); }
  };

  window._editAssign = async host => {
    const zone = document.getElementById('az-' + host);
    if (zone.innerHTML) { zone.innerHTML = ''; return; }
    zone.innerHTML = '<div class="loading">加载分配…</div>';
    let assigned = [];
    try { assigned = await api('/assignments?machine=' + encodeURIComponent(host)); } catch (e) {}
    const assignedIDs = new Set(assigned.map(s => s.id));
    zone.innerHTML = `
      <div style="margin-top:10px;border-top:1px solid var(--line);padding-top:10px">
        <b class="small">分配给 ${esc(host)} 的 skill（勾选后保存；部署时覆盖落盘到设备 skill 目录）</b>
        <div style="max-height:260px;overflow:auto;margin:8px 0;border:1px solid var(--line);border-radius:8px;padding:8px">
          ${uniqueSkills.map(s => `<label class="small" style="display:block;padding:2px 0">
            <input type="checkbox" class="assign-ck" data-id="${s.id}" ${assignedIDs.has(s.id) ? 'checked' : ''}>
            <b>${esc(s.name)}</b> <span class="badge b-machine">${esc(s.machine)}</span>
            <span class="muted">${esc((s.description || '').slice(0, 60))}</span></label>`).join('') || '<span class="muted">hub 上无 skill</span>'}
        </div>
        <div class="toolbar">
          <button class="primary" onclick="window._saveAssign('${esc(host)}')">保存分配</button>
          <span class="muted small" id="asgmsg-${esc(host)}"></span>
        </div>
      </div>`;
  };

  window._saveAssign = async host => {
    const ids = [...document.querySelectorAll(`#az-${CSS.escape(host)} .assign-ck:checked`)].map(ck => +ck.dataset.id);
    try {
      const r = await apiPut('/assignments/' + encodeURIComponent(host), {skill_ids: ids});
      document.getElementById('asgmsg-' + host).textContent = `已保存 ${r.count} 个分配`;
      toast(`已保存 ${host} 的 ${r.count} 个 skill 分配`);
    } catch (e) { toast(e.message, true); }
  };
}

/* ---------- 去重工作台 ---------- */
async function pageDedup() {
  $app.innerHTML = '<div class="loading">加载中…</div>';
  const groups = await api('/dup/groups?status=all');
  const open = groups.filter(g => g.status === 'open');
  const done = groups.filter(g => g.status !== 'open');
  $app.innerHTML = `
    <h1>去重与合并工作台</h1>
    <div class="toolbar">
      <button class="primary" id="scanbtn">执行去重扫描</button>
      <span class="muted small">精确哈希分组 + 名称/描述相似度预筛（GLM 配置后自动逐对判定）</span>
    </div>
    <div id="dupbox"></div>`;
  const box = document.getElementById('dupbox');
  const renderGroups = (gs, title) => {
    if (!gs.length) return '';
    return `<h2>${title}</h2>` + gs.map(g => `
      <div class="panel" data-gid="${g.id}">
        <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap">
          <b>组 #${g.id}</b>
          <span class="badge b-${esc(g.type)}">${g.type === 'exact' ? '精确重复' : '近似变体'}</span>
          <span class="badge b-${esc(g.status)}">${g.status === 'open' ? '待处理' : g.status === 'merged' ? '已合并' : '已忽略'}</span>
          <span class="muted small">相似度 ${(g.similarity * 100).toFixed(0)}%</span>
        </div>
        ${g.reason ? `<p class="small muted" style="margin:6px 0">${esc(g.reason)}</p>` : ''}
        <table class="tbl"><tbody>
        ${g.members.map(m => `<tr>
          <td style="width:220px"><a href="#/skill/${m.id}"><b>${esc(m.name)}</b></a></td>
          <td class="desc-cell">${esc((m.description || '').slice(0, 90))}</td>
          <td><span class="badge b-machine">${esc(m.machine)}</span></td>
          <td class="mono small muted">v${m.latest_version_id} · ${esc(m.tree_hash.slice(0, 10))}</td>
        </tr>`).join('')}
        </tbody></table>
        ${g.status === 'open' ? `
        <div class="toolbar" style="margin-top:10px">
          <button class="primary" onclick="window._mergeProp(${g.id})">GLM 生成合并建议</button>
          <button onclick="window._dismiss(${g.id})">忽略此组</button>
        </div>
        <div id="mp-${g.id}"></div>` : ''}
      </div>`).join('');
  };
  box.innerHTML = renderGroups(open, `待处理 (${open.length})`) + renderGroups(done, `已处理 (${done.length})`) ||
    '<div class="panel muted">暂无重复组。点击上方「执行去重扫描」开始分析。</div>';

  document.getElementById('scanbtn').onclick = async () => {
    const btn = document.getElementById('scanbtn');
    btn.disabled = true; btn.textContent = '扫描中…';
    try {
      const r = await apiPost('/dup/scan');
      toast(`精确组 +${r.exact_groups ?? 0}` + (r.fuzzy_groups != null ? `，近似组 +${r.fuzzy_groups}` : `（GLM: ${r.fuzzy_error ?? '未配置'}）`));
      pageDedup();
    } catch (e) { toast(e.message, true); btn.disabled = false; btn.textContent = '执行去重扫描'; }
  };

  window._dismiss = async gid => {
    try { await apiPost(`/dup/groups/${gid}/dismiss`); pageDedup(); } catch (e) { toast(e.message, true); }
  };
  window._mergeProp = async gid => {
    const box = document.getElementById(`mp-${gid}`);
    box.innerHTML = '<div class="loading">GLM 生成中（可能需 30~90 秒）…</div>';
    try {
      const p = await apiPost(`/dup/groups/${gid}/merge-proposal`);
      window._proposals = window._proposals || {};
      window._proposals[gid] = p;
      box.innerHTML = `
        <div class="panel" style="background:#fffdf5">
          <b>合并建议预览</b> <span class="muted small">${esc(p.notes || '')}</span>
          <p class="small muted" style="margin:6px 0">新 description: ${esc(p.description)}</p>
          <div class="merge-cols">
            <div><b class="small">合并后 SKILL.md</b><pre class="code" style="max-height:380px">${esc(p.skill_md)}</pre></div>
            <div><b class="small">与组内首个成员对比</b><div id="mdiff-${gid}" class="loading">计算 diff…</div></div>
          </div>
          <div class="toolbar" style="margin-top:12px">
            <button class="primary" onclick="window._adopt(${gid})">采纳（落为新版本，打 merged 标记）</button>
            <button onclick="document.getElementById('mp-${gid}').innerHTML=''">取消</button>
          </div>
        </div>`;
      // 与组内第一成员的最新版对比
      const groups2 = await api('/dup/groups?status=all');
      const grp = groups2.find(x => x.id === gid);
      if (grp && grp.members.length) {
        const first = grp.members[0];
        const r = await authFetch(`/api/skills/${first.id}/file?version=${first.latest_version_id}&path=SKILL.md`);
        const oldMd = await r.text();
        document.getElementById(`mdiff-${gid}`).innerHTML = renderDiff(simpleDiff(oldMd, p.skill_md, first.name));
      }
    } catch (e) { box.innerHTML = `<div class="panel" style="color:var(--danger)">失败: ${esc(e.message)}</div>`; }
  };
  window._adopt = async gid => {
    try {
      const p = (window._proposals || {})[gid];
      if (!p) { toast('请先生成合并建议', true); return; }
      const r = await apiPost(`/dup/groups/${gid}/adopt`, { skill_md: p.skill_md, description: p.description, notes: p.notes });
      delete window._proposals[gid];
      toast(`已落为新版本 v${r.version_id}`);
      pageDedup();
    } catch (e) { toast(e.message, true); }
  };
}

// 客户端简易 diff（仅用于合并预览的并排示意）
function simpleDiff(a, b, name) {
  const al = a.split('\n'), bl = b.split('\n');
  const out = [];
  const n = Math.max(al.length, bl.length);
  for (let i = 0; i < n; i++) {
    if (al[i] === bl[i]) out.push(' ' + (al[i] ?? ''));
    else {
      if (al[i] !== undefined) out.push('-' + al[i]);
      if (bl[i] !== undefined) out.push('+' + bl[i]);
    }
  }
  return `--- ${name}@旧版\n+++ ${name}@合并建议\n` + out.join('\n');
}

/* ---------- 设置 ---------- */
async function pageSettings() {
  $app.innerHTML = '<div class="loading">加载中…</div>';
  const s = await api('/settings');
  $app.innerHTML = `
    <h1>设置</h1>
    <div class="panel" style="border-left:4px solid var(--accent)">
      <h2 style="margin-top:0">一键接入（免命令行，推荐）</h2>
      <div class="toolbar">
        <button class="primary" onclick="window._dlClient('skill-hub.exe')">⬇ Windows 客户端（双击即用）</button>
        <button onclick="window._dlClient('skill-hub')">⬇ Linux 客户端</button>
        <button onclick="window._dlClient('skill-hub-darwin-arm64')">⬇ macOS (Apple)</button>
        <button onclick="window._dlClient('skill-hub-darwin-amd64')">⬇ macOS (Intel)</button>
      </div>
      <p class="small muted">下载的二进制<b>已内嵌本 Hub 地址与接入配置</b>：放到任意目录，Windows 直接双击 — 托盘出现蓝色图标（右键：打开 Hub / 立即同步 / 退出），自动全盘推送本机 skill、增量同步、应用云端合并、接收远程调用。Linux/macOS：<code class="inline">chmod +x skill-hub && ./skill-hub</code>。<b>开机自启</b>：Win+R 输入 <code class="inline">shell:startup</code>，给 exe 创建快捷方式放进去。配置同时落在 <code class="inline">~/.skill-hub/config.json</code> 可查看。</p>
    </div>
    <div class="panel">
      <h2 style="margin-top:0">接入 Token（管理员主凭据）</h2>
      <p class="small muted">管理员 token：可执行一切操作（远程调用下发、去重合并、GLM 重构、创建用户 token）。</p>
      <div class="toolbar">
        <input type="text" readonly value="${esc(s.token)}" style="min-width:420px;font-family:var(--mono)" id="tokinput">
        <button onclick="navigator.clipboard.writeText(document.getElementById('tokinput').value).then(()=>toast('已复制'))">复制</button>
      </div>
      <h2>用户 Token 管理</h2>
      <p class="small muted">用户 token：仅能同步 skill 数据（push/pull/读取），<b>不能下发远程调用任务</b>；其客户端也不会领取执行任务。新 token 只显示一次。</p>
      <div class="toolbar">
        <input type="text" id="newtoknote" placeholder="备注（如：VM1）" style="min-width:160px">
        <button class="primary" id="mkuser">创建用户 token</button>
        <span id="newtokout" class="mono small"></span>
      </div>
      <table class="tbl" id="toktbl"><thead><tr><th>Token</th><th>角色</th><th>备注</th><th>创建时间</th><th></th></tr></thead><tbody></tbody></table>
    </div>
    <div class="panel">
      <h2 style="margin-top:0">GLM 接入</h2>
      <div class="kv">
        <div class="k">状态</div><div>${s.glm.enabled ? '<span class="badge b-push">已配置</span>' : '<span class="badge b-exact">未配置</span> <span class="muted small">去重判定/合并建议/关系分析将降级不可用</span>'}</div>
        <div class="k">Base URL</div><div class="mono">${esc(s.glm.base_url)}</div>
        <div class="k">模型</div><div class="mono">${esc(s.glm.model)}</div>
      </div>
      <div class="toolbar" style="margin-top:12px">
        <button class="primary" id="glmtest" ${s.glm.enabled ? '' : 'disabled'}>测试连通性</button>
        <span id="glmres" class="small"></span>
      </div>
      <p class="small muted">key 从 <code class="inline">config.yaml</code>（与二进制同目录）或环境变量 <code class="inline">GLM_API_KEY</code>/<code class="inline">ZHIPUAI_API_KEY</code> 读取，env 优先。</p>
    </div>
    <div class="panel">
      <h2 style="margin-top:0">新机器接入（复制即用）</h2>
      <div class="toolbar">
        <span class="small muted">机器名（留空=自动取主机名）</span>
        <input type="text" id="bsHost" placeholder="如 my-laptop" style="min-width:200px">
      </div>
      <p class="small muted">在目标机器终端粘贴执行即可：自动下载二进制到 <code class="inline">~/.skill-hub/</code>，<b>全盘收集</b>本机所有 agent 工具的 skill（.agents / .claude / .zcode 插件缓存等，存在的才推）并上传。重复执行安全（幂等）。</p>
      <b class="small">Linux / macOS / Windows Git Bash（一条命令）</b>
      <div class="toolbar">
        <input type="text" readonly id="cmdUnix" class="mono" style="min-width:620px">
        <button onclick="window._copyCmd('cmdUnix')">复制</button>
      </div>
      <b class="small">Windows（PowerShell）</b>
      <div class="toolbar">
        <input type="text" readonly id="cmdWin" class="mono" style="min-width:620px">
        <button onclick="window._copyCmd('cmdWin')">复制</button>
      </div>
      <b class="small">反向分发（任意已接入机器，把 hub 合并成果拉回本地）</b>
      <div class="toolbar">
        <input type="text" readonly id="cmdPull" class="mono" style="min-width:620px">
        <button onclick="window._copyCmd('cmdPull')">复制</button>
      </div>
      <b class="small">本机常驻客户端（增量监控推送 + 自动应用云端合并 + 接收远程调用）</b>
      <div class="toolbar">
        <input type="text" readonly id="cmdDaemon" class="mono" style="min-width:620px">
        <button onclick="window._copyCmd('cmdDaemon')">复制</button>
      </div>
      <p class="small muted">在目标机器终端执行即可常驻（加 nohup 或做成开机任务更佳）；执行后该机器的 skill 变更 30 秒内自动上云、hub 合并结果自动落回本地、可从本页远程调用其 skill。</p>
    </div>`;
  // 已配置客户端下载：服务端把 server/token 追加进二进制尾部
  // 用户 token 管理
  const loadTokens = async () => {
    try {
      const ts = await api('/tokens');
      const tb = document.querySelector('#toktbl tbody');
      if (!tb) return;
      tb.innerHTML = ts.map(t => `<tr>
        <td class="mono small">${esc(t.token_masked)}</td>
        <td><span class="badge ${t.role === 'admin' ? 'b-merged' : 'b-machine'}">${esc(t.role)}</span></td>
        <td>${esc(t.note || '')}</td>
        <td class="muted small">${esc(fmtTime(t.created_at))}</td>
        <td><button onclick="window._revokeTok('${esc(t.token_masked)}')">吊销</button></td>
      </tr>`).join('') || '<tr><td colspan="5" class="muted small">暂无角色 token（现有各客户端用主 token）</td></tr>';
    } catch (e) { /* 非 admin 忽略 */ }
  };
  loadTokens();
  const mkBtn = document.getElementById('mkuser');
  if (mkBtn) mkBtn.onclick = async () => {
    const note = document.getElementById('newtoknote').value.trim();
    try {
      const r = await apiPost('/tokens/create', { role: 'user', note });
      document.getElementById('newtokout').innerHTML = `<span style="color:var(--ok)">新 token（仅此一次显示）：</span><input readonly value="${esc(r.token)}" style="width:340px;font-family:var(--mono)">`;
      loadTokens();
    } catch (e) { toast(e.message, true); }
  };
  window._revokeTok = async masked => {
    if (!confirm('吊销该 token？对应客户端将立即失去访问。')) return;
    try { await apiPost('/tokens/revoke', { token: masked }); loadTokens(); toast('已吊销'); }
    catch (e) { toast(e.message, true); }
  };
  window._dlClient = async name => {
    try {
      toast('正在生成已配置客户端…');
      const r = await fetch('/api/client/' + name, {headers: signHeaders('GET', '/api/client/' + name, '')});
      if (r.status === 401) { const t = await askToken(); if (!t) return; return window._dlClient(name); }
      if (!r.ok) throw new Error('HTTP ' + r.status);
      const blob = await r.blob();
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = name;
      a.click();
      URL.revokeObjectURL(a.href);
      toast('已下载（含配置）。Windows 双击即用；macOS/Linux 先 chmod +x');
    } catch (e) { toast(e.message, true); }
  };
  // 一键引导命令构建（token 已在本页，直接注入命令文本）
  const buildCmds = () => {
    const hub = location.origin;
    const hn = (document.getElementById('bsHost').value || '').trim();
    const hnU = hn ? ' --hostname ' + hn : '';
    document.getElementById('cmdUnix').value =
      `curl -fsSL ${hub}/agent.sh | sh -s -- --server ${hub} --token ${s.token}${hnU}`;
    const hnW = hn ? ` -Hostname '${hn}'` : '';
    document.getElementById('cmdWin').value =
      `powershell -NoProfile -ExecutionPolicy Bypass -Command "iwr '${hub}/agent.ps1' -OutFile $env:TEMP\\sh-agent.ps1; & $env:TEMP\\sh-agent.ps1 -Server '${hub}' -Token '${s.token}'${hnW}"`;
    document.getElementById('cmdPull').value =
      `~/.skill-hub/skill-hub pull --server ${hub} --token ${s.token} --dest ~/.agents/skills`;
    document.getElementById('cmdDaemon').value =
      `nohup ~/.skill-hub/skill-hub daemon --server ${hub} --token ${s.token}${hnU} > ~/.skill-hub/daemon.log 2>&1 &`;
  };
  buildCmds();
  document.getElementById('bsHost').addEventListener('input', buildCmds);
  window._copyCmd = id => {
    const el = document.getElementById(id);
    el.select();
    navigator.clipboard.writeText(el.value).then(() => toast('已复制，去目标机器终端粘贴执行')).catch(() => {
      document.execCommand('copy'); toast('已复制');
    });
  };
  const btn = document.getElementById('glmtest');
  if (btn) btn.onclick = async () => {
    document.getElementById('glmres').textContent = '测试中…';
    try {
      const r = await apiPost('/settings/glm-test');
      document.getElementById('glmres').innerHTML = `<span style="color:var(--ok)">✓ 连通正常，模型回复: ${esc(r.reply)}</span>`;
    } catch (e) {
      document.getElementById('glmres').innerHTML = `<span style="color:var(--danger)">✗ ${esc(e.message)}</span>`;
    }
  };
}


/* ---------- 远程调用 ---------- */
async function pageJobs() {
  $app.innerHTML = '<div class="loading">加载中…</div>';
  const [machines, skills, jobs] = await Promise.all([api('/machines'), api('/skills'), api('/jobs')]);
  const skillNames = [...new Set(skills.map(s => s.name))];
  $app.innerHTML = `
    <h1>远程调用</h1>
    <div class="panel">
      <h2 style="margin-top:0">下发任务（目标机器需运行常驻客户端，见设置页）</h2>
      <div class="toolbar">
        <select id="jmachine">${machines.map(m => `<option value="${esc(m.hostname)}">${esc(m.hostname)}</option>`).join('')}</select>
        <input type="text" id="jskill" list="skilllist" placeholder="skill 名（可空）" style="min-width:220px">
        <datalist id="skilllist">${skillNames.map(n => `<option value="${esc(n)}">`).join('')}</datalist>
      </div>
      <textarea id="jprompt" placeholder="任务提示词…（将交给目标机器的 claude/zcode -p 进程执行）" style="width:100%;min-height:88px;border:1px solid var(--line);border-radius:8px;padding:9px 11px;font:inherit"></textarea>
      <div class="toolbar" style="margin-top:10px">
        <button class="primary" id="jsubmit">下发</button>
        <span class="small muted" id="jmsg"></span>
      </div>
    </div>
    <h2>任务列表</h2>
    <table class="tbl"><thead><tr><th>#</th><th>机器</th><th>skill</th><th>状态</th><th>CLI</th><th>创建</th><th>完成</th></tr></thead>
    <tbody id="jbody">${renderJobRows(jobs)}</tbody></table>
    <div id="jdetail"></div>`;

  document.getElementById('jsubmit').onclick = async () => {
    const machine = document.getElementById('jmachine').value;
    const skill = document.getElementById('jskill').value.trim();
    const prompt = document.getElementById('jprompt').value.trim();
    if (!prompt) { toast('提示词不能为空', true); return; }
    try {
      const r = await apiPost('/jobs', {machine, skill, prompt});
      document.getElementById('jmsg').textContent = `已下发任务 #${r.id}`;
      refreshJobs();
    } catch (e) { toast(e.message, true); }
  };
  $app._jobsTimer = setInterval(refreshJobs, 5000);

  async function refreshJobs() {
    try {
      const jobs = await api('/jobs');
      document.getElementById('jbody').innerHTML = renderJobRows(jobs);
    } catch (e) { /* ignore */ }
  }
  function renderJobRows(js) {
    return (js || []).map(j => `<tr onclick="window._jobDetail(${j.id})" style="cursor:pointer">
      <td>#${j.id}</td><td><span class="badge b-machine">${esc(j.machine)}</span></td>
      <td>${esc(j.skill || '—')}</td>
      <td><span class="badge b-${j.status === 'ok' || j.status === 'done' ? 'push' : j.status === 'pending' ? 'open' : j.status === 'running' ? 'machine' : 'exact'}">${esc(j.status)}</span></td>
      <td class="mono small">${esc(j.agent_cli || '—')}</td>
      <td class="muted small">${esc(fmtTime(j.created_at))}</td>
      <td class="muted small">${esc(fmtTime(j.finished_at))}</td></tr>`).join('') ||
      '<tr><td colspan="7" class="muted">暂无任务</td></tr>';
  }
}

window._jobDetail = async id => {
  const box = document.getElementById('jdetail');
  box.innerHTML = '<div class="loading">加载报告…</div>';
  try {
    const j = await api(`/jobs/${id}`);
    let rep = {};
    try { rep = JSON.parse(j.result || '{}'); } catch (e) {}
    box.innerHTML = `
      <h2>任务 #${j.id} 报告 <span class="badge b-machine">${esc(j.machine)}</span> <span class="small muted">skill: ${esc(j.skill || '—')} · CLI: ${esc(j.agent_cli || '—')}</span></h2>
      <div class="panel">
        <b class="small">提示词</b>
        <pre class="code" style="max-height:140px">${esc(j.prompt)}</pre>
        <b class="small">asdste100 报告</b>
        <pre class="code" style="max-height:220px">${esc(JSON.stringify(rep, null, 2))}</pre>
        ${rep.output ? `<b class="small">执行输出</b><pre class="code" style="max-height:420px">${esc(rep.output)}</pre>` : ''}
      </div>`;
    box.scrollIntoView({behavior: 'smooth'});
  } catch (e) { box.innerHTML = `<div class="panel" style="color:var(--danger)">${esc(e.message)}</div>`; }
};

route();
