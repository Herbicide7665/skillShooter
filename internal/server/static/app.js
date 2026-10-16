/* SkillShooter SPA */
const $app = document.getElementById('app');
const $nav = document.querySelectorAll('#navlinks a');

/* ---- i18n：中英双语，localStorage 记忆 ---- */
const I18N = {
  zh: {
    nav_dashboard: '总览', nav_graph: '关系图', nav_dedup: '去重合并', nav_devices: '设备',
    nav_jobs: '远程调用', nav_settings: '设置',
    login_title: 'SkillShooter 登录', login_user: '用户名', login_pass: '密码',
    login_btn: '登 录', login_need: '请输入用户名和密码',
    role_admin: '管理员', role_user: '用户', logout: '退出',
    loading: '加载中…',
    dash_title: '总览', dash_skillcount: 'Skill 总数', dash_vercount: '版本总数', dash_machines: '接入机器',
    dash_dupopen: '待处理重复组', dash_edges: '关联边', dash_blob: '内容存储',
    dash_families: 'skill 家族（按命名聚类）', dash_families_empty: '尚未生成。到「关系图」页点「更新关系」，LLM 会按内容聚类并固化。',
    dash_onlinestatus: '设备在线状态（30min 心跳探活，超 35min 判离线）', dash_recent: '最近变更流',
    dash_online: '● 在线', dash_offline: '○ 离线', dash_lastactive: '最后活跃',
    dash_skilln: 'skill 数', dash_pushn: '推送次数', dash_monline: '来源机器', dash_type: '类型', dash_version: '版本', dash_time: '时间', dash_skill: 'Skill',
    dash_empty_recent: '暂无记录，先用 skillshooter push 推送数据', dash_no_machines: '尚无设备接入',
    skills_title: 'Skills', skills_search: '搜索名称或描述…', skills_allmachines: '全部机器', skills_allfams: '全部分类',
    skills_hidesys: '隐藏系统自带', skills_hidedup: '隐藏重复', skills_count: '个 skill', skills_noresult: '无匹配结果',
    skills_col_name: '名称', skills_col_desc: '描述', skills_col_machine: '机器', skills_col_version: '版本', skills_col_updated: '更新时间', skills_col_hash: '哈希',
    detail_versions: '版本时间线（演变路线）', detail_files: '文件浏览（可选历史版本）', detail_edges: '关联关系',
    detail_vdiff: '查看 SKILL.md Diff', detail_pickfile: '选择文件查看内容', detail_downtar: '下载此版本 tar.gz',
    detail_issystem: '系统 skill', detail_marksys: '标记为系统 skill', detail_unmark: '取消系统标记',
    detail_outedges: '出边（本 skill 指向）', detail_inedges: '入边（指向本 skill）',
    graph_title: '关系图', graph_rescan: '更新关系（引用边 + LLM 关系/家族重构，固化）', graph_hidesys: '隐藏系统 skill',
    dedup_title: '去重与合并工作台', dedup_scan: '执行去重扫描', dedup_open: '待处理', dedup_done: '已处理', dedup_ignore: '忽略此组',
    dedup_propose: 'GLM 生成合并建议', dedup_adopt: '采纳（落为新版本，打 merged 标记）', dedup_cancel: '取消',
    devices_title: '设备管理', devices_dispatch: '⬇ 下发部署（同步+部署分配 skill）', devices_pull: '仅部署分配 skill',
    devices_assign: '分配 skill…', devices_save: '保存分配', devices_history: '下发指令历史（最近 50 条）',
    jobs_title: '远程调用', jobs_send: '下发', jobs_list: '任务列表', jobs_none: '暂无任务',
    settings_title: '设置', settings_onestep: '一键接入（免命令行，推荐）', settings_token: 'API Token（程序化调用凭据）',
    settings_usertok: '用户 Token 管理', settings_mkuser: '创建用户 token', settings_llm: 'LLM 接入（自定义模型）',
    settings_changepw: '修改我的密码', settings_users: '用户账号管理', settings_newmachine: '新机器接入（复制即用）',
    sys_admin_only: '仅管理员可见',
  },
  en: {
    nav_dashboard: 'Dashboard', nav_graph: 'Graph', nav_dedup: 'Dedup', nav_devices: 'Devices',
    nav_jobs: 'Jobs', nav_settings: 'Settings',
    login_title: 'SkillShooter Sign In', login_user: 'Username', login_pass: 'Password',
    login_btn: 'Sign In', login_need: 'Please enter username and password',
    role_admin: 'Admin', role_user: 'User', logout: 'Sign out',
    loading: 'Loading…',
    dash_title: 'Dashboard', dash_skillcount: 'Skills', dash_vercount: 'Versions', dash_machines: 'Machines',
    dash_dupopen: 'Open dup groups', dash_edges: 'Edges', dash_blob: 'Storage',
    dash_families: 'Skill families (clustered by name)', dash_families_empty: 'Not generated yet. Go to Graph and click "Update relations"; the LLM clusters by content and persists.',
    dash_onlinestatus: 'Device online status (30min heartbeat, offline after 35min silence)', dash_recent: 'Recent changes',
    dash_online: '● Online', dash_offline: '○ Offline', dash_lastactive: 'Last active',
    dash_skilln: 'skills', dash_pushn: 'pushes', dash_monline: 'Machine', dash_type: 'Type', dash_version: 'Version', dash_time: 'Time', dash_skill: 'Skill',
    dash_empty_recent: 'No records yet — push data with skillshooter push first', dash_no_machines: 'No devices connected',
    skills_title: 'Skills', skills_search: 'Search name or description…', skills_allmachines: 'All machines', skills_allfams: 'All families',
    skills_hidesys: 'Hide built-in', skills_hidedup: 'Hide duplicates', skills_count: 'skills', skills_noresult: 'No results',
    skills_col_name: 'Name', skills_col_desc: 'Description', skills_col_machine: 'Machine', skills_col_version: 'Versions', skills_col_updated: 'Updated', skills_col_hash: 'Hash',
    detail_versions: 'Version timeline (evolution)', detail_files: 'File browser (any version)', detail_edges: 'Relations',
    detail_vdiff: 'View SKILL.md Diff', detail_pickfile: 'Pick a file to view', detail_downtar: 'Download this version (tar.gz)',
    detail_issystem: 'System skill', detail_marksys: 'Mark as system', detail_unmark: 'Unmark system',
    detail_outedges: 'Out edges (from this skill)', detail_inedges: 'In edges (to this skill)',
    graph_title: 'Graph', graph_rescan: 'Update relations (reference edges + LLM clustering, persisted)', graph_hidesys: 'Hide system skills',
    dedup_title: 'Dedup & Merge Workbench', dedup_scan: 'Run dedup scan', dedup_open: 'Open', dedup_done: 'Processed', dedup_ignore: 'Dismiss',
    dedup_propose: 'Generate merge proposal (GLM)', dedup_adopt: 'Adopt (new version, merged)', dedup_cancel: 'Cancel',
    devices_title: 'Devices', devices_dispatch: '⬇ Deploy (sync + assigned skills)', devices_pull: 'Deploy assigned only',
    devices_assign: 'Assign skills…', devices_save: 'Save assignment', devices_history: 'Dispatch history (last 50)',
    jobs_title: 'Jobs', jobs_send: 'Dispatch', jobs_list: 'Jobs', jobs_none: 'No jobs yet',
    settings_title: 'Settings', settings_onestep: 'One-click onboarding (recommended)', settings_token: 'API Token (for programmatic use)',
    settings_usertok: 'User tokens', settings_mkuser: 'Create user token', settings_llm: 'LLM (custom model)',
    settings_changepw: 'Change my password', settings_users: 'User accounts', settings_newmachine: 'Onboard a new machine (copy & run)',
    sys_admin_only: 'Admin only',
  },
};
let LANG = localStorage.getItem('sklang') || 'zh';
function t(key) { return (I18N[LANG] && I18N[LANG][key]) || I18N.zh[key] || key; }
// 服务端已知错误串的英文映射（服务端中文为主，客户端按当前语言转显）
const ERR_EN = {
  '用户名或密码错误': 'Wrong username or password',
  '旧密码错误': 'Wrong old password',
  '新密码至少 6 位': 'New password must be at least 6 chars',
  '密码至少 6 位': 'Password must be at least 6 chars',
  '用户名不能为空且密码至少 6 位': 'Username required; password min 6 chars',
  '用户名已存在': 'Username already exists',
  '不能删除最后一个管理员': 'Cannot delete the last admin',
  'token 无效': 'Invalid token',
  '需要管理员权限': 'Admin permission required',
};
function terr(msg) { return LANG === 'en' ? (ERR_EN[msg] || msg) : msg; }
function toggleLang() {
  LANG = LANG === 'zh' ? 'en' : 'zh';
  localStorage.setItem('sklang', LANG);
  applyStaticI18n();
  route();
  if (state.me) applyRoleNav();
}
function applyStaticI18n() {
  document.getElementById('langBtn').textContent = LANG === 'zh' ? 'EN' : '中';
  document.querySelectorAll('[data-i18n]').forEach(el => { el.textContent = t(el.dataset.i18n); });
}


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
/* ---- 会话认证：浏览器用登录 cookie，token 只留给 API 客户端 ---- */
const state = { machines: [], llm: null, me: null };

async function api(path, opts) {
  opts = opts || {};
  opts.method = (opts.method || 'GET').toUpperCase();
  if (opts.body && !opts.headers) opts.headers = {'Content-Type': 'application/json'};
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
  if (r.status === 401 && !opts._retried && path !== '/login') {
    renderLogin();
    throw new Error('未登录或会话已过期');
  }
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(data.error || ('HTTP ' + r.status));
  return data;
}
async function apiPost(path, body) {
  return api(path, {method: 'POST', headers: {'Content-Type': 'application/json'}, body: body ? JSON.stringify(body) : undefined});
}
async function apiPut(path, body) {
  return api(path, {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: body ? JSON.stringify(body) : undefined});
}
// 原始文件请求（正文/下载），同名保留
async function authFetch(url) { return fetch(url); }

/* ---- 登录页 ---- */
function renderLogin() {
  $nav.forEach(a => a.style.display = 'none');
  document.getElementById('userbox').innerHTML = '';
  $app.innerHTML = `
    <div class="login-wrap">
      <div class="panel login-panel">
        <div class="logo">🎯</div>
        <h2 style="margin-top:0">${t('login_title')}</h2>
        <div class="toolbar" style="flex-direction:column;align-items:stretch;gap:8px">
          <input type="text" id="lguser" placeholder="${t('login_user')}" autocomplete="username">
          <input type="password" id="lgpass" placeholder="${t('login_pass')}" autocomplete="current-password">
        </div>
        <div class="toolbar" style="margin-top:10px">
          <button class="primary" id="lgbtn" style="width:100%">${t('login_btn')}</button>
        </div>
        <p class="small muted" id="lgmsg"></p>
      </div>
    </div>`;
  const doLogin = async () => {
    const u = document.getElementById('lguser').value.trim();
    const p = document.getElementById('lgpass').value;
    if (!u || !p) { document.getElementById('lgmsg').textContent = t('login_need'); return; }
    try {
      const r = await fetch('/api/login', {method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({username: u, password: p})});
      const d = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(d.error || ('HTTP ' + r.status));
      state.me = {user: d.user, role: d.role};
      $nav.forEach(a => a.style.display = '');
      applyRoleNav();
      location.hash = '#/dashboard';
      route();
    } catch (e) { document.getElementById('lgmsg').textContent = terr(e.message); }
  };
  document.getElementById('lgbtn').onclick = doLogin;
  for (const id of ['lguser', 'lgpass']) document.getElementById(id).addEventListener('keydown', e => { if (e.key === 'Enter') doLogin(); });
  document.getElementById('lguser').focus();
}

async function doLogout() {
  try { await fetch('/api/logout', {method: 'POST'}); } catch (e) {}
  state.me = null;
  renderLogin();
}

// 按角色显示/隐藏管理入口（非 admin 隐藏 远程调用）
function applyRoleNav() {
  const admin = state.me && state.me.role === 'admin';
  document.querySelectorAll('#navlinks a').forEach(a => {
    if (a.dataset.r === 'jobs') a.style.display = admin ? '' : 'none';
  });
  const ub = document.getElementById('userbox');
  if (state.me) {
    ub.innerHTML = `<span class="small">${esc(state.me.user)} · ${state.me.role === 'admin' ? t('role_admin') : t('role_user')}</span>
      <button style="margin-left:8px;padding:4px 12px" onclick="doLogout()">${t('logout')}</button>`;
  } else {
    ub.innerHTML = '';
  }
}

async function boot() {
  applyStaticI18n();
  try {
    const r = await fetch('/api/whoami');
    const d = await r.json();
    if (d.ok) {
      state.me = {user: d.user || d.role, role: d.role};
      $nav.forEach(a => a.style.display = '');
      applyRoleNav();
      route();
      return;
    }
  } catch (e) {}
  renderLogin();
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
    <div class="page-head"><h1>${t('dash_title')}</h1></div>
    <div class="cards">
      <div class="card"><div class="num">${st.skill_count}</div><div class="lbl">${t('dash_skillcount')}</div></div>
      <div class="card"><div class="num">${st.version_count}</div><div class="lbl">${t('dash_vercount')}</div></div>
      <div class="card"><div class="num">${st.machine_count}</div><div class="lbl">${t('dash_machines')}</div></div>
      <div class="card"><div class="num">${st.open_dup_groups}</div><div class="lbl">${t('dash_dupopen')}</div></div>
      <div class="card"><div class="num">${st.edge_count}</div><div class="lbl">${t('dash_edges')}</div></div>
      <div class="card"><div class="num">${fmtBytes(st.blob_bytes)}</div><div class="lbl">${t('dash_blob')}</div></div>
    </div>
    <h2>${t('dash_families')}</h2>
    <div class="panel" style="display:flex;gap:8px;flex-wrap:wrap" id="famchips">
      ${(await api('/families')).slice(0, 10).map(f => `<a href="#" class="famchip" onclick="sessionStorage.setItem('famPreselect','${esc(f.kind + ':' + esc(f.key))}');location.hash='#/skills';return false"><b>${esc(f.key)}</b> <span class="muted">${f.names.length} 个</span></a>`).join('') || (`<span class="muted small">${t('dash_families_empty')}</span>`)}
    </div>
    <h2>${t('dash_onlinestatus')}</h2>
    <div id="devbox"><div class="loading">加载中…</div></div>
    <h2>${t('dash_recent')}</h2>
    <table class="tbl"><thead><tr><th>${t('dash_time')}</th><th>${t('dash_skill')}</th><th>${t('dash_monline')}</th><th>${t('dash_type')}</th><th>${t('dash_version')}</th></tr></thead><tbody>
    ${st.recent.map(r => `<tr>
      <td class="muted small">${esc(fmtTime(r.created_at))}</td>
      <td><a href="#/skill/${r.skill_id}">${esc(r.name)}</a></td>
      <td><span class="badge b-machine">${esc(r.machine)}</span></td>
      <td><span class="badge b-${r.kind === 'merged' ? 'merged' : 'push'}">${r.kind === 'merged' ? '合并' : '推送'}</span></td>
      <td class="mono small">v${r.version_id}</td>
    </tr>`).join('') || `<tr><td colspan="5" class="muted">${t('dash_empty_recent')}</td></tr>`}
    </tbody></table>`;
  document.getElementById('devbox').innerHTML = `
    <table class="tbl"><thead><tr><th>${t('nav_devices')}</th><th>Status</th><th>${t('dash_lastactive')}</th><th>${t('dash_skilln')}</th><th>${t('dash_pushn')}</th></tr></thead><tbody>
    ${machines.map(m => `<tr>
      <td><b>${esc(m.hostname)}</b></td>
      <td><span class="badge ${m.online ? 'b-push' : 'b-exact'}">${m.online ? t('dash_online') : t('dash_offline')}</span></td>
      <td class="muted small">${esc(fmtTime(m.last_seen))}</td>
      <td>${m.skill_count}</td>
      <td>${m.push_count}</td>
    </tr>`).join('') || `<tr><td colspan="5" class="muted">${t('dash_no_machines')}</td></tr>`}
    </tbody></table>
    <p class="muted small">${LANG === 'zh' ? '管理分配与主动下发请到 <a href="#/devices">设备页</a>。' : 'Manage assignments & dispatch on the <a href="#/devices">Devices</a> page.'}</p>`;
}

/* ---------- Skills 列表 ---------- */
async function pageSkills() {
  if (!state.machines.length) state.machines = await api('/machines');
  const families = await api('/families');
  state.families = families;
  const pre = sessionStorage.getItem('famPreselect');
  $app.innerHTML = `
    <div class="page-head"><h1>${t('skills_title')}</h1></div>
    <div class="toolbar">
      <input type="text" id="q" placeholder="${t('skills_search')}" style="min-width:260px">
      <select id="mfilter"><option value="">${t('skills_allmachines')}</option>
        ${state.machines.map(m => `<option value="${esc(m.hostname)}">${esc(m.hostname)} (${m.skill_count})</option>`).join('')}
      </select>
      <select id="ffilter"><option value="">${t('skills_allfams')}</option>
        ${families.map(f => `<option value="${esc(f.kind + ':' + f.key)}">${esc(f.key)}（${f.names.length}）</option>`).join('')}
      </select>
      <label class="small" style="display:inline-flex;align-items:center;gap:4px"><input type="checkbox" id="hidesys"> ${t('skills_hidesys')}</label>
      <label class="small" style="display:inline-flex;align-items:center;gap:4px"><input type="checkbox" id="hidedup"> ${t('skills_hidedup')}</label>
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
    document.getElementById('cnt').textContent = `${list.length} ${t('skills_count')}`;
    document.getElementById('tblbox').innerHTML = `
      <table class="tbl"><thead><tr><th>${t('skills_col_name')}</th><th>${t('skills_col_desc')}</th><th>${t('skills_col_machine')}</th><th>${t('skills_col_version')}</th><th>${t('skills_col_updated')}</th><th>${t('skills_col_hash')}</th></tr></thead><tbody>
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
      <div class="k">${t('detail_issystem')}</div><div><span id="sysmark" class="badge ${s.is_system ? 'b-push' : 'b-open'}">${s.is_system ? (LANG==='zh'?'是':'Yes') : (LANG==='zh'?'否':'No')}</span>
        <button style="margin-left:8px" onclick="window._toggleSys(${s.id}, ${s.is_system ? 'false' : 'true'})">${s.is_system ? t('detail_unmark') : t('detail_marksys')}</button></div>
    </div>

    <h2>${t('detail_versions')}</h2>
    <div class="panel">
      <ul class="timeline" id="vline"></ul>
      <div class="toolbar" style="margin-top:12px">
        <select id="dfrom"></select><span class="muted">→</span><select id="dto"></select>
        <button class="primary" id="dobtn">${t('detail_vdiff')}</button>
      </div>
      <div id="diffbox"></div>
    </div>

    <h2>${t('detail_files')}</h2>
    <div class="panel" id="filebox"><div class="loading">加载中…</div></div>

    <h2>${t('detail_edges')}</h2>
    <div class="panel">
      <b>${t('detail_outedges')}</b>
      <div class="edge-list">${edgeHTML(d.edges_out, '引用 →') || '<span class="muted small">无</span>'}</div>
      <b>${t('detail_inedges')}</b>
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

  // 文件浏览（版本可选：全部历史版本留档，可回看/下载任意版本）
  const fb = document.getElementById('filebox');
  if (s.latest_version_id) {
    let curTree = {};
    fb.innerHTML = `
      <div class="toolbar">
        <select id="vsel">${d.versions.map(v => `<option value="${v.id}"${v.id === s.latest_version_id ? ' selected' : ''}>v${v.id} · ${fmtTime(v.created_at)} · ${v.kind === 'merged' ? '合并' : '推送'}</option>`).join('')}</select>
        <select id="fpick"></select>
        <span class="muted small" id="fmeta"></span>
        <button onclick="window._dlVersion()">${t('detail_downtar')}</button>
      </div>
      <pre class="code" id="fview">${t('detail_pickfile')}</pre>`;
    const loadTree = async () => {
      const vid = document.getElementById('vsel').value;
      curTree = await api(`/skills/${id}/tree?version=${vid}`);
      document.getElementById('fpick').innerHTML = Object.keys(curTree).map(p => `<option value="${esc(p)}">${esc(p)}</option>`).join('');
      await loadFile();
    };
    const loadFile = async () => {
      const vid = document.getElementById('vsel').value;
      const p = document.getElementById('fpick').value;
      if (!p) return;
      const r = await authFetch(`/api/skills/${id}/file?version=${vid}&path=${encodeURIComponent(p)}`);
      document.getElementById('fview').textContent = await r.text();
      document.getElementById('fmeta').textContent = 'blob ' + (curTree[p] || '').slice(0, 12);
    };
    document.getElementById('vsel').addEventListener('change', loadTree);
    document.getElementById('fpick').addEventListener('change', loadFile);
    window._dlVersion = async () => {
      const vid = document.getElementById('vsel').value;
      const r = await authFetch(`/api/skills/${id}/download?version=${vid}`);
      if (!r.ok) { toast('下载失败: HTTP ' + r.status, true); return; }
      const blob = await r.blob();
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = `${s.name}-v${vid}.tar.gz`;
      a.click();
      URL.revokeObjectURL(a.href);
    };
    await loadTree();
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
    <div class="page-head"><h1>${t('graph_title')}</h1></div>
    <div class="toolbar">
      <button class="primary" id="rescan">更新关系（引用边 + GLM 关系/家族重构，固化）</button>
      <label class="small" style="display:inline-flex;align-items:center;gap:4px"><input type="checkbox" id="ghidesys"> ${t('graph_hidesys')}</label>
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
    <div class="page-head"><h1>${t('devices_title')}</h1></div>
    <div class="panel" style="border-left:4px solid var(--accent)">
      <b class="small">说明</b>
      <p class="small muted" style="margin:6px 0">
        设备每 30 分钟心跳一次（daemon 客户端），超 35 分钟无心跳/推送判离线。
        为设备分配 skill 后，可点「下发部署」让设备在下次心跳（或客户端手动同步）时自动把分配的 skill 拉取落盘到本地 skill 目录。
        客户端托盘菜单也可主动「立即同步 / 上传本机全部 / 下载云端分配」。命令行：<code class="inline">skillshooter pull --assigned --dest ~/.agents/skills</code>。
      </p>
    </div>
    <div id="devlist"><div class="loading">加载中…</div></div>
    <h2>${t('devices_history')}</h2>
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
        <span class="badge ${m.online ? 'b-push' : 'b-exact'}">${m.online ? t('dash_online') : t('dash_offline')}</span>
        <span class="muted small">最后活跃 ${esc(fmtTime(m.last_seen))} · ${m.skill_count} 个 skill · ${m.push_count} 次推送</span>
        <span style="flex:1"></span>
        <button class="primary" onclick="window._dispatch('${esc(m.hostname)}','sync')">${t('devices_dispatch')}</button>
        <button onclick="window._dispatch('${esc(m.hostname)}','pull')">${t('devices_pull')}</button>
        <button onclick="window._editAssign('${esc(m.hostname)}')">${t('devices_assign')}</button>
      </div>
      <div class="assign-zone" id="az-${esc(m.hostname)}"></div>
    </div>`).join('') || `<div class="panel muted">${t('dash_no_machines')}${LANG === 'zh' ? '。新机器接入命令见「设置」页。' : ' — see Settings for onboarding commands.'}</div>`;

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
          <button class="primary" onclick="window._saveAssign('${esc(host)}')">${t('devices_save')}</button>
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
    <div class="page-head"><h1>${t('dedup_title')}</h1></div>
    <div class="toolbar">
      <button class="primary" id="scanbtn">${t('dedup_scan')}</button>
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
          <button class="primary" onclick="window._mergeProp(${g.id})">${t('dedup_propose')}</button>
          <button onclick="window._dismiss(${g.id})">${t('dedup_ignore')}</button>
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
            <button class="primary" onclick="window._adopt(${gid})">${t('dedup_adopt')}</button>
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
  $app.innerHTML = `<div class="loading">${t('loading')}</div>`;
  const s = await api('/settings');
  const admin = state.me && state.me.role === 'admin';
  $app.innerHTML = `
    <div class="page-head"><h1>${t('settings_title')}</h1></div>
    <div class="settings-layout">
      <nav class="settings-side" id="settingsSide">
        ${admin ? `<a href="#set-access" class="on" data-sec="access"><span class="ico">🚀</span>${t('settings_onestep')}</a>` : ''}
        ${admin ? `<a href="#set-token" data-sec="token"><span class="ico">🔑</span>${t('settings_token')}</a>` : ''}
        <a href="#set-llm" data-sec="llm"><span class="ico">🧠</span>${t('settings_llm')}</a>
        <a href="#set-pw" data-sec="pw"><span class="ico">🔒</span>${t('settings_changepw')}</a>
        ${admin ? `<a href="#set-users" data-sec="users"><span class="ico">👥</span>${t('settings_users')}</a>` : ''}
        ${admin ? `<a href="#set-machine" data-sec="machine"><span class="ico">💻</span>${t('settings_newmachine')}</a>` : ''}
      </nav>
      <div class="settings-body">
    ${admin ? `
    <div class="panel" id="set-access" style="border-left:4px solid var(--accent)">
      <h2 style="margin-top:0">${t('settings_onestep')}</h2>
      <div class="toolbar">
        <button class="primary" onclick="window._dlClient('skillshooter.exe')">⬇ Windows</button>
        <button onclick="window._dlClient('skillshooter')">⬇ Linux</button>
        <button onclick="window._dlClient('skillshooter-darwin-arm64')">⬇ macOS (Apple)</button>
        <button onclick="window._dlClient('skillshooter-darwin-amd64')">⬇ macOS (Intel)</button>
      </div>
      <p class="small muted">${LANG === 'zh'
        ? '下载的二进制<b>已内嵌本 Hub 地址与接入配置</b>：放到任意目录，Windows 直接双击 — 托盘出现图标（右键：打开 Hub / 立即同步 / 配置 / 退出），自动全盘推送本机 skill、增量同步、应用云端合并、接收远程调用。Linux/macOS：<code class="inline">chmod +x skillshooter && ./skillshooter</code>。'
        : 'The downloaded binary <b>embeds this Hub URL and credentials</b>: drop it anywhere, double-click on Windows — a tray icon appears (right-click: Open Hub / Sync now / Configure / Quit), auto-pushing local skills, incremental sync, applying cloud merges, receiving jobs. Linux/macOS: <code class="inline">chmod +x skillshooter && ./skillshooter</code>.'}</p>
    </div>
    <div class="panel" id="set-token">
      <h2 style="margin-top:0">${t('settings_token')}</h2>
      <p class="small muted">${LANG === 'zh'
        ? '浏览器已用<b>账号登录</b>，无需 token。token 仅供 API 客户端（push/pull/daemon）使用：管理员 token 可执行一切操作，用户 token 绑定设备（创建可留空=首次使用自动认领），只能同步本机与分配给本机的 skill，管理视图 403。新 token 只显示一次。'
        : 'The browser uses <b>account login</b> — no token needed. Tokens are for API clients: the admin token can do everything; user tokens are device-bound (blank on creation = auto-claimed on first use). Admin views return 403 for user tokens. New tokens shown once.'}</p>
      <div class="toolbar">
        <input type="text" readonly value="${esc(s.token || '')}" style="min-width:420px;font-family:var(--mono)" id="tokinput">
        <button onclick="navigator.clipboard.writeText(document.getElementById('tokinput').value).then(()=>toast('copied'))">${LANG === 'zh' ? '复制主 token' : 'Copy admin token'}</button>
      </div>
      <h2>${t('settings_usertok')}</h2>
      <div class="toolbar">
        <input type="text" id="newtoknote" placeholder="${LANG === 'zh' ? '备注（如：VM1）' : 'Note (e.g. VM1)'}" style="min-width:140px">
        <input type="text" id="newtokhost" placeholder="${LANG === 'zh' ? '绑定设备名（留空=首次使用自动认领）' : 'Device name (blank = auto-claim)'}" style="min-width:240px">
        <button class="primary" id="mkuser">${t('settings_mkuser')}</button>
        <span id="newtokout" class="mono small"></span>
      </div>
      <table class="tbl" id="toktbl"><thead><tr><th>Token</th><th>Role</th><th>Note</th><th>${LANG === 'zh' ? '绑定设备' : 'Device'}</th><th>Created</th><th></th></tr></thead><tbody></tbody></table>
    </div>` : ''}
    <div class="panel" id="set-llm">
      <h2 style="margin-top:0">${t('settings_llm')}</h2>
      <p class="small muted">${LANG === 'zh'
        ? '任何 OpenAI 兼容端点均可（GLM/DeepSeek/OpenAI/OpenRouter/本地 vLLM 等）；base_url 含 <code class="inline">/anthropic</code> 时自动走 Anthropic messages 协议。key 留空表示不修改。优先级：环境变量 &gt; 此处保存 &gt; config.yaml。'
        : 'Any OpenAI-compatible endpoint works (GLM/DeepSeek/OpenAI/OpenRouter/local vLLM…); a base_url containing <code class="inline">/anthropic</code> switches to the Anthropic messages protocol. Leave key blank to keep the current one. Priority: env &gt; saved here &gt; config.yaml.'}</p>
      <div class="kv">
        <div class="k">${LANG === 'zh' ? '状态' : 'Status'}</div><div>${s.llm.enabled ? `<span class="badge b-push">${LANG === 'zh' ? '已配置' : 'Configured'}</span>` : `<span class="badge b-exact">${LANG === 'zh' ? '未配置' : 'Not configured'}</span>`}</div>
      </div>
      ${admin ? `
      <div class="toolbar" style="flex-wrap:wrap;gap:8px;margin-top:10px">
        <input type="text" id="llmkey" placeholder="API Key (${esc(s.llm.key_masked || (LANG === 'zh' ? '未设置' : 'not set'))} · ${LANG === 'zh' ? '留空不改' : 'blank = keep'})" style="min-width:300px;font-family:var(--mono)">
        <input type="text" id="llmbase" placeholder="Base URL" value="${esc(s.llm.base_url || '')}" style="min-width:300px;font-family:var(--mono)">
        <input type="text" id="llmmodel" placeholder="Model" value="${esc(s.llm.model || '')}" style="min-width:220px;font-family:var(--mono)">
        <button class="primary" id="llmsave">${LANG === 'zh' ? '保存' : 'Save'}</button>
        <button id="llmtestbtn" ${s.llm.enabled ? '' : 'disabled'}>${LANG === 'zh' ? '测试连通性' : 'Test'}</button>
        <span id="llmres" class="small"></span>
      </div>` : `
      <div class="kv"><div class="k">Base URL</div><div class="mono">${esc(s.llm.base_url)}</div><div class="k">Model</div><div class="mono">${esc(s.llm.model)}</div></div>`}
    </div>
    <div class="panel" id="set-pw">
      <h2 style="margin-top:0">${t('settings_changepw')}</h2>
      <div class="toolbar" style="flex-wrap:wrap;gap:8px">
        <input type="password" id="pwold" placeholder="${LANG === 'zh' ? '旧密码' : 'Old password'}" style="min-width:180px">
        <input type="password" id="pwnew" placeholder="${LANG === 'zh' ? '新密码（至少 6 位）' : 'New password (min 6 chars)'}" style="min-width:180px">
        <button class="primary" id="pwbtn">${LANG === 'zh' ? '修改密码' : 'Change'}</button>
        <span id="pwmsg" class="small"></span>
      </div>
    </div>
    ${admin ? `
    <div class="panel" id="set-users">
      <h2 style="margin-top:0">${t('settings_users')}</h2>
      <p class="small muted">${LANG === 'zh' ? 'web 后台登录账号（与 API token 相互独立）。admin 角色可管理一切，user 角色只读。' : 'Web console accounts (independent of API tokens). admin manages everything; user is read-only.'}</p>
      <div class="toolbar" style="flex-wrap:wrap;gap:8px">
        <input type="text" id="unew" placeholder="${LANG === 'zh' ? '用户名' : 'Username'}" style="min-width:140px">
        <input type="password" id="upass" placeholder="${LANG === 'zh' ? '密码（≥6 位）' : 'Password (≥6)'}" style="min-width:140px">
        <select id="urole"><option value="user">user</option><option value="admin">admin</option></select>
        <button class="primary" id="ucreate">${LANG === 'zh' ? '创建' : 'Create'}</button>
      </div>
      <table class="tbl" id="usertbl"><thead><tr><th>ID</th><th>${LANG === 'zh' ? '用户名' : 'Username'}</th><th>Role</th><th>${LANG === 'zh' ? '创建时间' : 'Created'}</th><th></th></tr></thead><tbody></tbody></table>
    </div>` : ''}
    ${admin ? `
    <div class="panel" id="set-machine">
      <h2 style="margin-top:0">${t('settings_newmachine')}</h2>
      <div class="toolbar">
        <span class="small muted">${LANG === 'zh' ? '机器名（留空=自动取主机名）' : 'Machine name (blank = hostname)'}</span>
        <input type="text" id="bsHost" placeholder="e.g. my-laptop" style="min-width:200px">
      </div>
      <p class="small muted">${LANG === 'zh' ? '在目标机器终端粘贴执行：自动下载二进制到 ~/.skillshooter/，全盘收集本机 agent 工具的 skill 并上传。幂等可重复。' : 'Paste into the target terminal: downloads the binary to ~/.skillshooter/ and pushes all discovered skills. Idempotent.'}</p>
      <b class="small">Linux / macOS / Windows Git Bash</b>
      <div class="toolbar"><input type="text" readonly id="cmdUnix" class="mono" style="min-width:620px"><button onclick="window._copyCmd('cmdUnix')">${LANG === 'zh' ? '复制' : 'Copy'}</button></div>
      <b class="small">${LANG === 'zh' ? 'Windows（PowerShell）' : 'Windows (PowerShell)'}</b>
      <div class="toolbar"><input type="text" readonly id="cmdWin" class="mono" style="min-width:620px"><button onclick="window._copyCmd('cmdWin')">${LANG === 'zh' ? '复制' : 'Copy'}</button></div>
      <b class="small">${LANG === 'zh' ? '反向分发（拉回合并成果）' : 'Pull merged results back'}</b>
      <div class="toolbar"><input type="text" readonly id="cmdPull" class="mono" style="min-width:620px"><button onclick="window._copyCmd('cmdPull')">${LANG === 'zh' ? '复制' : 'Copy'}</button></div>
      <b class="small">${LANG === 'zh' ? '常驻客户端（增量推送+应用合并+远程调用）' : 'Resident daemon'}</b>
      <div class="toolbar"><input type="text" readonly id="cmdDaemon" class="mono" style="min-width:620px"><button onclick="window._copyCmd('cmdDaemon')">${LANG === 'zh' ? '复制' : 'Copy'}</button></div>
    </div>` : ''}
      </div><!-- /settings-body -->
    </div><!-- /settings-layout -->`;

  // 侧边栏：点击平滑滚动 + 滚动高亮
  const side = document.getElementById('settingsSide');
  if (side) {
    const secs = [...document.querySelectorAll('.settings-body .panel[id]')];
    side.querySelectorAll('a').forEach(a => a.onclick = ev => {
      ev.preventDefault();
      const el = document.getElementById('set-' + a.dataset.sec);
      if (el) el.scrollIntoView({behavior: 'smooth'});
    });
    const pick = () => {
      let cur = secs[0];
      for (const pn of secs) if (pn.getBoundingClientRect().top < 110) cur = pn;
      side.querySelectorAll('a').forEach(a => a.classList.toggle('on', a.dataset.sec === (cur && cur.id || '').replace('set-', '')));
    };
    window.addEventListener('scroll', pick, {passive: true});
    setTimeout(pick, 50);
  }
  if (admin) {
    const loadTokens = async () => {
      try {
        const ts = await api('/tokens');
        const tb = document.querySelector('#toktbl tbody');
        if (!tb) return;
        tb.innerHTML = ts.map(t => `<tr>
          <td class="mono small">${esc(t.token_masked)}</td>
          <td><span class="badge ${t.role === 'admin' ? 'b-merged' : 'b-machine'}">${esc(t.role)}</span></td>
          <td>${esc(t.note || '')}</td>
          <td>${t.hostname ? `<span class="badge b-machine">${esc(t.hostname)}</span>` : `<span class="muted small">${LANG === 'zh' ? '待认领' : 'unclaimed'}</span>`}</td>
          <td class="muted small">${esc(fmtTime(t.created_at))}</td>
          <td><button onclick="window._rebindTok('${esc(t.token_masked)}', '${esc(t.hostname || '')}')">${LANG === 'zh' ? '改绑' : 'Rebind'}</button>
              <button onclick="window._revokeTok('${esc(t.token_masked)}')">${LANG === 'zh' ? '吊销' : 'Revoke'}</button></td>
        </tr>`).join('') || `<tr><td colspan="6" class="muted small">${LANG === 'zh' ? '暂无角色 token（现有各客户端用主 token）' : 'No role tokens yet'}</td></tr>`;
      } catch (e) {}
    };
    loadTokens();
    const mkBtn = document.getElementById('mkuser');
    if (mkBtn) mkBtn.onclick = async () => {
      const note = document.getElementById('newtoknote').value.trim();
      try {
        const r = await apiPost('/tokens/create', { role: 'user', note, hostname: document.getElementById('newtokhost').value.trim() });
        document.getElementById('newtokout').innerHTML = '<span style="color:var(--ok)">' + (LANG === 'zh' ? '新 token（仅此一次显示）：' : 'New token (shown once): ') + '</span><input readonly value="' + esc(r.token) + '" style="width:340px;font-family:var(--mono)">';
        loadTokens();
      } catch (e) { toast(e.message, true); }
    };
    window._rebindTok = async (masked, cur) => {
      const host = prompt('改绑设备（留空=解绑回待认领态）。当前: ' + (cur || '待认领'), cur || '');
      if (host === null) return;
      try { await apiPost('/tokens/rebind', { token: masked, hostname: host.trim() }); toast(host.trim() ? '已改绑 ' + host.trim() : '已解绑（待认领）'); loadTokens(); }
      catch (e) { toast(e.message, true); }
    };
    window._revokeTok = async masked => {
      if (!confirm('吊销该 token？对应客户端将立即失去访问。')) return;
      try { await apiPost('/tokens/revoke', { token: masked }); loadTokens(); toast('已吊销'); }
      catch (e) { toast(e.message, true); }
    };

    const loadUsers = async () => {
      try {
        const us = await api('/users');
        const tb = document.querySelector('#usertbl tbody');
        if (!tb) return;
        const DEL = LANG === 'zh' ? '删除' : 'Delete';
        const RST = LANG === 'zh' ? '重置密码' : 'Reset pw';
        tb.innerHTML = us.map(u => `<tr><td>${u.id}</td><td><b>${esc(u.username)}</b></td>
          <td><span class="badge ${u.role === 'admin' ? 'b-merged' : 'b-machine'}">${esc(u.role)}</span></td>
          <td class="muted small">${esc(fmtTime(u.created_at))}</td>
          <td><button onclick="window._delUser(${u.id}, '${esc(u.username)}')">${DEL}</button>
              <button onclick="window._resetPw(${u.id}, '${esc(u.username)}')">${RST}</button></td></tr>`).join('');
      } catch (e) {}
    };
    loadUsers();
    document.getElementById('ucreate').onclick = async () => {
      const username = document.getElementById('unew').value.trim();
      const password = document.getElementById('upass').value;
      const role = document.getElementById('urole').value;
      try { await apiPost('/users/create', {username, password, role}); toast('用户已创建'); loadUsers(); }
      catch (e) { toast(e.message, true); }
    };
    window._delUser = async (id, name) => {
      if (!confirm('删除用户 ' + name + '？')) return;
      try { await apiPost('/users/delete', {id}); toast('已删除'); loadUsers(); }
      catch (e) { toast(e.message, true); }
    };
    window._resetPw = async (id, name) => {
      const pw = prompt('为 ' + name + ' 设置新密码（≥6 位）');
      if (!pw) return;
      try { await apiPost('/users/password', {id, password: pw}); toast('密码已重置'); }
      catch (e) { toast(e.message, true); }
    };

    const buildCmds = () => {
      const hub = location.origin;
      const hn = (document.getElementById('bsHost').value || '').trim();
      const hnU = hn ? ' --hostname ' + hn : '';
      document.getElementById('cmdUnix').value =
        'curl -fsSL ' + hub + '/agent.sh | sh -s -- --server ' + hub + ' --token ' + s.token + hnU;
      const hnW = hn ? " -Hostname '" + hn + "'" : '';
      document.getElementById('cmdWin').value =
        "powershell -NoProfile -ExecutionPolicy Bypass -Command \"iwr '" + hub + "/agent.ps1' -OutFile $env:TEMP\\sh-agent.ps1; & $env:TEMP\\sh-agent.ps1 -Server '" + hub + "' -Token '" + s.token + "'" + hnW + '"';
      document.getElementById('cmdPull').value =
        '~/.skillshooter/skillshooter pull --server ' + hub + ' --token ' + s.token + ' --dest ~/.agents/skills';
      document.getElementById('cmdDaemon').value =
        'nohup ~/.skillshooter/skillshooter daemon --server ' + hub + ' --token ' + s.token + hnU + ' > ~/.skillshooter/daemon.log 2>&1 &';
    };
    buildCmds();
    document.getElementById('bsHost').addEventListener('input', buildCmds);
    window._copyCmd = id => {
      const el = document.getElementById(id);
      el.select();
      navigator.clipboard.writeText(el.value).then(() => toast('已复制，去目标机器终端粘贴执行')).catch(() => {
        document.execCommand('copy'); toast(LANG === 'zh' ? '已复制' : 'Copied');
      });
    };
    window._dlClient = async name => {
      try {
        toast('正在生成已配置客户端…');
        const r = await fetch('/api/client/' + name);
        if (r.status === 403) throw new Error('当前身份不能下载客户端（需管理员）');
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
    document.getElementById('llmsave').onclick = async () => {
      try {
        const r = await apiPost('/settings/llm', {
          api_key: document.getElementById('llmkey').value.trim(),
          base_url: document.getElementById('llmbase').value.trim(),
          model: document.getElementById('llmmodel').value.trim(),
        });
        toast(r.enabled ? (LANG === 'zh' ? '已保存并生效（LLM 已配置）' : 'Saved & active (LLM configured)') : (LANG === 'zh' ? '已保存（key/model 不完整，LLM 未启用）' : 'Saved (key/model incomplete, LLM disabled)'));
        pageSettings();
      } catch (e) { toast(e.message, true); }
    };
    document.getElementById('llmtestbtn').onclick = async () => {
      document.getElementById('llmres').textContent = LANG === 'zh' ? '测试中…' : 'Testing…';
      try {
        const r = await apiPost('/settings/llm-test');
        document.getElementById('llmres').innerHTML = '<span style="color:var(--ok)">✓ ' + (LANG === 'zh' ? '连通正常，模型回复: ' : 'OK, model replied: ') + esc(r.reply) + '</span>';
      } catch (e) {
        document.getElementById('llmres').innerHTML = '<span style="color:var(--danger)">✗ ' + esc(e.message) + '</span>';
      }
    };
  }
  document.getElementById('pwbtn').onclick = async () => {
    const old_ = document.getElementById('pwold').value, nw = document.getElementById('pwnew').value;
    if (!nw || nw.length < 6) { document.getElementById('pwmsg').textContent = LANG === 'zh' ? '新密码至少 6 位' : 'New password must be at least 6 chars'; return; }
    try {
      await apiPost('/password', {old: old_, new: nw});
      document.getElementById('pwmsg').innerHTML = '<span style="color:var(--ok)">✓ ' + (LANG === 'zh' ? '已修改' : 'Changed') + '</span>';
      document.getElementById('pwold').value = ''; document.getElementById('pwnew').value = '';
    } catch (e) { document.getElementById('pwmsg').innerHTML = '<span style="color:var(--danger)">✗ ' + esc(terr(e.message)) + '</span>'; }
  };
}

/* ---------- 远程调用 ---------- */
async function pageJobs() {
  $app.innerHTML = '<div class="loading">加载中…</div>';
  const [machines, skills, jobs] = await Promise.all([api('/machines'), api('/skills'), api('/jobs')]);
  const skillNames = [...new Set(skills.map(s => s.name))];
  $app.innerHTML = `
    <div class="page-head"><h1>${t('jobs_title')}</h1></div>
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
    <h2>${t('jobs_list')}</h2>
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

boot();
