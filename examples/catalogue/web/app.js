'use strict';
const headers = {Authorization: 'Bearer catalogue-demo'};
const el = id => document.getElementById(id);
let snapshot = {documents: []};
const bytes = n => n < 1024 ? `${n} B` : n < 1048576 ? `${(n/1024).toFixed(1)} KiB` : `${(n/1048576).toFixed(1)} MiB`;
function render(data) {
 if (!data.policy) return;
 snapshot = data;
 el('count').textContent = data.documents.length;
 el('storage').textContent = `${bytes(data.bytes)} of ${bytes(data.policy.max_total_bytes)} used · ${data.documents.length} of ${data.policy.max_documents} documents`;
 el('limit').textContent = `Up to ${bytes(data.policy.max_file_bytes)} per file.`;
 const query = el('search').value.toLowerCase();
 const visible = data.documents.filter(d => d.name.toLowerCase().includes(query));
 const list = el('documents');
 const ids = new Set(visible.map(doc => String(doc.id)));
 for (const row of [...list.children]) {
  if (!row.dataset.id || !ids.has(row.dataset.id)) row.remove();
 }
 if (!visible.length) {const p = document.createElement('p');p.className='empty';p.textContent=query?'No matching documents.':'No documents yet. Add your first file.';el('documents').append(p);}
 for (const doc of visible) {
  if ([...list.children].some(row => row.dataset.id === String(doc.id))) continue;
  const row=document.createElement('div');row.className='document';row.dataset.id=String(doc.id);
  const link=document.createElement('a');link.href=`/api/documents/${doc.id}`;link.textContent=doc.name;
  link.addEventListener('click',async event=>{event.preventDefault();try{const res=await fetch(link.href,{headers});if(!res.ok)throw new Error(await res.text());const url=URL.createObjectURL(await res.blob());const a=document.createElement('a');a.href=url;a.download=doc.name;a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}catch(error){el('result').textContent=error.message;}});
  const size=document.createElement('small');size.textContent=bytes(doc.size);row.append(link,size);el('documents').append(row);
 }
}
el('search').addEventListener('input',()=>render(snapshot));
el('upload').addEventListener('submit',async event=>{
 event.preventDefault();const file=el('file').files[0];if(!file)return;
 const button=event.currentTarget.querySelector('button');button.disabled=true;el('result').textContent='Uploading…';
 try{if(file.size>snapshot.policy.max_file_bytes)throw new Error('This file exceeds the catalogue limit.');const response=await fetch('/api/documents',{method:'POST',headers:{...headers,'Content-Type':'application/octet-stream','X-Document-Name':encodeURIComponent(file.name)},body:file});if(!response.ok)throw new Error(await response.text());el('result').textContent=`Added ${file.name}.`;el('upload').reset();await refresh();}catch(error){el('result').textContent=error.message;}finally{button.disabled=false;}
});
async function refresh(){const res=await fetch('/api/documents',{headers});if(!res.ok)throw new Error(await res.text());render(await res.json());}
let active;
async function connect(){
 active=new AbortController();
 try{
  const response=await fetch('/api/events',{headers,signal:active.signal});if(!response.ok)throw new Error(await response.text());
  const reader=response.body.pipeThrough(new TextDecoderStream()).getReader();let pending='';el('connection').textContent='Live updates connected';
  try{while(true){const {value,done}=await reader.read();if(done)break;pending+=value;let boundary;while((boundary=pending.indexOf('\n\n'))>=0){const frame=pending.slice(0,boundary);pending=pending.slice(boundary+2);const data=frame.split('\n').filter(line=>line.startsWith('data: ')).map(line=>line.slice(6)).join('\n');if(data)render(JSON.parse(data));}}}finally{await reader.cancel();reader.releaseLock();}
 }catch(error){if(error.name==='AbortError')return;}
 if(!active.signal.aborted){el('connection').textContent='Reconnecting live updates…';setTimeout(connect,1500);}
}
window.addEventListener('pagehide',()=>active?.abort());
refresh().then(connect).catch(error=>{el('connection').textContent='Connection unavailable';el('result').textContent=error.message;});
