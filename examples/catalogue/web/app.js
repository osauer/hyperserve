'use strict';

const headers = {Authorization: 'Bearer catalogue-demo'};
const el = id => document.getElementById(id);
const uploadButton = el('upload').querySelector('button');
let snapshot = {documents: []};
let uploading = false;
let active;
let reconnectTimer;

function bytes(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1048576) return `${(n / 1024).toFixed(1)} KiB`;
  return `${(n / 1048576).toFixed(1)} MiB`;
}

async function request(path, options = {}) {
  const response = await fetch(path, {...options, headers: {...headers, ...options.headers}});
  if (!response.ok) throw new Error(await response.text());
  return response;
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
    el('result').textContent = error.message;
  }
}

function render(data) {
  if (!data.policy) return;
  snapshot = data;
  uploadButton.disabled = uploading;
  el('count').textContent = data.documents.length;
  el('storage').textContent = `${bytes(data.bytes)} of ${bytes(data.policy.max_total_bytes)} used · ${data.documents.length} of ${data.policy.max_documents} documents`;
  el('limit').textContent = `Up to ${bytes(data.policy.max_file_bytes)} per file.`;

  const query = el('search').value.toLowerCase();
  const visible = data.documents.filter(doc => doc.name.toLowerCase().includes(query));
  const list = el('documents');
  const ids = new Set(visible.map(doc => String(doc.id)));
  // Keep existing rows so live updates preserve keyboard focus.
  for (const row of [...list.children]) {
    if (!row.dataset.id || !ids.has(row.dataset.id)) row.remove();
  }
  if (!visible.length) {
    const empty = document.createElement('p');
    empty.className = 'empty';
    empty.textContent = query ? 'No matching documents.' : 'No documents yet. Add your first file.';
    list.append(empty);
  }
  for (const doc of visible) {
    if ([...list.children].some(row => row.dataset.id === String(doc.id))) continue;
    const row = document.createElement('div');
    row.className = 'document';
    row.dataset.id = String(doc.id);
    const link = document.createElement('a');
    link.href = `/api/documents/${doc.id}`;
    link.textContent = doc.name;
    link.addEventListener('click', event => {
      event.preventDefault();
      download(doc);
    });
    const size = document.createElement('small');
    size.textContent = bytes(doc.size);
    row.append(link, size);
    list.append(row);
  }
}

el('search').addEventListener('input', () => render(snapshot));
el('upload').addEventListener('submit', async event => {
  event.preventDefault();
  const file = el('file').files[0];
  if (!file || !snapshot.policy || uploading) return;
  uploading = true;
  uploadButton.disabled = true;
  el('result').textContent = 'Uploading…';
  try {
    if (file.size > snapshot.policy.max_file_bytes) throw new Error('This file exceeds the catalogue limit.');
    await request('/api/documents', {
      method: 'POST',
      headers: {'Content-Type': 'application/octet-stream', 'X-Document-Name': encodeURIComponent(file.name)},
      body: file,
    });
    el('result').textContent = `Added ${file.name}.`;
    el('upload').reset();
    // SSE is the single source of snapshots; a competing GET could arrive late
    // and overwrite a newer live update.
  } catch (error) {
    el('result').textContent = error.message;
  } finally {
    uploading = false;
    uploadButton.disabled = false;
  }
});

async function connect() {
  const controller = new AbortController();
  active = controller;
  try {
    const response = await request('/api/events', {signal: controller.signal});
    const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
    let pending = '';
    el('connection').textContent = 'Live updates connected';
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
          if (data) render(JSON.parse(data));
        }
      }
    } finally {
      await reader.cancel();
      reader.releaseLock();
    }
  } catch (error) {
    if (controller.signal.aborted) return;
  }
  if (!controller.signal.aborted) {
    el('connection').textContent = 'Reconnecting live updates…';
    reconnectTimer = setTimeout(connect, 1500);
  }
}

window.addEventListener('pagehide', () => {
  clearTimeout(reconnectTimer);
  active?.abort();
});
window.addEventListener('pageshow', event => {
  if (event.persisted) connect();
});
connect();
