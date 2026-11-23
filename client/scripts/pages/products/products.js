const $=(s,r=document)=>r.querySelector(s); const $$=(s,r=document)=>[...r.querySelectorAll(s)];
const state={products:[],filtered:[]};
const money=v=>new Intl.NumberFormat('en-PH',{style:'currency',currency:'PHP',maximumFractionDigits:0}).format(Number(v||0));
const escapeHTML=v=>String(v??'').replace(/[&<>'"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]));
function setTheme(theme){const n=theme==='dark'?'dark':'light';document.documentElement.dataset.theme=n;try{localStorage.setItem('lixr-theme',n)}catch{}}
function initTheme(){let s='';try{s=localStorage.getItem('lixr-theme')||''}catch{}setTheme(s||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'))}
function toggleTheme(){setTheme(document.documentElement.dataset.theme==='dark'?'light':'dark')}
function updateScroll(){const y=scrollY;$('#siteHeader').classList.toggle('scrolled',y>18);const max=Math.max(1,document.documentElement.scrollHeight-innerHeight);$('#scrollProgress').style.transform=`scaleX(${Math.min(1,y/max)})`}
function saleInfo(p){const retail=Number(p.retail_price||p.compare_price||p.price||0),sale=Number(p.sale_price||p.price||retail);const active=retail>0&&sale>0&&sale<retail;return{retail,sale,active,percent:active?Math.max(1,Math.round((1-sale/retail)*100)):0}}
function stockKey(p){if(p.availability==='Sold out')return'out';if(p.availability==='Low stock')return'low';return'available'}
function availableSizes(p){return (Array.isArray(p.sizes)?p.sizes:[]).filter(x=>Number(x.quantity||0)>0)}
function sizeSummary(p){
  if(!p.has_sizes)return '';
  const sizes=availableSizes(p);
  return sizes.length?`<div class="product-sizes" aria-label="Available sizes">${sizes.map(x=>`<span>${escapeHTML(x.label)}</span>`).join('')}</div>`:'<div class="product-sizes sold-out-sizes"><span>No sizes available</span></div>'
}
function render(){
  const grid=$('#allProducts');grid.replaceChildren();
  for(const p of state.filtered){
    const sale=saleInfo(p),card=document.createElement('article');card.className='product-card';
    card.innerHTML=`<div class="product-media"><img src="${escapeHTML(p.image_url)}" alt="${escapeHTML(p.image_alt||p.name)}" loading="lazy">${p.badge?`<span class="product-badge">${escapeHTML(p.badge)}</span>`:''}${sale.active?`<span class="sale-burst">${sale.percent}%<br>OFF</span>`:''}<button class="product-open" data-product-id="${p.id}" aria-label="View ${escapeHTML(p.name)}"></button></div><div class="product-info"><h3>${escapeHTML(p.name)}</h3><span class="price-wrap"><span class="price">${money(sale.sale||p.price)}</span>${sale.active?`<span class="compare-price">${money(sale.retail)}</span>`:''}</span><span class="subtitle">${escapeHTML(p.subtitle)}</span>${sizeSummary(p)}<span class="availability ${stockKey(p)==='low'?'low':stockKey(p)==='out'?'out':''}">${escapeHTML(p.availability)} · ${Number(p.stock||0)} pcs</span></div>`;
    grid.append(card)
  }
  if(!state.filtered.length)grid.innerHTML='<p>No products match these filters.</p>';
  $('#catalogCount').textContent=`${state.filtered.length} of ${state.products.length} products`
}
function filter(){const q=$('#productSearch').value.trim().toLowerCase(),c=$('#collectionFilter').value,a=$('#availabilityFilter').value;state.filtered=state.products.filter(p=>(!q||[p.name,p.subtitle,p.category,p.collection,p.color].join(' ').toLowerCase().includes(q))&&(!c||p.collection===c)&&(!a||stockKey(p)===a));render()}
function openProduct(p){
  const sale=saleInfo(p),sizes=availableSizes(p),sizeBox=$('#dialogProductSizes');
  $('#dialogProductImage').src=p.image_url;$('#dialogProductImage').alt=p.image_alt||p.name;
  $('#dialogProductCollection').textContent=p.collection||p.category;$('#dialogProductName').textContent=p.name;
  $('#dialogProductSubtitle').textContent=p.subtitle;$('#dialogProductDescription').textContent=p.description;
  $('#dialogProductPrice').textContent=sale.active?`${money(sale.sale)} · ${sale.percent}% off`:money(p.price);
  $('#dialogProductAvailability').textContent=`${p.availability} · ${Number(p.stock||0)} pcs`;
  sizeBox.hidden=!p.has_sizes;
  sizeBox.innerHTML=p.has_sizes?`<small>Available sizes</small><div>${sizes.length?sizes.map(x=>`<span>${escapeHTML(x.label)} <b>${Number(x.quantity)} left</b></span>`).join(''):'<span>Currently sold out in all sizes</span>'}</div>`:'';
  $('#productDialog').showModal()
}
async function load(){try{const r=await fetch('/api/products',{headers:{Accept:'application/json'}}),d=await r.json();if(!r.ok)throw new Error(d.error||'Products unavailable');state.products=d.products||[];state.filtered=[...state.products];const collections=[...new Set(state.products.map(p=>p.collection).filter(Boolean))].sort();for(const c of collections){const o=document.createElement('option');o.value=c;o.textContent=c;$('#collectionFilter').append(o)}const param=new URLSearchParams(location.search).get('collection');if(param&&collections.includes(param))$('#collectionFilter').value=param;filter()}catch(e){$('#allProducts').innerHTML=`<p>${escapeHTML(e.message)}</p>`;$('#catalogCount').textContent='Products unavailable.'}}
function trackPageVisit(){let session='';try{session=localStorage.getItem('lixr-visit-session')||'';if(!session){session=globalThis.crypto?.randomUUID?.()||`visit-${Date.now()}-${Math.random().toString(36).slice(2)}`;localStorage.setItem('lixr-visit-session',session)}}catch{session=`visit-${Date.now()}-${Math.random().toString(36).slice(2)}`}fetch('/api/analytics/visit',{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','Accept':'application/json'},body:JSON.stringify({session,path:location.pathname})}).catch(()=>{})}
initTheme();trackPageVisit();$('#themeToggle').addEventListener('click',toggleTheme);$('#footerTheme').addEventListener('click',toggleTheme);window.addEventListener('scroll',updateScroll,{passive:true});updateScroll();['productSearch','collectionFilter','availabilityFilter'].forEach(id=>$('#'+id).addEventListener(id==='productSearch'?'input':'change',filter));$('#clearFilters').addEventListener('click',()=>{$('#productSearch').value='';$('#collectionFilter').value='';$('#availabilityFilter').value='';filter()});$('#allProducts').addEventListener('click',e=>{const b=e.target.closest('[data-product-id]');if(!b)return;const p=state.products.find(x=>String(x.id)===b.dataset.productId);if(p)openProduct(p)});$$('[data-close-dialog]').forEach(b=>b.addEventListener('click',()=>b.closest('dialog').close()));$('#productDialog').addEventListener('click',e=>{if(e.target===e.currentTarget)e.currentTarget.close()});$('#year').textContent=new Date().getFullYear();load();
