const $=(s,r=document)=>{const root=typeof r==='string'?document.querySelector(r):r;return root?.querySelector?.(s)||null};
const $$=(s,r=document)=>{const root=typeof r==='string'?document.querySelector(r):r;return root?.querySelectorAll?[...root.querySelectorAll(s)]:[]};
const state={session:null,products:[],staff:[],activeView:'dashboard',dashboard:null,pendingImage:null,salesPage:1,databaseTable:'',databasePage:1,stockMode:'single',inboxTab:'forms',chatThreads:[],chatThread:'',chatPoll:null};

async function request(path,options={}){
  const response=await fetch(path,{credentials:'same-origin',headers:{Accept:'application/json','Content-Type':'application/json',...(options.headers||{})},...options});
  const data=await response.json().catch(()=>({}));
  if(response.status===401){location.replace('/access?next=/admin');throw new Error('Session expired')}
  if(!response.ok)throw new Error(data.error||`Request failed (${response.status})`);
  return data;
}
function money(v){return new Intl.NumberFormat('en-PH',{style:'currency',currency:'PHP',maximumFractionDigits:2}).format(Number(v)||0)}
function escapeHTML(v){return String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function initials(name){return String(name||'?').split(/\s+/).filter(Boolean).slice(0,2).map(x=>x[0]).join('').toUpperCase()}
function toast(message){if(!message)return;const r=$('#toastRegion'),e=document.createElement('div');e.className='toast';e.textContent=message;r.append(e);setTimeout(()=>e.remove(),3400)}
function setTheme(theme){document.documentElement.dataset.theme=theme==='dark'?'dark':'light';try{localStorage.setItem('lixr-theme',document.documentElement.dataset.theme)}catch{};if(state.dashboard)requestAnimationFrame(()=>drawDashboardCharts(state.dashboard))}
function initTheme(){let saved='';try{saved=localStorage.getItem('lixr-theme')||''}catch{};setTheme(saved||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'))}

const viewHints={dashboard:'Today’s store activity at a glance',inventory:'Products, prices and size-by-size stock',sales:'Receipts and completed sales records',contact:'Customer messages and follow-up status',ratings:'Customer reviews and visibility',staff:'Cashier, admin and developer access',database:'Browse stored records and tables',system:'Computer, storage and connection details'};
function showView(name){
  state.activeView=name;
  $$('.admin-view').forEach(x=>x.classList.toggle('active',x.dataset.viewPanel===name));
  $$('.admin-nav').forEach(x=>x.classList.toggle('active',x.dataset.view===name));
  $('#adminBreadcrumb').textContent=`Admin / ${name==='sales'?'Sales History':name[0].toUpperCase()+name.slice(1)}`;
  $('#adminHeaderHint').textContent=viewHints[name]||'Lixr';
  $('#adminSidebar').classList.remove('open');
  clearInterval(state.chatPoll);state.chatPoll=null;
  if(name==='dashboard')loadDashboard();
  if(name==='inventory')loadInventory();
  if(name==='sales')loadSales();
  if(name==='staff')loadStaff();
  if(name==='contact'){loadContacts();loadChats();state.chatPoll=setInterval(()=>{if(state.activeView==='contact'){loadChats(true);if(state.inboxTab==='chat'&&state.chatThread)loadAdminChat(state.chatThread,true)}},4000)}
  if(name==='ratings')loadRatings();
  if(name==='database')loadDatabase();
  if(name==='system')loadSystem();
}

async function bootstrap(){
  try{
    const [session,health]=await Promise.all([request('/api/auth/session'),request('/api/health')]);
    if(!session.authenticated||!session.staff){location.replace('/access?next=/admin');return}
    if(session.staff.role==='cashier'){location.replace('/pos');return}
    state.session=session.staff;$('#adminUser').textContent=session.staff.display_name||session.staff.username;$('#adminRole').textContent=session.staff.role;$('#dashboardDbState').textContent='Store records ready';$('#integrationOAuth').textContent=health.oauth_configured?'Ready':'Not set';$('#integrationSMTP').textContent=health.smtp_configured?'Ready':'Not set';await loadDashboard();
  }catch(error){$('#dashboardDbState').textContent='Unavailable';toast(error.message)}
}

async function loadDashboard(){
  try{const data=await request('/api/admin/dashboard'),d=data.dashboard||{};state.dashboard=d;$('#kpiVisitors').textContent=d.visitors_today??0;$('#kpiVisitorsTotal').textContent=`${d.visitors_total??0} total visitors`;$('#kpiSales').textContent=d.sales_count??0;$('#kpiSalesToday').textContent=`${d.sales_today??0} today`;$('#kpiRevenue').textContent=money(d.revenue);$('#kpiProfit').textContent=money(d.profit);$('#kpiMargin').textContent=`${Number(d.margin_percent||0).toFixed(1)}%`;$('#kpiLow').textContent=d.low_stock??0;$('#kpiOut').textContent=d.out_of_stock??0;$('#kpiMessages').textContent=d.messages??0;$('#kpiRatings').textContent=d.ratings??0;$('#kpiStaff').textContent=d.staff??0;$('#kpiInventoryValue').textContent=money(d.inventory_cost_value);$('#navLowStock').textContent=d.low_stock??0;$('#navMessages').textContent=d.messages??0;$('#dashboardRating').textContent=`${Number(d.average_rating||0).toFixed(1)} / 5 rating`;renderDashboardTables(d);drawDashboardCharts(d)}catch(error){toast(error.message)}
}
function renderDashboardTables(d){const sales=d.recent_sales||[],stock=d.stock_watch||[];$('#dashboardSalesRows').innerHTML=sales.length?sales.map(x=>`<tr><td><button class="receipt-link" type="button" data-dashboard-receipt="${x.id}">${escapeHTML(x.receipt_no||'Pending')}</button></td><td>${escapeHTML(x.customer||'Walk-in')}</td><td>${money(x.total)}</td><td><span class="status-pill ${x.status==='completed'?'safe':''}">${escapeHTML(x.status)}</span></td></tr>`).join(''):'<tr><td colspan="4" class="mini-empty">No completed sales yet.</td></tr>';$('#dashboardStockRows').innerHTML=stock.length?stock.map(x=>`<tr><td>${escapeHTML(x.name)}</td><td>${escapeHTML(x.sku)}</td><td><b class="${Number(x.stock)<=0?'danger-text':'warn-text'}">${Number(x.stock||0)}</b></td></tr>`).join(''):'<tr><td colspan="3" class="mini-empty">Stock levels look healthy.</td></tr>';$$('[data-dashboard-receipt]').forEach(b=>b.onclick=()=>openReceipt(Number(b.dataset.dashboardReceipt)))}
function chartColors(){const cs=getComputedStyle(document.documentElement);return{ink:cs.getPropertyValue('--ink').trim()||'#222',muted:cs.getPropertyValue('--muted').trim()||'#777',line:cs.getPropertyValue('--line').trim()||'rgba(0,0,0,.15)',accent:cs.getPropertyValue('--accent-deep').trim()||'#75694d',surface:cs.getPropertyValue('--surface').trim()||'#fff'}}
function prepareCanvas(canvas){if(!canvas)return null;const rect=canvas.getBoundingClientRect(),ratio=Math.min(2,window.devicePixelRatio||1),h=Number(canvas.getAttribute('height'))||190;canvas.width=Math.max(1,Math.floor(rect.width*ratio));canvas.height=Math.floor(h*ratio);const ctx=canvas.getContext('2d');ctx.setTransform(ratio,0,0,ratio,0,0);return{ctx,w:rect.width,h}}
function drawGrid(ctx,w,h,c){ctx.strokeStyle=c.line;ctx.lineWidth=1;ctx.globalAlpha=.7;for(let i=1;i<5;i++){const y=18+(h-42)*i/5;ctx.beginPath();ctx.moveTo(0,y);ctx.lineTo(w,y);ctx.stroke()}ctx.globalAlpha=1}
function lineChart(canvas,values,{wave=false}={}){const box=prepareCanvas(canvas);if(!box)return;const{ctx,w,h}=box,c=chartColors(),vals=(values||[]).map(Number),max=Math.max(1,...vals);ctx.clearRect(0,0,w,h);drawGrid(ctx,w,h,c);const left=5,right=w-5,top=18,bottom=h-24,step=vals.length>1?(right-left)/(vals.length-1):0;ctx.strokeStyle=c.accent;ctx.lineWidth=2;ctx.beginPath();vals.forEach((v,i)=>{const x=left+step*i,y=bottom-(v/max)*(bottom-top);if(i===0)ctx.moveTo(x,y);else if(wave){const px=left+step*(i-1),pv=vals[i-1],py=bottom-(pv/max)*(bottom-top),mx=(px+x)/2;ctx.bezierCurveTo(mx,py,mx,y,x,y)}else ctx.lineTo(x,y)});ctx.stroke();ctx.lineTo(right,bottom);ctx.lineTo(left,bottom);ctx.closePath();ctx.globalAlpha=.08;ctx.fillStyle=c.accent;ctx.fill();ctx.globalAlpha=1}
function barChart(canvas,values){const box=prepareCanvas(canvas);if(!box)return;const{ctx,w,h}=box,c=chartColors(),vals=(values||[]).map(Number),max=Math.max(1,...vals);ctx.clearRect(0,0,w,h);drawGrid(ctx,w,h,c);const gap=5,usable=w-8,bw=Math.max(3,(usable-gap*(vals.length-1))/Math.max(1,vals.length));vals.forEach((v,i)=>{const bh=(v/max)*(h-46),x=4+i*(bw+gap),y=h-24-bh;ctx.fillStyle=c.accent;ctx.globalAlpha=.78;ctx.fillRect(x,y,bw,bh)});ctx.globalAlpha=1}
function pieChart(canvas,labels,values){const box=prepareCanvas(canvas);if(!box)return;const{ctx,w,h}=box,c=chartColors(),vals=(values||[]).map(Number),total=vals.reduce((a,b)=>a+b,0)||1,cx=w/2,cy=h/2,r=Math.min(w,h)*.34;ctx.clearRect(0,0,w,h);let start=-Math.PI/2;vals.forEach((v,i)=>{const angle=v/total*Math.PI*2;ctx.beginPath();ctx.moveTo(cx,cy);ctx.arc(cx,cy,r,start,start+angle);ctx.closePath();ctx.globalAlpha=.22+.72*((i%6)+1)/6;ctx.fillStyle=c.accent;ctx.fill();start+=angle});ctx.globalAlpha=1;ctx.beginPath();ctx.arc(cx,cy,r*.56,0,Math.PI*2);ctx.fillStyle=c.surface;ctx.fill();ctx.fillStyle=c.ink;ctx.font='600 20px Manrope';ctx.textAlign='center';ctx.fillText(String(vals.reduce((a,b)=>a+b,0)),cx,cy+6);const legend=$('#stockLegend');if(legend)legend.innerHTML=(labels||[]).map((l,i)=>`<span><i style="--legend-opacity:${.22+.72*((i%6)+1)/6}"></i>${escapeHTML(l)} · ${Number(vals[i]||0)}</span>`).join('')}
function drawDashboardCharts(d){lineChart($('#visitorChart'),d.chart_visitors);barChart($('#salesBar'),d.chart_sales);lineChart($('#profitWave'),d.chart_profit,{wave:true});pieChart($('#stockPie'),d.category_labels,d.category_stock)}

function statusFor(p){if(!p.active)return['Hidden',''];if(p.stock<=0)return['Sold out','out'];if(p.stock<=p.low_stock_threshold)return['Low','low'];return['Available','safe']}
async function loadInventory(){const q=$('#adminProductSearch').value.trim();try{const data=await request(`/api/admin/inventory${q?`?q=${encodeURIComponent(q)}`:''}`);state.products=data.products||[];renderInventory()}catch(error){$('#inventoryRows').innerHTML=`<tr><td colspan="10" class="empty-admin">${escapeHTML(error.message)}</td></tr>`}}
function renderInventory(){const rows=$('#inventoryRows');$('#inventoryMeta').textContent=`${state.products.length} product${state.products.length===1?'':'s'}`;if(!state.products.length){rows.innerHTML='<tr><td colspan="10" class="empty-admin">No products match this view.</td></tr>';return}rows.innerHTML=state.products.map(p=>{const[status,cls]=statusFor(p),margin=Number(p.sale_price)-Number(p.cost_price);return`<tr><td><div class="product-cell"><img src="${escapeHTML(p.image_url)}" alt="${escapeHTML(p.image_alt||p.name)}" loading="lazy"><div><b>${escapeHTML(p.name)}</b><span>${escapeHTML(p.sku)} · ${escapeHTML([p.color,p.size].filter(Boolean).join(' / ')||'No sizes saved')}</span></div></div></td><td>${escapeHTML(p.category)}<br><small>${escapeHTML(p.collection||'')}</small></td><td><span class="stock-value">${Number(p.stock||0)}</span></td><td><span class="status-pill ${cls}">${status}</span></td><td>${money(p.cost_price)}</td><td>${money(p.retail_price)}</td><td>${money(p.sale_price)}</td><td class="${margin>=0?'margin-positive':''}">${money(margin)}</td><td>${p.featured?'Yes':'—'}</td><td><div class="row-actions"><button class="table-action primary" data-stock-product="${p.id}" type="button">Stock</button><button class="table-action" data-edit-product="${p.id}" type="button">Edit</button><button class="table-action danger" data-delete-product="${p.id}" type="button">Delete</button></div></td></tr>`}).join('');$$('[data-stock-product]').forEach(b=>b.onclick=()=>openStock(Number(b.dataset.stockProduct)));$$('[data-edit-product]').forEach(b=>b.onclick=()=>openProduct(Number(b.dataset.editProduct)));$$('[data-delete-product]').forEach(b=>b.onclick=()=>openDelete(Number(b.dataset.deleteProduct)))}
function openProduct(id=0){const p=id?state.products.find(x=>x.id===id):null;state.pendingImage=null;$('#productDialogTitle').textContent=p?'Edit product':'Add product';$('#productId').value=p?.id||'';$('#productName').value=p?.name||'';$('#productSku').value=p?.sku||'';$('#productSlug').value=p?.slug||'';$('#productBarcode').value=p?.barcode||'';$('#productCategory').value=p?.category||'';$('#productCollection').value=p?.collection||'';$('#productColor').value=p?.color||'';$('#productCost').value=p?.cost_price??'';$('#productRetail').value=p?.retail_price??'';$('#productSale').value=p?.sale_price??'';$('#productThreshold').value=p?.low_stock_threshold??10;$('#productBadge').value=p?.badge||'';$('#productSubtitle').value=p?.subtitle||'';$('#productImageUrl').value=p?.image_source_url||(!String(p?.image_url||'').startsWith('/api/products/')?p?.image_url||'':'');$('#productDescription').value=p?.description||'';$('#productFeatured').checked=Boolean(p?.featured);$('#productActive').checked=p?Boolean(p.active):true;$('#productImageFile').value='';$('#productFileName').textContent='No file selected';$('#productStatus').textContent='';renderImagePreview(p?.image_url||'');$('#productDialog').showModal()}
function renderImagePreview(url='',dataURL=''){const t=$('#productImagePreview'),src=dataURL||url;if(!src){t.innerHTML='<span>Image preview</span>';return}t.innerHTML=`<img src="${escapeHTML(src)}" alt="Product preview">`}
function fileToDataURL(file){return new Promise((resolve,reject)=>{const r=new FileReader();r.onload=()=>resolve(r.result);r.onerror=()=>reject(new Error('Image could not be read.'));r.readAsDataURL(file)})}
async function setPendingImage(file){if(!file||!file.type.startsWith('image/')){toast('Drop an image file.');return}if(file.size>16*1024*1024){toast('Choose an image under 16 MB.');return}state.pendingImage=file;$('#productFileName').textContent=`${file.name} · ${(file.size/1024/1024).toFixed(2)} MB`;const data=await fileToDataURL(file);renderImagePreview('',data);if(!$('#productDialog').open)openProduct();state.pendingImage=file;$('#productFileName').textContent=`${file.name} · ${(file.size/1024/1024).toFixed(2)} MB`;renderImagePreview('',data)}
async function productPayload(){let imageData='',imageMime='';if(state.pendingImage){imageData=await fileToDataURL(state.pendingImage);imageMime=state.pendingImage.type}return{name:$('#productName').value.trim(),sku:$('#productSku').value.trim(),slug:$('#productSlug').value.trim(),barcode:$('#productBarcode').value.trim(),category:$('#productCategory').value.trim(),collection:$('#productCollection').value.trim(),color:$('#productColor').value.trim(),size:'',cost_price:Number($('#productCost').value),retail_price:Number($('#productRetail').value),sale_price:Number($('#productSale').value),stock:0,low_stock_threshold:Number.parseInt($('#productThreshold').value||'10',10),badge:$('#productBadge').value.trim(),subtitle:$('#productSubtitle').value.trim(),image_url:$('#productImageUrl').value.trim(),image_data:imageData,image_mime:imageMime,image_alt:$('#productName').value.trim(),description:$('#productDescription').value.trim(),featured:$('#productFeatured').checked,active:$('#productActive').checked}}
async function saveProduct(event){event.preventDefault();const status=$('#productStatus'),id=Number($('#productId').value||0);status.className='dialog-status';status.textContent='Saving product…';try{const payload=await productPayload();const data=await request(id?`/api/admin/products/${id}`:'/api/admin/products',{method:id?'PUT':'POST',body:JSON.stringify(payload)});status.classList.add('success');status.textContent=data.message||'Saved.';state.pendingImage=null;await Promise.all([loadInventory(),loadDashboard()]);setTimeout(()=>$('#productDialog').close(),220);toast(data.message||'Product saved.')}catch(error){status.classList.add('error');status.textContent=error.message}}

function addStockRow(label='',quantity=0){
  const row=document.createElement('div');row.className='stock-row';
  row.innerHTML=`<input class="stock-label" maxlength="40" placeholder="Size (S, M, L, XL…)" value="${escapeHTML(label)}"><input class="stock-qty" type="number" min="0" step="1" value="${Number(quantity)||0}" aria-label="Quantity"><button type="button" aria-label="Remove size">×</button>`;
  row.querySelector('button').onclick=()=>{row.remove();updateStockTotal()};
  row.querySelector('.stock-qty').addEventListener('input',updateStockTotal);
  $('#stockSizeList').append(row);updateStockTotal()
}
function setStockMode(mode){
  state.stockMode=mode==='sizes'?'sizes':'single';
  $$('[data-stock-mode]').forEach(b=>{const active=b.dataset.stockMode===state.stockMode;b.classList.toggle('active',active);b.setAttribute('aria-pressed',active?'true':'false')});
  $('#stockSinglePanel').hidden=state.stockMode!=='single';
  $('#stockSizedPanel').hidden=state.stockMode!=='sizes';
  if(state.stockMode==='sizes'&&!$$('.stock-row','#stockSizeList').length){['S','M','L','XL'].forEach(size=>addStockRow(size,0))}
  updateStockTotal()
}
function updateStockTotal(){
  let total=0;
  if(state.stockMode==='single')total=Math.max(0,Number.parseInt($('#stockSingleQty').value||'0',10)||0);
  else total=$$('.stock-qty','#stockSizeList').reduce((n,x)=>n+Math.max(0,Number.parseInt(x.value||'0',10)||0),0);
  $('#stockTotal').textContent=total
}
async function openStock(id){
  const p=state.products.find(x=>x.id===id);
  $('#stockProductId').value=id;$('#stockDialogTitle').textContent=p?`${p.name} stock`:'Stock';
  $('#stockSizeList').replaceChildren();$('#stockStatus').textContent='';$('#stockSingleQty').value=Number(p?.stock||0);
  try{
    const data=await request(`/api/admin/products/${id}/stock`);
    $('#stockSingleQty').value=Number(data.stock||0);
    const sizes=Array.isArray(data.sizes)?data.sizes:[];
    sizes.forEach(x=>addStockRow(x.label,x.quantity));
    setStockMode(data.has_sizes?'sizes':'single');
    $('#stockDialog').showModal()
  }catch(error){toast(error.message)}
}
async function saveStock(event){
  event.preventDefault();
  const id=Number($('#stockProductId').value),status=$('#stockStatus');
  const sizes=$$('.stock-row','#stockSizeList').map(row=>({label:$('.stock-label',row)?.value.trim()||'',quantity:Number.parseInt($('.stock-qty',row)?.value||'0',10)||0}));
  const payload=state.stockMode==='single'
    ?{mode:'single',stock:Math.max(0,Number.parseInt($('#stockSingleQty').value||'0',10)||0),sizes:[]}
    :{mode:'sizes',stock:0,sizes};
  status.className='dialog-status';status.textContent='Saving stock…';
  try{
    const data=await request(`/api/admin/products/${id}/stock`,{method:'PUT',body:JSON.stringify(payload)});
    status.classList.add('success');status.textContent=data.message;
    await Promise.all([loadInventory(),loadDashboard()]);
    setTimeout(()=>$('#stockDialog').close(),180);toast(`Stock updated · ${data.stock} total`)
  }catch(error){status.classList.add('error');status.textContent=error.message}
}
function openDelete(id){const p=state.products.find(x=>x.id===id);$('#deleteProductId').value=id;$('#deleteProductCopy').textContent=p?`Delete “${p.name}” and all of its size quantities? This cannot be undone.`:'Delete this product?';$('#deleteDialog').showModal()}
async function deleteProduct(event){event.preventDefault();const id=Number($('#deleteProductId').value);try{const data=await request(`/api/admin/products/${id}`,{method:'DELETE',body:'{}'});$('#deleteDialog').close();toast(data.message);await Promise.all([loadInventory(),loadDashboard()])}catch(error){toast(error.message)}}

async function loadSales(page=state.salesPage){state.salesPage=page;const body=$('#salesRows');body.innerHTML='<tr><td colspan="9" class="empty-admin">Loading sales…</td></tr>';try{const data=await request(`/api/admin/sales?page=${page}&limit=20`),items=data.sales||[];$('#salesMeta').textContent=`${data.total||0} sale${Number(data.total||0)===1?'':'s'}`;body.innerHTML=items.length?items.map(s=>`<tr><td><b>${escapeHTML(s.receipt_no||'Pending')}</b></td><td>${escapeHTML(s.created_at)}</td><td>${escapeHTML(s.customer_name||'Walk-in')}</td><td>${escapeHTML(s.cashier)}</td><td>${escapeHTML(s.payment_method)}</td><td>${money(s.total)}</td><td class="margin-positive">${money(s.profit)}</td><td><span class="status-pill ${s.status==='completed'?'safe':''}">${escapeHTML(s.status)}</span></td><td><button class="table-action" data-receipt="${s.id}" type="button">Receipt</button></td></tr>`).join(''):'<tr><td colspan="9" class="empty-admin">No sales have been recorded yet.</td></tr>';renderPager($('#salesPager'),data.page||1,data.pages||0,p=>loadSales(p));$$('[data-receipt]').forEach(b=>b.onclick=()=>openReceipt(Number(b.dataset.receipt)))}catch(error){body.innerHTML=`<tr><td colspan="9" class="empty-admin">${escapeHTML(error.message)}</td></tr>`}}
function renderPager(root,page,pages,onPage){root.replaceChildren();if(pages<=1)return;const mk=(label,p,active=false,disabled=false)=>{const b=document.createElement('button');b.type='button';b.textContent=label;b.classList.toggle('active',active);b.disabled=disabled;b.onclick=()=>onPage(p);root.append(b)};mk('‹',Math.max(1,page-1),false,page<=1);for(let p=Math.max(1,page-2);p<=Math.min(pages,page+2);p++)mk(String(p),p,p===page);mk('›',Math.min(pages,page+1),false,page>=pages)}
async function openReceipt(id){try{const data=await request(`/api/admin/sales/${id}/receipt`),r=data.receipt||{},items=r.items||[];$('#receiptContent').innerHTML=`<div class="receipt-sheet"><div class="receipt-brand">LIXR</div><div class="receipt-meta"><span>Receipt</span><b>${escapeHTML(r.receipt_no)}</b><span>Date</span><b>${escapeHTML(r.created_at)}</b><span>Cashier</span><b>${escapeHTML(r.cashier)}</b><span>Customer</span><b>${escapeHTML(r.customer_name||'Walk-in')}</b></div><table class="receipt-items"><thead><tr><th>Item</th><th>Qty</th><th>Total</th></tr></thead><tbody>${items.map(x=>`<tr><td>${escapeHTML(x.name)}<br><small>${escapeHTML([x.sku,x.size].filter(Boolean).join(' · '))}</small></td><td>${x.quantity}</td><td>${money(x.line_total)}</td></tr>`).join('')||'<tr><td colspan="3">No line items saved.</td></tr>'}</tbody></table><div class="receipt-totals"><div><span>Subtotal</span><b>${money(r.subtotal)}</b></div><div><span>Discount</span><b>${money(r.discount)}</b></div><div class="grand"><span>Total</span><b>${money(r.total)}</b></div></div></div>`;$('#receiptDialog').showModal()}catch(error){toast(error.message)}}

async function loadStaff(){try{const data=await request('/api/admin/staff');state.staff=data.staff||[];renderStaff()}catch(error){$('#staffGrid').innerHTML=`<div class="empty-admin">${escapeHTML(error.message)}</div>`}}
function renderStaff(){const grid=$('#staffGrid');if(!state.staff.length){grid.innerHTML='<div class="empty-admin">No staff accounts.</div>';return}grid.innerHTML=state.staff.map(s=>{const locked=state.session?.role==='admin'&&(s.role==='developer'||s.protected);return`<article class="staff-card"><div class="staff-card-head"><div class="staff-avatar">${escapeHTML(initials(s.display_name))}</div><span class="status-pill ${s.active?'safe':'out'}">${s.active?'Active':'Inactive'}</span></div><h3>${escapeHTML(s.display_name)}</h3><p>@${escapeHTML(s.username)}${s.email?` · ${escapeHTML(s.email)}`:''}</p><div class="staff-badges"><span class="staff-badge">${escapeHTML(s.role)}</span>${s.protected?'<span class="staff-badge">Protected</span>':''}</div><footer><small>${s.last_login_at?`Last login ${escapeHTML(s.last_login_at)}`:'No login recorded'}</small><button class="table-edit" type="button" data-edit-staff="${s.id}" ${locked?'disabled':''}>${locked?'Protected':'Edit'}</button></footer></article>`}).join('');$$('[data-edit-staff]:not([disabled])').forEach(b=>b.onclick=()=>openStaff(Number(b.dataset.editStaff)))}
function openStaff(id=0){const s=id?state.staff.find(x=>x.id===id):null;$('#staffDialogTitle').textContent=s?'Edit account':'Add account';$('#staffId').value=s?.id||'';$('#staffUsername').value=s?.username||'';$('#staffUsername').readOnly=Boolean(s);$('#staffEmail').value=s?.email||'';$('#staffDisplayName').value=s?.display_name||'';$('#staffRole').value=s?.role||'cashier';$('#staffPassword').value='';$('#staffPassword').required=!s;$('#staffPasswordLabel').textContent=s?'New password (optional)':'Password';$('#staffActive').checked=s?Boolean(s.active):true;$('#staffStatus').textContent='';const dev=$('#staffRole option[value="developer"]');dev.disabled=state.session?.role==='admin';if(state.session?.role==='admin'&&!s)$('#staffRole').value='cashier';$('#staffDialog').showModal()}
async function saveStaff(event){event.preventDefault();const id=Number($('#staffId').value||0),status=$('#staffStatus'),payload={username:$('#staffUsername').value.trim(),email:$('#staffEmail').value.trim(),display_name:$('#staffDisplayName').value.trim(),role:$('#staffRole').value,password:$('#staffPassword').value,active:$('#staffActive').checked};status.className='dialog-status';status.textContent='Saving account…';try{const data=await request(id?`/api/admin/staff/${id}`:'/api/admin/staff',{method:id?'PATCH':'POST',body:JSON.stringify(payload)});status.classList.add('success');status.textContent=data.message;await Promise.all([loadStaff(),loadDashboard()]);setTimeout(()=>$('#staffDialog').close(),200);toast(data.message)}catch(error){status.classList.add('error');status.textContent=error.message}}

function setInboxTab(tab){
  state.inboxTab=tab==='chat'?'chat':'forms';
  $$('[data-inbox-tab]').forEach(b=>{const active=b.dataset.inboxTab===state.inboxTab;b.classList.toggle('active',active);b.setAttribute('aria-selected',active?'true':'false')});
  $$('[data-inbox-panel]').forEach(panel=>{const active=panel.dataset.inboxPanel===state.inboxTab;panel.hidden=!active;panel.classList.toggle('active',active)});
  if(state.inboxTab==='chat')loadChats()
}
async function loadContacts(){
  const list=$('#contactList');list.innerHTML='<div class="empty-admin">Loading contact forms…</div>';
  try{
    const data=await request('/api/admin/contacts'),items=Array.isArray(data.messages)?data.messages:[];
    $('#contactFormCount').textContent=items.length;
    list.innerHTML=items.length?items.map(x=>`<article class="contact-record"><div><h3>${escapeHTML(x.name||'Customer')}</h3><p>${escapeHTML(x.email||'No email')}</p><div class="record-actions"><select class="small-select" data-contact-status="${x.id}"><option value="new" ${x.status==='new'?'selected':''}>New</option><option value="read" ${x.status==='read'?'selected':''}>Read</option><option value="resolved" ${x.status==='resolved'?'selected':''}>Resolved</option></select></div></div><div><h3>${escapeHTML(x.subject||'Message')}</h3><p>${escapeHTML(x.message||'')}</p></div><time>${escapeHTML(x.created_at||'')}</time></article>`).join(''):'<div class="empty-admin">No contact-form messages yet. New messages sent from the storefront will appear here.</div>';
    $$('[data-contact-status]').forEach(s=>s.onchange=()=>updateContactStatus(Number(s.dataset.contactStatus),s.value))
  }catch(error){$('#contactFormCount').textContent='—';list.innerHTML=`<div class="empty-admin error-state">${escapeHTML(error.message)}</div>`}
}
async function updateContactStatus(id,status){try{const data=await request(`/api/admin/contacts/${id}`,{method:'PATCH',body:JSON.stringify({status})});toast(data.message);await Promise.all([loadContacts(),loadDashboard()])}catch(error){toast(error.message);loadContacts()}}
async function loadChats(silent=false){
  const root=$('#adminChatThreads');
  if(!silent&&!state.chatThreads.length)root.innerHTML='<div class="empty-admin">Loading conversations…</div>';
  try{
    const data=await request('/api/admin/chats'),items=Array.isArray(data.threads)?data.threads:[];
    state.chatThreads=items;$('#contactChatCount').textContent=items.length;
    root.innerHTML=items.length?items.map(t=>`<button class="admin-chat-thread ${t.id===state.chatThread?'active':''}" type="button" data-chat-thread="${escapeHTML(t.id)}"><span class="chat-thread-top"><b>${escapeHTML(t.name||'Guest')}</b>${t.needs_reply?'<i>Needs reply</i>':''}</span><span class="chat-thread-email">${escapeHTML(t.email||'Guest conversation')}</span><span class="chat-thread-preview">${escapeHTML(t.last_message||'Conversation started')}</span><small>${escapeHTML(t.last_created_at||t.updated_at||'')}</small></button>`).join(''):'<div class="empty-admin">No store chats yet. Conversations started from the storefront will appear here.</div>';
    $$('[data-chat-thread]').forEach(b=>b.onclick=()=>loadAdminChat(b.dataset.chatThread));
    if(items.length&&!state.chatThread)loadAdminChat(items[0].id,true)
  }catch(error){$('#contactChatCount').textContent='—';if(!silent)root.innerHTML=`<div class="empty-admin error-state">${escapeHTML(error.message)}</div>`}
}
function resizeAdminChat(){
  const input=$('#adminChatInput');if(!input)return;
  input.style.height='auto';input.style.height=`${Math.min(150,Math.max(44,input.scrollHeight))}px`;input.style.overflowY=input.scrollHeight>150?'auto':'hidden'
}
async function loadAdminChat(threadID,silent=false){
  if(!threadID)return;
  try{
    const data=await request(`/api/admin/chats/${encodeURIComponent(threadID)}/messages`);
    state.chatThread=threadID;
    $('#adminChatName').textContent=data.name||'Guest';
    $('#adminChatEmail').textContent=data.email||'Guest conversation';
    $('#adminChatState').textContent='Store chat';
    const items=Array.isArray(data.messages)?data.messages:[];
    const root=$('#adminChatMessages');
    root.innerHTML=items.length?items.map(m=>`<div class="admin-chat-bubble ${m.sender==='staff'?'staff':'visitor'}"><p>${escapeHTML(m.body)}</p><small>${escapeHTML(m.created_at||'')}</small></div>`).join(''):'<div class="empty-admin">No messages in this conversation.</div>';
    root.scrollTop=root.scrollHeight;
    $('#adminChatInput').disabled=false;$('#adminChatForm button[type="submit"]').disabled=false;
    $$('[data-chat-thread]').forEach(b=>b.classList.toggle('active',b.dataset.chatThread===threadID))
  }catch(error){if(!silent)toast(error.message)}
}
async function sendAdminChat(event){
  event.preventDefault();
  if(!state.chatThread)return;
  const input=$('#adminChatInput'),button=$('#adminChatForm button[type="submit"]'),message=input.value.trim();
  if(!message)return;
  button.disabled=true;
  try{
    const data=await request(`/api/admin/chats/${encodeURIComponent(state.chatThread)}/messages`,{method:'POST',body:JSON.stringify({message})});
    input.value='';resizeAdminChat();toast(data.message||'Reply sent.');
    await Promise.all([loadAdminChat(state.chatThread,true),loadChats(true),loadDashboard()])
  }catch(error){toast(error.message)}
  finally{button.disabled=false;input.focus()}
}
async function loadRatings(){const grid=$('#ratingsGrid');grid.innerHTML='<div class="empty-admin">Loading reviews…</div>';try{const data=await request('/api/admin/ratings'),items=data.ratings||[];grid.innerHTML=items.length?items.map(x=>`<article class="record-card"><b>${escapeHTML(x.display_name)}</b>${x.email?`<span class="review-email">${escapeHTML(x.email)}</span>`:''}<div class="record-stars">${'★'.repeat(x.stars)}${'☆'.repeat(5-x.stars)}</div><p>“${escapeHTML(x.comment)}”</p><footer><small>${escapeHTML(x.created_at)} · ${x.visible?'Visible':'Hidden'}</small><button class="table-action" data-rating-toggle="${x.id}" data-visible="${x.visible?'1':'0'}" type="button">${x.visible?'Hide':'Show'}</button></footer></article>`).join(''):'<div class="empty-admin">No customer reviews yet.</div>';$$('[data-rating-toggle]').forEach(b=>b.onclick=()=>toggleRating(Number(b.dataset.ratingToggle),b.dataset.visible!=='1'))}catch(error){grid.innerHTML=`<div class="empty-admin">${escapeHTML(error.message)}</div>`}}
async function toggleRating(id,visible){try{const data=await request(`/api/admin/ratings/${id}`,{method:'PATCH',body:JSON.stringify({visible})});toast(data.message);await Promise.all([loadRatings(),loadDashboard()])}catch(error){toast(error.message)}}

async function loadDatabase(table=state.databaseTable,page=state.databasePage){
  $('#databaseTables').innerHTML='<div class="empty-admin">Loading tables…</div>';
  $('#databaseRows').innerHTML='<tr><td class="empty-admin">Loading records…</td></tr>';
  try{
    const qs=new URLSearchParams();if(table)qs.set('table',table);qs.set('page',page);qs.set('limit','50');
    const data=await request(`/api/admin/database?${qs}`);
    state.databaseTable=data.table||'';state.databasePage=data.page||1;
    $('#databaseAccess').textContent=data.can_modify?'Developer access':'Read only';
    $('#developerDatabase').hidden=!data.can_modify;
    renderDatabaseTables(Array.isArray(data.tables)?data.tables:[],state.databaseTable);
    renderDatabaseRows(data);
    renderPager($('#databasePager'),data.page||1,data.pages||0,p=>{state.databasePage=p;loadDatabase(state.databaseTable,p)})
  }catch(error){
    $('#databaseTables').innerHTML=`<div class="empty-admin error-state">${escapeHTML(error.message)}</div>`;
    $('#databaseHead').innerHTML='';$('#databaseRows').innerHTML='<tr><td class="empty-admin">Store records could not be opened.</td></tr>';
    $('#databaseTableMeta').textContent='Unavailable';toast(error.message)
  }
}
function renderDatabaseTables(tables,current){const root=$('#databaseTables');root.innerHTML=tables.map(t=>`<button class="database-table-button ${t.name===current?'active':''}" type="button" data-db-table="${escapeHTML(t.name)}"><span>${escapeHTML(t.name)}</span><span>${t.rows}</span></button>`).join('');$$('[data-db-table]').forEach(b=>b.onclick=()=>{state.databaseTable=b.dataset.dbTable;state.databasePage=1;loadDatabase(state.databaseTable,1)})}
function renderDatabaseRows(data){const cols=data.columns||[],rows=data.rows||[];$('#databaseTableTitle').textContent=data.table||'Database';$('#databaseTableMeta').textContent=`${data.total||0} row${Number(data.total||0)===1?'':'s'}`;$('#databaseHead').innerHTML=cols.length?`<tr>${cols.map(c=>`<th>${escapeHTML(c)}</th>`).join('')}</tr>`:'';$('#databaseRows').innerHTML=rows.length?rows.map(row=>`<tr>${cols.map(c=>`<td title="${escapeHTML(row[c]??'')}">${escapeHTML(row[c]??'')}</td>`).join('')}</tr>`).join(''):`<tr><td class="empty-admin" colspan="${Math.max(1,cols.length)}">No rows in this table.</td></tr>`}
async function runDatabaseSQL(){const sql=$('#databaseSQL').value.trim(),out=$('#databaseResult');if(!sql)return;out.textContent='Running…';try{const data=await request('/api/admin/database/query',{method:'POST',body:JSON.stringify({sql})});out.textContent=JSON.stringify(data.rows??{message:data.message,affected:data.affected},null,2);await loadDatabase(state.databaseTable,state.databasePage)}catch(error){out.textContent=error.message}}
async function resetDatabase(event){event.preventDefault();const status=$('#resetStatus');status.className='dialog-status';status.textContent='Resetting store data…';try{const data=await request('/api/admin/database/reset',{method:'POST',body:JSON.stringify({confirm:$('#resetConfirm').value})});status.classList.add('success');status.textContent=data.message;setTimeout(()=>$('#resetDialog').close(),240);toast(data.message);await Promise.all([loadDatabase('',1),loadInventory(),loadDashboard()])}catch(error){status.classList.add('error');status.textContent=error.message}}

async function loadSystem(){
  const grid=$('#systemGrid');grid.innerHTML='<div class="empty-admin">Reading server and device details…</div>';
  try{
    const data=await request('/api/admin/system'),info=data.system||{},storage=Array.isArray(info.storage)?info.storage:[];
    const cards=[['Computer name',info.computer_name],['Operating system',info.operating_system],['Processor',info.cpu],['Graphics',info.gpu],['Memory',info.memory],['Internet',info.internet],['Logical processors',info.logical_processors],['Google sign-in',info.google_sign_in?'Ready':'Not set'],['Email updates',info.email_service?'Ready':'Not set']];
    grid.innerHTML=cards.map(([k,v])=>`<article class="system-card"><span>${escapeHTML(k)}</span><strong>${escapeHTML(v??'—')}</strong></article>`).join('')+`<article class="system-card storage-card"><span>Storage</span><div class="storage-list">${storage.length?storage.map(d=>`<div class="storage-row"><b>${escapeHTML(d.drive||'Drive')}</b><span>${Number(d.free_gb||0).toFixed(1)} GB free / ${Number(d.size_gb||0).toFixed(1)} GB</span></div>`).join(''):'<strong>No fixed-drive details were returned by this computer.</strong>'}</div></article>`
  }catch(error){grid.innerHTML=`<div class="empty-admin error-state">${escapeHTML(error.message)}</div>`;toast(error.message)}
}

function bindDragDrop(){let depth=0;document.addEventListener('dragenter',e=>{if(![...e.dataTransfer?.items||[]].some(i=>i.kind==='file'))return;e.preventDefault();depth++;$('#dropScreen').hidden=false});document.addEventListener('dragover',e=>{if(e.dataTransfer?.types?.includes('Files'))e.preventDefault()});document.addEventListener('dragleave',()=>{depth=Math.max(0,depth-1);if(!depth)$('#dropScreen').hidden=true});document.addEventListener('drop',async e=>{if(!e.dataTransfer?.files?.length)return;e.preventDefault();depth=0;$('#dropScreen').hidden=true;const file=[...e.dataTransfer.files].find(f=>f.type.startsWith('image/'));if(file)await setPendingImage(file)})}
function initTooltips(){const layer=$('#tooltipLayer');if(!layer)return;document.addEventListener('mouseover',e=>{const t=e.target.closest('[data-tooltip]');if(!t)return;layer.textContent=t.dataset.tooltip;layer.hidden=false;const r=t.getBoundingClientRect();layer.style.left=`${r.left+r.width/2}px`;layer.style.top=`${r.bottom+8}px`});document.addEventListener('mouseout',e=>{if(e.target.closest?.('[data-tooltip]'))layer.hidden=true})}

let searchTimer=0;
$('#adminProductSearch').addEventListener('input',()=>{clearTimeout(searchTimer);searchTimer=setTimeout(loadInventory,220)});
$('#addProductButton').addEventListener('click',()=>openProduct());
$('#productForm').addEventListener('submit',saveProduct);
$('#productImageUrl').addEventListener('input',()=>{if(!state.pendingImage)renderImagePreview($('#productImageUrl').value.trim())});
$('#productImageFile').addEventListener('change',e=>setPendingImage(e.target.files?.[0]));
$('#stockForm').addEventListener('submit',saveStock);
$('#stockSingleQty').addEventListener('input',updateStockTotal);
$$('[data-stock-mode]').forEach(b=>b.addEventListener('click',()=>setStockMode(b.dataset.stockMode)));
$('#addSizeButton').addEventListener('click',()=>addStockRow());
$('#deleteForm').addEventListener('submit',deleteProduct);
$('#addStaffButton').addEventListener('click',()=>openStaff());
$('#staffForm').addEventListener('submit',saveStaff);
$$('[data-inbox-tab]').forEach(b=>b.addEventListener('click',()=>setInboxTab(b.dataset.inboxTab)));
$('#adminChatForm').addEventListener('submit',sendAdminChat);
$('#adminChatInput').addEventListener('input',resizeAdminChat);
$('#adminChatInput').addEventListener('keydown',e=>{if(e.key==='Enter'&&!e.shiftKey){e.preventDefault();$('#adminChatForm').requestSubmit()}});
$('#runDatabaseSQL').addEventListener('click',runDatabaseSQL);
$('#resetDatabaseButton').addEventListener('click',()=>{$('#resetConfirm').value='';$('#resetStatus').textContent='';$('#resetDialog').showModal()});
$('#resetForm').addEventListener('submit',resetDatabase);
$$('[data-close-dialog]').forEach(b=>b.addEventListener('click',()=>b.closest('dialog').close()));
$$('.admin-nav').forEach(b=>b.addEventListener('click',()=>showView(b.dataset.view)));
$$('[data-open-view]').forEach(b=>b.addEventListener('click',()=>showView(b.dataset.openView)));
$('#adminMenuButton').addEventListener('click',()=>$('#adminSidebar').classList.toggle('open'));
$('#themeButton').addEventListener('click',()=>setTheme(document.documentElement.dataset.theme==='dark'?'light':'dark'));
$('#adminLogout').addEventListener('click',async()=>{try{await request('/api/auth/logout',{method:'POST',body:'{}'})}catch{}location.href='/access'});
window.addEventListener('resize',()=>{if(state.dashboard)drawDashboardCharts(state.dashboard);resizeAdminChat()});
bindDragDrop();initTooltips();initTheme();setStockMode('single');setInboxTab('forms');bootstrap();
