'use strict';

const headers = {Authorization: 'Bearer catalogue-demo'};
const el = id => document.getElementById(id);
const uploadButton = el('upload-button');
let snapshot = {documents: []};
let uploading = false;
let connected = false;
let active;
let reconnectTimer;
let mode = 'json';
let mcpPath;
let mcpVersion;
let readGeneration = 0;

function bytes(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1048576) return `${Number((n / 1024).toFixed(1))} KiB`;
  return `${Number((n / 1048576).toFixed(1))} MiB`;
}

async function request(path, options = {}) {
  const response = await fetch(path, {...options, headers: {...headers, ...options.headers}});
  if (!response.ok) throw new Error((await response.text()).trim() || `Request failed (${response.status}).`);
  return response;
}

function feedback(message, state = 'success') {
  el('result').textContent = message;
  el('result').dataset.state = state;
}

function updateControls() {
  const ready = connected && snapshot.policy && !uploading;
  uploadButton.disabled = !ready || !el('file').files.length;
  el('sample').disabled = !ready;
  el('file').disabled = uploading;
  uploadButton.firstChild.textContent = uploading ? 'Uploading… ' : 'Upload document ';
}

function showFile() {
  const file = el('file').files[0];
  el('file-name').textContent = file ? file.name : 'Choose a sample file';
  el('file-detail').textContent = file ? `${bytes(file.size)} · choose another` : 'or drop it here';
  feedback('');
  updateControls();
}

async function download(doc) {
  try {
    const response = await request(`/api/documents/${doc.id}`);
    const url = URL.createObjectURL(await response.blob());
    const link = document.createElement('a');
    link.href = url;
    link.download = doc.name;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  } catch (error) {
    feedback(`Could not download ${doc.name}. ${error.message}`, 'error');
  }
}

function render(data) {
  if (!data.policy) return;
  const restarted = snapshot.policy && data.revision < snapshot.revision;
  snapshot = data;
  el('count').textContent = data.documents.length;
  el('storage').textContent = `${bytes(data.bytes)} of ${bytes(data.policy.max_total_bytes)} used · ${data.documents.length} of ${data.policy.max_documents} documents`;
  el('capacity').hidden = false;
  el('capacity').max = data.policy.max_total_bytes;
  el('capacity').value = data.bytes;
  el('limit').textContent = `Up to ${bytes(data.policy.max_file_bytes)} per file.`;
  if (restarted) feedback('The server restarted. This temporary collection has been cleared.');

  const query = el('search').value.toLowerCase();
  const visible = data.documents.filter(doc => doc.name.toLowerCase().includes(query));
  el('filtered').textContent = query ? `${visible.length} of ${data.documents.length}` : '';
  const list = el('documents');
  list.setAttribute('aria-busy', 'false');
  const ids = new Set(visible.map(doc => String(doc.id)));
  // Keep existing rows so live updates preserve keyboard focus. Compare the
  // contents too: a restarted process can reuse a document ID.
  for (const row of [...list.children]) {
    const doc = visible.find(item => String(item.id) === row.dataset.id);
    if (!ids.has(row.dataset.id) || row.dataset.created !== doc?.created) row.remove();
  }
  if (!visible.length) {
    const empty = document.createElement('div');
    empty.className = 'empty';
    const icon = document.createElement('span');
    icon.className = 'sheet-icon';
    icon.setAttribute('aria-hidden', 'true');
    icon.textContent = query ? '?' : '↳';
    const title = document.createElement('h3');
    title.textContent = query ? 'No matching filenames.' : 'Your first document belongs here.';
    const help = document.createElement('p');
    help.textContent = query ? 'Try another filename or clear the search to see the collection.' : 'Choose a file, or add the sample note to try all four interfaces.';
    empty.append(icon, title, help);
    list.replaceChildren(empty);
  }
  for (const doc of visible) {
    if ([...list.children].some(row => row.dataset.id === String(doc.id))) continue;
    const row = document.createElement('div');
    row.className = 'document';
    row.dataset.id = String(doc.id);
    row.dataset.created = doc.created;
    const link = document.createElement('a');
    link.href = `/api/documents/${doc.id}`;
    link.textContent = doc.name;
    link.setAttribute('aria-label', `Download ${doc.name}`);
    link.addEventListener('click', event => {
      event.preventDefault();
      download(doc);
    });
    const size = document.createElement('small');
    size.textContent = bytes(doc.size);
    row.append(link, size);
    list.append(row);
  }
  updateControls();
}

async function upload(file) {
  if (!file || !snapshot.policy || !connected || uploading) return;
  uploading = true;
  updateControls();
  feedback(`Uploading ${file.name}…`);
  try {
    if (file.size > snapshot.policy.max_file_bytes) throw new Error(`Choose a file up to ${bytes(snapshot.policy.max_file_bytes)}.`);
    if (snapshot.documents.length >= snapshot.policy.max_documents || snapshot.bytes + file.size > snapshot.policy.max_total_bytes) {
      throw new Error('The collection is full. Restart the demo server to clear it.');
    }
    await request('/api/documents', {
      method: 'POST',
      headers: {'Content-Type': 'application/octet-stream', 'X-Document-Name': encodeURIComponent(file.name)},
      body: file,
    });
    el('upload').reset();
    showFile();
    feedback(`Added ${file.name}.${el('search').value ? ' Clear the search if it is not shown.' : ''}`);
    // SSE owns the displayed collection; competing GET responses can arrive late.
  } catch (error) {
    feedback(error instanceof TypeError ? 'Upload could not be confirmed. Reconnect and check the collection before retrying.' : error.message, 'error');
  } finally {
    uploading = false;
    updateControls();
  }
}

el('file').addEventListener('change', showFile);
el('upload').addEventListener('submit', event => {
  event.preventDefault();
  upload(el('file').files[0]);
});
el('sample').addEventListener('click', () => upload(new File([
  '# Catalogue notes\n\nThis sample belongs to the HyperServe reference application.\nFind it in the browser, JSON API and MCP search tool.\nOpen a second view to see live updates.\n',
], 'catalogue-notes.txt', {type: 'text/plain'})));
for (const event of ['dragenter', 'dragover']) {
  el('dropzone').addEventListener(event, e => {
    e.preventDefault();
    if (!uploading) el('dropzone').classList.add('dragging');
  });
}
for (const event of ['dragleave', 'drop']) {
  el('dropzone').addEventListener(event, e => {
    e.preventDefault();
    el('dropzone').classList.remove('dragging');
    if (event === 'drop' && !uploading) {
      if (e.dataTransfer.files.length !== 1) {
        feedback('Choose one file at a time.', 'error');
        return;
      }
      el('file').files = e.dataTransfer.files;
      showFile();
    }
  });
}
document.addEventListener('keydown', event => {
  if (event.key === '/' && !event.ctrlKey && !event.metaKey && !event.altKey && !event.target.closest('input, textarea, [contenteditable]')) {
    event.preventDefault();
    el('search').focus();
  }
});

function shellQuote(value) {
  return "'" + value.replaceAll("'", "'\\''") + "'";
}
function readRequest() {
  const query = el('search').value;
  if (mode === 'json') return {path: `/api/documents${query ? `?q=${encodeURIComponent(query)}` : ''}`, options: {}};
  return {path: mcpPath, options: {
    method: 'POST',
    headers: {'Content-Type': 'application/json', 'Accept': 'application/json, text/event-stream', 'Mcp-Method': 'tools/call', 'Mcp-Name': 'search_documents', 'MCP-Protocol-Version': mcpVersion},
    body: JSON.stringify({jsonrpc: '2.0', id: 1, method: 'tools/call', params: {name: 'search_documents', arguments: {query}, _meta: {'io.modelcontextprotocol/protocolVersion': mcpVersion}}}),
  }};
}
function updateInspector() {
  readGeneration++;
  el('json-mode').setAttribute('aria-pressed', mode === 'json');
  el('mcp-mode').setAttribute('aria-pressed', mode === 'mcp');
  const {path, options} = readRequest();
  el('run-read').disabled = !path;
  el('request-label').textContent = mode === 'json' ? `GET ${path}` : 'tools/call → search_documents';
  el('response').textContent = 'Run a read to see the actual server response.';
  el('read-status').textContent = 'The public demo token is sent with each request.';
  const lines = [`curl ${shellQuote(location.origin + (path || ''))}`, "  -H 'Authorization: Bearer catalogue-demo'"];
  for (const [key, value] of Object.entries(options.headers || {})) lines.push(`  -H ${shellQuote(`${key}: ${value}`)}`);
  if (options.body) lines.push(`  --data ${shellQuote(options.body)}`);
  el('command').textContent = path ? lines.join(' \\\n') : 'Discovering the MCP endpoint…';
  el('copy').disabled = !path;
  el('copy').textContent = 'Copy command';
}
el('search').addEventListener('input', () => { render(snapshot); updateInspector(); });
for (const name of ['json', 'mcp']) el(`${name}-mode`).addEventListener('click', () => { mode = name; updateInspector(); });
el('run-read').addEventListener('click', async () => {
  const generation = ++readGeneration;
  const {path, options} = readRequest();
  if (!path) return;
  el('run-read').disabled = true;
  el('read-status').textContent = 'Reading from the server…';
  try {
    const response = await request(path, options);
    const data = await response.json();
    if (generation !== readGeneration) return;
    el('response').textContent = JSON.stringify(data, null, 2);
    el('read-status').textContent = data.error || data.result?.isError ? 'The server returned an error. See the response above.' : `Read completed at ${new Date().toLocaleTimeString()}. Run again to refresh.`;
  } catch (error) {
    if (generation !== readGeneration) return;
    el('response').textContent = error.message;
    el('read-status').textContent = 'Could not complete the read. Check the server and try again.';
  } finally {
    if (generation === readGeneration) el('run-read').disabled = false;
  }
});
el('copy').addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText(el('command').textContent);
    el('copy').textContent = 'Copied';
  } catch {
    el('copy').textContent = 'Select the command to copy it';
  }
});
async function discover() {
  try {
    const info = await (await request('/.well-known/mcp.json')).json();
    const endpoint = new URL(info.endpoints.mcp, location.href);
    if (endpoint.origin !== location.origin) throw new Error('MCP endpoint must belong to this server.');
    mcpPath = endpoint.pathname;
    mcpVersion = info.version;
    updateInspector();
  } catch {
    if (mode === 'mcp') el('read-status').textContent = 'MCP discovery is unavailable. Reload after the server reconnects.';
  }
}

async function connect() {
  const controller = new AbortController();
  active = controller;
  try {
    const response = await request('/api/events', {signal: controller.signal});
    const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
    let pending = '';
    try {
      while (true) {
        const {value, done} = await reader.read();
        if (done) break;
        pending += value;
        let boundary;
        // This endpoint emits UTF-8 SSE frames with LF separators.
        while ((boundary = pending.indexOf('\n\n')) >= 0) {
          const frame = pending.slice(0, boundary);
          pending = pending.slice(boundary + 2);
          const data = frame.split('\n').filter(line => line.startsWith('data: ')).map(line => line.slice(6)).join('\n');
          if (data) {
            connected = true;
            render(JSON.parse(data));
            el('connection').textContent = 'Live updates connected';
            el('connection').dataset.state = 'live';
          }
        }
      }
    } finally {
      await reader.cancel();
      reader.releaseLock();
    }
  } catch {
    if (controller.signal.aborted) return;
  }
  if (!controller.signal.aborted) {
    connected = false;
    updateControls();
    el('connection').textContent = 'Connection lost · reconnecting';
    el('connection').dataset.state = 'waiting';
    if (!snapshot.policy) {
      el('documents').setAttribute('aria-busy', 'false');
      el('documents').querySelector('h3').textContent = 'The server is not responding.';
      el('documents').querySelector('p').textContent = 'Check that the demo is running. This view will reconnect automatically.';
    }
    reconnectTimer = setTimeout(connect, 1500);
  }
}
window.addEventListener('pagehide', () => {
  clearTimeout(reconnectTimer);
  active?.abort();
});
window.addEventListener('pageshow', event => {
  if (event.persisted) { connect(); discover(); }
});
updateInspector();
discover();
connect();
