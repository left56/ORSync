package server

// rootDashboardHTML 为 GET / 返回的单页看板（无外部 JS/CSS 依赖）。
const rootDashboardHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ORSync 看板</title>
<style>
  * { box-sizing: border-box; }
  body {
    font-family: system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
    margin: 0;
    padding: 1.5rem;
    background: #f4f5f7;
    color: #1a1a1a;
  }
  .wrap { max-width: 960px; margin: 0 auto; }
  h1 {
    text-align: center;
    font-size: 1.75rem;
    font-weight: 600;
    margin: 0 0 1.25rem;
    letter-spacing: 0.02em;
  }
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 1rem;
  }
  button {
    cursor: pointer;
    padding: 0.5rem 1rem;
    font-size: 0.95rem;
    border: 1px solid #2563eb;
    background: #2563eb;
    color: #fff;
    border-radius: 6px;
  }
  button:hover { background: #1d4ed8; border-color: #1d4ed8; }
  button:disabled { opacity: 0.55; cursor: not-allowed; }
  .muted { color: #555; font-size: 0.9rem; }
  .status {
    min-height: 1.25rem;
    font-size: 0.9rem;
    margin-bottom: 0.75rem;
  }
  .status.err { color: #b91c1c; }
  .status.ok { color: #15803d; }
  table {
    width: 100%;
    border-collapse: collapse;
    background: #fff;
    border-radius: 8px;
    overflow: hidden;
    box-shadow: 0 1px 3px rgba(0,0,0,0.08);
    font-size: 0.875rem;
  }
  th, td {
    padding: 0.5rem 0.65rem;
    text-align: left;
    border-bottom: 1px solid #e5e7eb;
    vertical-align: top;
  }
  th { background: #eef2ff; font-weight: 600; }
  tr:last-child td { border-bottom: none; }
  .col-msg { max-width: 280px; word-break: break-word; }
  caption {
    caption-side: top;
    text-align: left;
    padding: 0 0 0.5rem;
    font-weight: 600;
  }
</style>
</head>
<body>
  <div class="wrap">
    <h1>ORSync 中间件运行状态</h1>
    <div class="toolbar">
      <button type="button" id="btnFullSync">触发全量同步</button>
      <span class="muted" id="logMeta"></span>
    </div>
    <div class="status" id="status" role="status"></div>
    <table>
      <caption>最近同步日志</caption>
      <thead>
        <tr>
          <th>ID</th>
          <th>时间</th>
          <th>事件</th>
          <th>文档</th>
          <th>状态</th>
          <th class="col-msg">说明</th>
        </tr>
      </thead>
      <tbody id="logBody"></tbody>
    </table>
  </div>
<script>
(function () {
  var statusEl = document.getElementById('status');
  var logBody = document.getElementById('logBody');
  var logMeta = document.getElementById('logMeta');
  var btn = document.getElementById('btnFullSync');

  function setStatus(msg, cls) {
    statusEl.textContent = msg || '';
    statusEl.className = 'status' + (cls ? ' ' + cls : '');
  }

  function esc(s) {
    if (s == null) return '';
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  async function loadLogs() {
    setStatus('正在加载日志…', '');
    logBody.innerHTML = '';
    try {
      var r = await fetch('/api/logs?page=1&page_size=20', { method: 'GET' });
      var j = await r.json();
      if (!r.ok) {
        setStatus('加载失败: ' + (j.error || r.statusText), 'err');
        return;
      }
      logMeta.textContent = '共 ' + j.total + ' 条 · 本页 ' + (j.data ? j.data.length : 0) + ' 条';
      setStatus('', '');
      if (!j.data || !j.data.length) {
        var tr = document.createElement('tr');
        var td = document.createElement('td');
        td.colSpan = 6;
        td.textContent = '暂无日志';
        tr.appendChild(td);
        logBody.appendChild(tr);
        return;
      }
      j.data.forEach(function (row) {
        var tr = document.createElement('tr');
        tr.innerHTML =
          '<td>' + esc(row.id) + '</td>' +
          '<td>' + esc(row.created_at) + '</td>' +
          '<td>' + esc(row.event_type) + '</td>' +
          '<td>' + esc(row.target_doc_id) + '</td>' +
          '<td>' + esc(row.status) + '</td>' +
          '<td class="col-msg">' + esc(row.message) + '</td>';
        logBody.appendChild(tr);
      });
    } catch (e) {
      setStatus('请求失败: ' + e, 'err');
    }
  }

  async function triggerFullSync() {
    btn.disabled = true;
    setStatus('正在请求全量同步…', '');
    try {
      var r = await fetch('/api/sync/full', { method: 'POST' });
      var j = await r.json().catch(function () { return {}; });
      if (r.status === 429) {
        setStatus(j.error || '全量同步已在进行中', 'err');
      } else if (!r.ok) {
        setStatus('请求失败: ' + (j.error || r.statusText), 'err');
      } else {
        setStatus(j.message || '已启动后台全量同步', 'ok');
        await loadLogs();
      }
    } catch (e) {
      setStatus('请求失败: ' + e, 'err');
    } finally {
      btn.disabled = false;
    }
  }

  btn.addEventListener('click', triggerFullSync);
  loadLogs();
})();
</script>
</body>
</html>
`
