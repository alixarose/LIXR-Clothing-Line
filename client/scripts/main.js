const API = {
  async request(path, options = {}) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 12000);
    try {
      const response = await fetch(path, {
        credentials: 'same-origin',
        headers: { Accept: 'application/json', ...(options.body ? { 'Content-Type': 'application/json' } : {}), ...(options.headers || {}) },
        signal: controller.signal,
        ...options,
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data.error || `Request failed (${response.status})`);
      return data;
    } finally {
      clearTimeout(timeout);
    }
  },
  store: () => API.request('/api/store'),
  products: () => API.request('/api/products?featured=1'),
  ratings: (page = 1, limit = 6) => API.request(`/api/ratings?page=${page}&limit=${limit}`),
  trackVisit: payload => API.request('/api/analytics/visit', { method: 'POST', body: JSON.stringify(payload) }),
  submitRating: payload => API.request('/api/ratings', { method: 'POST', body: JSON.stringify(payload) }),
  visitor: () => API.request('/api/visitor/session'),
  visitorLogout: () => API.request('/api/visitor/logout', { method: 'POST', body: '{}' }),
  contact: payload => API.request('/api/contact', { method: 'POST', body: JSON.stringify(payload) }),
  newsletter: email => API.request('/api/newsletter', { method: 'POST', body: JSON.stringify({ email }) }),
  chat: thread => API.request(`/api/chat/messages?thread=${encodeURIComponent(thread)}`),
  sendChat: payload => API.request('/api/chat/messages', { method: 'POST', body: JSON.stringify(payload) }),
};

const STORE_LOCATION = { lat: 10.0372, lng: 124.0651, label: 'Saa, Inabanga, Bohol, Philippines' };
const state = {
  products: [],
  selectedProduct: null,
  visitor: null,
  googleAvailable: false,
  selectedStars: 0,
  map: null,
  routeLayer: null,
  userMarker: null,
  chatThread: '',
  chatPoll: null,
  productPage: 1,
  productPageSize: 8,
  reviewPage: 1,
  reviewPages: 1,
};

const $ = (selector, root = document) => root.querySelector(selector);
const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];
const escapeHTML = value => String(value ?? '').replace(/[&<>'"]/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[char]));
const escapeAttr = escapeHTML;
const GOOGLE_ICON = `<svg class="google-icon" viewBox="0 0 24 24" aria-hidden="true"><path fill="#4285F4" d="M21.8 12.2c0-.7-.1-1.4-.2-2H12v3.8h5.5a4.7 4.7 0 0 1-2 3.1v2.5h3.2c1.9-1.8 3.1-4.4 3.1-7.4Z"/><path fill="#34A853" d="M12 22c2.7 0 5-.9 6.7-2.4l-3.2-2.5c-.9.6-2 1-3.5 1-2.6 0-4.8-1.8-5.6-4.2H3.1v2.6A10 10 0 0 0 12 22Z"/><path fill="#FBBC05" d="M6.4 13.9A6 6 0 0 1 6 12c0-.7.1-1.3.4-1.9V7.5H3.1A10 10 0 0 0 2 12c0 1.6.4 3.1 1.1 4.5l3.3-2.6Z"/><path fill="#EA4335" d="M12 5.9c1.6 0 3 .5 4.1 1.6l3-3A10 10 0 0 0 3.1 7.5l3.3 2.6C7.2 7.7 9.4 5.9 12 5.9Z"/></svg>`;

function money(value) {
  return new Intl.NumberFormat('en-PH', { style: 'currency', currency: 'PHP', maximumFractionDigits: 0 }).format(Number(value || 0));
}

function toast(message) {
  const region = $('#toastRegion');
  if (!region || !message) return;
  const item = document.createElement('div');
  item.className = 'toast';
  item.textContent = message;
  region.append(item);
  setTimeout(() => item.remove(), 3800);
}

function setTheme(theme) {
  const next = theme === 'dark' ? 'dark' : 'light';
  document.documentElement.dataset.theme = next;
  try { localStorage.setItem('lixr-theme', next); } catch {}
}
function toggleTheme() { setTheme(document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark'); }
function initTheme() {
  let saved = '';
  try { saved = localStorage.getItem('lixr-theme') || ''; } catch {}
  setTheme(saved || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'));
}

function headerOffset() {
  return ($('#siteHeader')?.offsetHeight || 0) + 1;
}

function setActiveNav(sectionName) {
  $$('[data-nav-section]').forEach(link => link.classList.toggle('active', link.dataset.navSection === sectionName));
}

function updateActiveNav() {
  const targets = [
    ['top', $('#overview')],
    ['best-sellers', $('#best-sellers')],
    ['about', $('#about')],
    ['reviews', $('#reviews')],
    ['visit', $('#visit')],
  ].filter(([, node]) => node);
  const probe = (window.scrollY || 0) + headerOffset() + 22;
  let active = 'top';
  for (const [name, node] of targets) {
    if (node.offsetTop <= probe) active = name;
  }
  if (window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 12) active = 'visit';
  setActiveNav(active);
}

function updateScrollUI() {
  const y = window.scrollY || document.documentElement.scrollTop;
  $('#siteHeader')?.classList.toggle('scrolled', y > 18);
  const max = Math.max(1, document.documentElement.scrollHeight - window.innerHeight);
  const bar = $('#scrollProgress');
  if (bar) bar.style.transform = `scaleX(${Math.min(1, Math.max(0, y / max))})`;
  updateActiveNav();
}

function scrollToTarget(target, updateHash = true) {
  if (!target) return;
  const isTop = target === document.body || target.id === 'top';
  const top = isTop ? 0 : Math.max(0, target.getBoundingClientRect().top + window.scrollY - headerOffset());
  window.scrollTo({ top, behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
  if (updateHash && target.id && target.id !== 'top') history.replaceState(null, '', `#${target.id}`);
}

function initAnchorNavigation() {
  document.addEventListener('click', event => {
    const link = event.target.closest('a[href^="#"]');
    if (!link) return;
    const href = link.getAttribute('href');
    const target = href === '#top' ? document.body : document.querySelector(href);
    if (!target) return;
    event.preventDefault();
    setMenu(false);
    if (link.dataset.navSection) setActiveNav(link.dataset.navSection);
    if (href === '#top') history.replaceState(null, '', `${location.pathname}${location.search}`);
    scrollToTarget(target, href !== '#top');
  });
}

function initHeroSpotlight() {
  const hero = $('.hero');
  if (!hero || matchMedia('(pointer: coarse)').matches) return;
  hero.addEventListener('pointermove', event => {
    const rect = hero.getBoundingClientRect();
    hero.style.setProperty('--spot-x', `${event.clientX - rect.left}px`);
    hero.style.setProperty('--spot-y', `${event.clientY - rect.top}px`);
  });
  hero.addEventListener('pointerleave', () => {
    hero.style.setProperty('--spot-x', '50%');
    hero.style.setProperty('--spot-y', '50%');
  });
}

function initTooltips() {
  const layer = $('#tooltipLayer');
  if (!layer) return;
  let current = null;
  const hide = () => { current = null; layer.hidden = true; };
  const show = trigger => {
    if (!trigger?.dataset.tooltip || matchMedia('(pointer: coarse)').matches) return;
    current = trigger;
    layer.textContent = trigger.dataset.tooltip;
    layer.hidden = false;
    requestAnimationFrame(() => {
      if (current !== trigger) return;
      const rect = trigger.getBoundingClientRect();
      const tip = layer.getBoundingClientRect();
      let left = rect.left + rect.width / 2;
      left = Math.max(tip.width / 2 + 8, Math.min(window.innerWidth - tip.width / 2 - 8, left));
      let top = rect.bottom + 9;
      if (top + tip.height > window.innerHeight - 8) top = Math.max(8, rect.top - tip.height - 9);
      layer.style.left = `${left}px`;
      layer.style.top = `${top}px`;
    });
  };
  document.addEventListener('pointerover', event => { const trigger = event.target.closest('[data-tooltip]'); if (trigger) show(trigger); });
  document.addEventListener('pointerout', event => { if (current && !event.relatedTarget?.closest?.('[data-tooltip]')) hide(); });
  document.addEventListener('focusin', event => { const trigger = event.target.closest?.('[data-tooltip]'); if (trigger) show(trigger); });
  document.addEventListener('focusout', hide);
  window.addEventListener('scroll', hide, { passive: true });
}

function setMenu(open) {
  const button = $('#menuButton');
  const menu = $('#mobileMenu');
  if (!button || !menu) return;
  button.setAttribute('aria-expanded', String(open));
  menu.hidden = !open;
  document.body.classList.toggle('menu-open', open);
}

function availabilityClass(value) {
  if (value === 'Low stock') return 'low';
  if (value === 'Sold out') return 'out';
  return '';
}

function saleInfo(product) {
  const retail = Number(product.retail_price || product.compare_price || product.price || 0);
  const sale = Number(product.sale_price || product.price || retail);
  const active = retail > 0 && sale > 0 && sale < retail;
  return { retail, sale, active, percent: active ? Math.max(1, Math.round((1 - sale / retail) * 100)) : 0 };
}

function productPageSize() {
  return matchMedia('(max-width: 980px)').matches ? 4 : 8;
}

function renderPager(container, page, pages, onChange, label) {
  if (!container) return;
  container.replaceChildren();
  container.hidden = pages <= 1;
  if (pages <= 1) return;
  const make = (text, target, disabled = false, current = false) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.textContent = text;
    button.disabled = disabled;
    button.className = current ? 'active' : '';
    if (current) button.setAttribute('aria-current', 'page');
    button.setAttribute('aria-label', current ? `${label} page ${target}, current page` : `${label} page ${target}`);
    button.addEventListener('click', () => onChange(target));
    return button;
  };
  container.append(make('←', Math.max(1, page - 1), page === 1));
  const from = Math.max(1, page - 2);
  const to = Math.min(pages, from + 4);
  for (let value = Math.max(1, to - 4); value <= to; value++) container.append(make(String(value), value, false, value === page));
  container.append(make('→', Math.min(pages, page + 1), page === pages));
}

function renderProducts(products) {
  const grid = $('#productGrid');
  if (!grid) return;
  const pageSize = productPageSize();
  state.productPageSize = pageSize;
  const pages = Math.max(1, Math.ceil(products.length / pageSize));
  state.productPage = Math.min(Math.max(1, state.productPage), pages);
  const visible = products.slice((state.productPage - 1) * pageSize, state.productPage * pageSize);
  grid.replaceChildren();
  if (!products.length) {
    grid.innerHTML = '<p>No featured pieces are available right now.</p>';
    $('#productStatus').textContent = 'Best sellers are being refreshed.';
    renderPager($('#productPager'), 1, 1, () => {}, 'Best sellers');
    return;
  }
  for (const product of visible) {
    const sale = saleInfo(product);
    const article = document.createElement('article');
    article.className = 'product-card';
    article.innerHTML = `
      <div class="product-media">
        <img src="${escapeAttr(product.image_url)}" alt="${escapeAttr(product.image_alt || product.name)}" loading="lazy">
        ${product.badge ? `<span class="product-badge">${escapeHTML(product.badge)}</span>` : ''}
        ${sale.active ? `<span class="sale-burst" aria-label="${sale.percent}% off">${sale.percent}%<br>OFF</span>` : ''}
        <button class="product-open" type="button" data-product-id="${product.id}" aria-label="View ${escapeAttr(product.name)}">View</button>
      </div>
      <div class="product-info">
        <h3>${escapeHTML(product.name)}</h3>
        <span class="price-wrap"><span class="price">${money(sale.sale || product.price)}</span>${sale.active ? `<span class="compare-price">${money(sale.retail)}</span>` : ''}</span>
        <span class="subtitle">${escapeHTML(product.subtitle)}</span>
        <span class="availability ${availabilityClass(product.availability)}">${escapeHTML(product.availability)}${Number.isFinite(Number(product.stock)) ? ` · ${Number(product.stock)} pcs` : ''}</span>
      </div>`;
    grid.append(article);
  }
  const start = (state.productPage - 1) * pageSize + 1;
  const finish = Math.min(products.length, state.productPage * pageSize);
  $('#productStatus').textContent = products.length > pageSize ? `${start}–${finish} of ${products.length} best sellers · current store availability.` : `${products.length} best sellers · current store availability.`;
  renderPager($('#productPager'), state.productPage, pages, page => {
    state.productPage = page;
    renderProducts(state.products);
    scrollToTarget($('#best-sellers'), false);
  }, 'Best sellers');
}

function formatReviewDate(value) {
  if (!value) return '';
  const raw = String(value);
  const date = new Date(raw.includes('T') ? raw : raw.replace(' ', 'T') + 'Z');
  if (Number.isNaN(date.getTime())) return '';
  return new Intl.DateTimeFormat('en-PH', { month: 'short', day: 'numeric', year: 'numeric' }).format(date);
}

function formatSubscriptionDate(value) {
  if (!value) return '';
  const raw = String(value);
  const date = new Date(raw.includes('T') ? raw : raw.replace(' ', 'T') + 'Z');
  if (Number.isNaN(date.getTime())) return raw;
  return new Intl.DateTimeFormat('en-PH', { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

function renderRatings(data) {
  $('#ratingAverage').textContent = Number(data.average || 0).toFixed(1);
  const rounded = Math.round(data.average || 0);
  $('#ratingStars').textContent = '★'.repeat(rounded) + '☆'.repeat(5 - rounded);
  state.reviewPages = Math.max(1, Number(data.pages || 1));
  state.reviewPage = Math.min(state.reviewPages, Math.max(1, Number(data.page || state.reviewPage || 1)));
  const list = $('#reviewList');
  list.replaceChildren();
  for (const review of data.ratings || []) {
    const article = document.createElement('article');
    article.className = 'review-card';
    article.innerHTML = `<div class="review-person"><strong>${escapeHTML(review.display_name)}</strong><span>${'★'.repeat(review.stars)}${'☆'.repeat(5 - review.stars)}</span>${formatReviewDate(review.created_at) ? `<time>${escapeHTML(formatReviewDate(review.created_at))}</time>` : ''}</div><blockquote>“${escapeHTML(review.comment)}”</blockquote>`;
    list.append(article);
  }
  if (!list.children.length) list.innerHTML = '<p class="reviews-empty">No reviews yet. Be the first to share your Lixr experience.</p>';
  renderPager($('#reviewPager'), state.reviewPage, state.reviewPages, page => {
    state.reviewPage = page;
    loadRatings(page, true);
  }, 'Reviews');
}

function openModal(layer) {
  if (!layer) return;
  layer.hidden = false;
  document.body.classList.add('modal-open');
  requestAnimationFrame(() => layer.querySelector('button, a, input, textarea, select')?.focus());
}
function closeModal(layer) {
  if (!layer) return;
  layer.hidden = true;
  if (!$$('.modal-layer:not([hidden])').length) document.body.classList.remove('modal-open');
}

function openProduct(product) {
  state.selectedProduct = product;
  $('#dialogProductImage').src = product.image_url;
  $('#dialogProductImage').alt = product.image_alt || product.name;
  $('#dialogProductCollection').textContent = product.collection || product.category || 'Lixr';
  $('#dialogProductName').textContent = product.name;
  $('#dialogProductSubtitle').textContent = product.subtitle;
  $('#dialogProductDescription').textContent = product.description;
  const sale = saleInfo(product);
  $('#dialogProductPrice').textContent = sale.active ? `${money(sale.sale)} · ${sale.percent}% off` : money(product.price);
  $('#dialogProductAvailability').textContent = `${product.availability}${Number.isFinite(Number(product.stock)) ? ` · ${Number(product.stock)} pcs` : ''}`;
  openModal($('#productDialog'));
}

function googleSignIn(next = '/#reviews') {
  if (!state.googleAvailable) {
    toast('Google sign-in is not available right now.');
    return;
  }
  location.href = `/api/auth/google/start?next=${encodeURIComponent(next)}`;
}

function renderVisitor() {
  const visitor = state.visitor;
  const fields = $('#reviewFields');
  const signIn = $('#reviewButton');
  const avatar = $('#reviewAvatar');
  const profileButton = $('#googleProfileButton');
  const profileStatus = $('#profileStatus');
  const signOut = $('#visitorSignOut');

  if (visitor) {
    fields.disabled = false;
    $('#reviewName').value = visitor.display_name || '';
    $('#reviewEmail').value = visitor.email || '';
    $('#reviewAccountLabel').textContent = 'Signed in with Google';
    $('#reviewAccountName').textContent = visitor.display_name || 'Lixr visitor';
    $('#reviewAccountEmail').textContent = visitor.email || '';
    $('#reviewAuthNote').textContent = 'Your Google profile verifies this review.';
    signIn.hidden = true;
    if (visitor.avatar_url) avatar.innerHTML = `<img src="${escapeAttr(visitor.avatar_url)}" alt="">`;
    else avatar.innerHTML = GOOGLE_ICON;

    $('#profileTitle').textContent = `Welcome, ${(visitor.display_name || 'there').split(' ')[0]}.`;
    $('#profileCopy').textContent = 'Your Google account is connected. You can leave a verified review whenever you like.';
    profileButton.hidden = true;
    signOut.hidden = false;
    profileStatus.textContent = visitor.email || '';
    if (visitor.avatar_url) $('#profileMark').innerHTML = `<img src="${escapeAttr(visitor.avatar_url)}" alt="">`;

    const contactName = $('#contactForm input[name="name"]');
    const contactEmail = $('#contactForm input[name="email"]');
    if (contactName && !contactName.value) contactName.value = visitor.display_name || '';
    if (contactEmail && !contactEmail.value) contactEmail.value = visitor.email || '';
  } else {
    fields.disabled = true;
    state.selectedStars = 0;
    updateStarPicker();
    $('#reviewName').value = '';
    $('#reviewEmail').value = '';
    $('#reviewAccountLabel').textContent = 'Sign in to review';
    $('#reviewAccountName').textContent = 'Google account required';
    $('#reviewAccountEmail').textContent = 'Your name and email will be filled in after sign-in.';
    $('#reviewAuthNote').textContent = state.googleAvailable ? 'Sign in with Google to unlock the form.' : 'Google sign-in is not available right now.';
    signIn.hidden = false;
    signIn.disabled = !state.googleAvailable;
    signIn.innerHTML = state.googleAvailable ? `${GOOGLE_ICON}<span>Continue with Google</span>` : '<span>Google unavailable</span>';
    avatar.innerHTML = GOOGLE_ICON;
    $('#profileMark').innerHTML = '<img src="/assets/brand/lixr-mark.svg" alt="">';
    $('#profileTitle').textContent = 'Your Lixr profile.';
    $('#profileCopy').textContent = 'Sign in with Google to leave a verified review and keep your profile details ready when you return.';
    profileButton.hidden = false;
    profileButton.disabled = !state.googleAvailable;
    $('#googleProfileLabel').textContent = state.googleAvailable ? 'Continue with Google' : 'Google sign-in unavailable';
    signOut.hidden = true;
    profileStatus.textContent = state.googleAvailable ? 'Google sign-in is ready.' : 'Google sign-in is not available right now.';
  }
}

function updateStarPicker() {
  $$('#starPicker [data-stars]').forEach(button => {
    const active = Number(button.dataset.stars) <= state.selectedStars;
    button.classList.toggle('active', active);
    button.setAttribute('aria-checked', String(Number(button.dataset.stars) === state.selectedStars));
  });
}

async function loadVisitor() {
  try {
    const data = await API.visitor();
    state.visitor = data.authenticated ? data.visitor : null;
    state.googleAvailable = Boolean(data.google_available);
  } catch {
    state.visitor = null;
    state.googleAvailable = false;
  }
  renderVisitor();
}

async function loadStore() {
  try {
    const { store } = await API.store();
    $('#announcement').textContent = store.announcement;
    $('#storeAddress').textContent = store.address || STORE_LOCATION.label;
    $('#mapAddress').textContent = store.address || STORE_LOCATION.label;
    $('#storeHours').textContent = store.hours;
    $('#storeContact').textContent = store.email;
    $('#footerAddress').textContent = store.address || STORE_LOCATION.label;
    $('#footerHours').textContent = store.hours;
  } catch {}
}

async function loadProducts(showToast = false) {
  const status = $('#productStatus');
  if (status) status.textContent = 'Refreshing availability…';
  try {
    const data = await API.products();
    state.products = data.products || [];
    state.productPage = Math.min(state.productPage, Math.max(1, Math.ceil(state.products.length / productPageSize())));
    renderProducts(state.products);
    if (showToast) toast('Availability refreshed.');
  } catch (error) {
    if (status) status.textContent = 'Products are unavailable right now. Please try again shortly.';
    if (showToast) toast(error.message);
  }
}

async function loadRatings(page = state.reviewPage, returnToSection = false) {
  try {
    renderRatings(await API.ratings(page, 6));
    if (returnToSection) scrollToTarget($('#reviews'), false);
  } catch {
    $('#reviewList').innerHTML = '<p>Reviews are temporarily unavailable.</p>';
    $('#reviewPager').hidden = true;
  }
}

function initMap() {
  const mapNode = $('#storeMap');
  if (!mapNode || !window.L) {
    $('#routeStatus').textContent = 'The map could not load. Please try again.';
    return;
  }
  const L = window.L;
  const map = L.map(mapNode, { zoomControl: true, scrollWheelZoom: false }).setView([STORE_LOCATION.lat, STORE_LOCATION.lng], 15);
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', { maxZoom: 19, attribution: '&copy; OpenStreetMap contributors' }).addTo(map);
  L.marker([STORE_LOCATION.lat, STORE_LOCATION.lng]).addTo(map).bindPopup(`<strong>Lixr</strong><br>${STORE_LOCATION.label}`).openPopup();
  state.map = map;
  setTimeout(() => map.invalidateSize(), 120);
}

async function drawRoute(userLat, userLng) {
  const status = $('#routeStatus');
  if (!state.map || !window.L) return;
  status.textContent = 'Finding the best route…';
  if (state.userMarker) state.map.removeLayer(state.userMarker);
  state.userMarker = window.L.circleMarker([userLat, userLng], { radius: 7, weight: 3, fillOpacity: 1 }).addTo(state.map).bindPopup('Your current location');
  try {
    const url = `https://router.project-osrm.org/route/v1/driving/${userLng},${userLat};${STORE_LOCATION.lng},${STORE_LOCATION.lat}?overview=full&geometries=geojson&steps=false`;
    const response = await fetch(url, { headers: { Accept: 'application/json' } });
    const data = await response.json();
    if (!response.ok || !data.routes?.[0]) throw new Error('No route found');
    if (state.routeLayer) state.map.removeLayer(state.routeLayer);
    state.routeLayer = window.L.geoJSON(data.routes[0].geometry, { style: { weight: 5, opacity: .82 } }).addTo(state.map);
    state.map.fitBounds(state.routeLayer.getBounds(), { padding: [34, 34] });
    const km = data.routes[0].distance / 1000;
    const minutes = data.routes[0].duration / 60;
    status.textContent = `${km.toFixed(1)} km · about ${Math.round(minutes)} min by road`;
    toast('Route ready from your current location.');
  } catch {
    if (state.routeLayer) state.map.removeLayer(state.routeLayer);
    state.routeLayer = window.L.polyline([[userLat, userLng], [STORE_LOCATION.lat, STORE_LOCATION.lng]], { weight: 3, opacity: .55, dashArray: '7 8' }).addTo(state.map);
    state.map.fitBounds(state.routeLayer.getBounds(), { padding: [34, 34] });
    status.textContent = 'Location found. Road directions are temporarily unavailable.';
    toast('Your location is on the map, but road directions could not be loaded.');
  }
}

async function navigateToStore() {
  const status = $('#routeStatus');
  if (!navigator.geolocation) {
    status.textContent = 'Current-location directions are not supported by this browser.';
    toast('Current location is unavailable in this browser.');
    return;
  }
  if (!window.isSecureContext && !['localhost', '127.0.0.1'].includes(location.hostname)) {
    status.textContent = 'Open this page securely to use your current location.';
    toast('Location access requires a secure page.');
    return;
  }
  try {
    if (navigator.permissions?.query) {
      const permission = await navigator.permissions.query({ name: 'geolocation' });
      if (permission.state === 'denied') {
        status.textContent = 'Location is blocked. Allow location access in your browser, then try again.';
        toast('Location access is blocked in your browser settings.');
        return;
      }
    }
  } catch {}
  status.textContent = 'Allow location access to build your route…';
  navigator.geolocation.getCurrentPosition(
    position => drawRoute(position.coords.latitude, position.coords.longitude),
    error => {
      const messages = {
        1: 'Location permission was not granted. Allow it in your browser, then try again.',
        2: 'Your current location could not be determined. Check location services and try again.',
        3: 'Finding your location took too long. Please try again.',
      };
      status.textContent = messages[error.code] || 'Your location could not be used right now.';
      toast(status.textContent);
    },
    { enableHighAccuracy: false, timeout: 20000, maximumAge: 0 },
  );
}

function openProfile() {
  setMenu(false);
  openModal($('#profileDialog'));
}

function focusContact(subject) {
  closeModal($('#productDialog'));
  const hidden = $('#contactSubject');
  const select = $('#inquiryType');
  if (subject?.startsWith('Availability:')) {
    if (select) select.value = 'Product availability';
    if (hidden) hidden.value = subject;
  }
  const visit = $('#visit');
  if (visit) scrollToTarget(visit);
  setTimeout(() => $('.inline-contact-form textarea')?.focus(), 450);
}

function getChatThread() {
  if (state.chatThread) return state.chatThread;
  try { state.chatThread = localStorage.getItem('lixr-chat-thread') || ''; } catch {}
  return state.chatThread;
}
function saveChatThread(thread) {
  state.chatThread = thread || '';
  try { if (thread) localStorage.setItem('lixr-chat-thread', thread); } catch {}
}
function renderChatMessages(messages) {
  const preview = $('#chatPreview');
  if (!preview) return;
  preview.replaceChildren();
  if (!messages.length) {
    preview.innerHTML = '<p class="chat-system">Start a conversation with the Lixr store team.</p>';
    return;
  }
  for (const message of messages) {
    const bubble = document.createElement('p');
    bubble.className = `chat-bubble ${message.sender === 'visitor' ? 'you' : 'team'}`;
    bubble.textContent = message.body;
    preview.append(bubble);
  }
  preview.scrollTop = preview.scrollHeight;
}
async function loadChatMessages(silent = true) {
  const thread = getChatThread();
  if (!thread) return;
  try {
    const data = await API.chat(thread);
    renderChatMessages(data.messages || []);
  } catch (error) {
    if (!silent) toast(error.message);
  }
}
function resizeChatInput() {
  const input = $('#chatInput');
  const panel = $('#quickMessage');
  if (!input || !panel) return;
  input.style.height = 'auto';
  const max = Math.max(88, Math.floor((panel.clientHeight || window.innerHeight) * .30));
  input.style.height = `${Math.min(max, input.scrollHeight)}px`;
  input.style.overflowY = input.scrollHeight > max ? 'auto' : 'hidden';
}
async function sendChatMessage() {
  const input = $('#chatInput');
  const send = $('#chatComposer button[type="submit"]');
  const status = $('#chatStatus');
  const value = input.value.trim();
  if (!value || send.disabled) return;
  send.disabled = true;
  status.textContent = 'Sending…';
  try {
    const data = await API.sendChat({ thread_id: getChatThread(), message: value });
    saveChatThread(data.thread_id);
    input.value = '';
    resizeChatInput();
    await loadChatMessages(false);
    status.textContent = 'Sent to the Lixr store team.';
  } catch (error) {
    status.textContent = 'Message not sent. Please try again.';
    toast(error.message);
  } finally {
    send.disabled = false;
    input.focus();
  }
}
function openChat() {
  const panel = $('#quickMessage');
  if (!panel) return;
  panel.hidden = false;
  document.body.classList.add('chat-open');
  $('#chatLauncher').setAttribute('aria-expanded', 'true');
  loadChatMessages();
  clearInterval(state.chatPoll);
  state.chatPoll = setInterval(() => loadChatMessages(true), 3000);
  setTimeout(() => { $('#chatInput')?.focus(); resizeChatInput(); }, 40);
}
function closeChat() {
  $('#quickMessage').hidden = true;
  document.body.classList.remove('chat-open');
  $('#chatLauncher').setAttribute('aria-expanded', 'false');
  clearInterval(state.chatPoll);
  state.chatPoll = null;
}

function handleAuthReturn() {
  const url = new URL(location.href);
  const result = url.searchParams.get('auth');
  if (!result) return;
  if (result === 'success') toast('Signed in with Google.');
  else if (result === 'cancelled') toast('Google sign-in was cancelled.');
  else toast('Google sign-in could not be completed.');
  url.searchParams.delete('auth');
  history.replaceState(null, '', `${url.pathname}${url.search}${url.hash}`);
}

function bindEvents() {
  window.addEventListener('scroll', updateScrollUI, { passive: true });
  window.addEventListener('resize', () => { updateScrollUI(); resizeChatInput(); }, { passive: true });
  updateScrollUI();
  initAnchorNavigation();
  initHeroSpotlight();
  initTooltips();

  $('#themeToggle')?.addEventListener('click', toggleTheme);
  $('#footerTheme')?.addEventListener('click', toggleTheme);
  $('#menuButton')?.addEventListener('click', () => setMenu($('#menuButton').getAttribute('aria-expanded') !== 'true'));
  $$('#mobileMenu a').forEach(link => link.addEventListener('click', () => setMenu(false)));
  $('#profileButton')?.addEventListener('click', openProfile);
  $('#mobileProfileButton')?.addEventListener('click', openProfile);
  $('#reviewButton')?.addEventListener('click', () => googleSignIn('/#reviews'));
  $('#googleProfileButton')?.addEventListener('click', () => googleSignIn('/#reviews'));
  $('#visitorSignOut')?.addEventListener('click', async () => {
    try {
      await API.visitorLogout();
      state.visitor = null;
      renderVisitor();
      closeModal($('#profileDialog'));
      toast('Signed out.');
    } catch (error) { toast(error.message); }
  });

  $$('#starPicker [data-stars]').forEach(button => button.addEventListener('click', () => {
    state.selectedStars = Number(button.dataset.stars);
    updateStarPicker();
  }));
  $('#reviewForm')?.addEventListener('submit', async event => {
    event.preventDefault();
    if (!state.visitor) { googleSignIn('/#reviews'); return; }
    const comment = $('#reviewComment').value.trim();
    if (!state.selectedStars) { toast('Choose a star rating first.'); return; }
    if (comment.length < 4) { toast('Add a short note about your experience.'); return; }
    const submit = event.currentTarget.querySelector('button[type="submit"]');
    submit.disabled = true;
    try {
      const data = await API.submitRating({ stars: state.selectedStars, comment });
      toast(data.message || 'Review posted.');
      $('#reviewComment').value = '';
      state.selectedStars = 0;
      updateStarPicker();
      state.reviewPage = 1;
      await loadRatings(1);
    } catch (error) { toast(error.message); }
    finally { submit.disabled = false; }
  });

  $('#refreshProducts')?.addEventListener('click', () => loadProducts(true));
  $('#productGrid')?.addEventListener('click', event => {
    const button = event.target.closest('[data-product-id]');
    if (!button) return;
    const product = state.products.find(item => String(item.id) === button.dataset.productId);
    if (product) openProduct(product);
  });

  $$('[data-close-dialog]').forEach(button => button.addEventListener('click', () => closeModal(button.closest('.modal-layer'))));
  $$('.modal-layer').forEach(layer => layer.addEventListener('click', event => { if (event.target === layer) closeModal(layer); }));
  document.addEventListener('keydown', event => {
    if (event.key === 'Escape') {
      const open = $$('.modal-layer:not([hidden])').at(-1);
      if (open) closeModal(open);
      else if (!$('#quickMessage')?.hidden) closeChat();
    }
  });
  $('#dialogMessageButton')?.addEventListener('click', () => focusContact(state.selectedProduct ? `Availability: ${state.selectedProduct.name}` : 'Product availability'));

  $('#chatLauncher')?.addEventListener('click', () => $('#quickMessage').hidden ? openChat() : closeChat());
  $('#chatClose')?.addEventListener('click', closeChat);
  $('#chatComposer')?.addEventListener('submit', event => { event.preventDefault(); sendChatMessage(); });
  $('#chatInput')?.addEventListener('input', resizeChatInput);
  $('#chatInput')?.addEventListener('keydown', event => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      $('#chatComposer')?.requestSubmit();
    }
  });

  $('#navigateButton')?.addEventListener('click', navigateToStore);
  $('#inquiryType')?.addEventListener('change', event => { $('#contactSubject').value = event.target.value; });
  $('#contactForm')?.addEventListener('submit', async event => {
    event.preventDefault();
    const form = event.currentTarget;
    const submit = form.querySelector('button[type="submit"]');
    const status = $('#contactStatus');
    const payload = Object.fromEntries(new FormData(form));
    submit.disabled = true;
    status.textContent = 'Sending…';
    try {
      const data = await API.contact(payload);
      status.textContent = data.message;
      form.reset();
      $('#contactSubject').value = $('#inquiryType').value;
      if (state.visitor) {
        form.querySelector('[name="name"]').value = state.visitor.display_name || '';
        form.querySelector('[name="email"]').value = state.visitor.email || '';
      }
      toast('Message sent to Lixr.');
    } catch (error) { status.textContent = error.message; }
    finally { submit.disabled = false; }
  });

  $('#newsletterForm')?.addEventListener('submit', async event => {
    event.preventDefault();
    const form = event.currentTarget;
    const input = $('#newsletterEmail');
    const button = form.querySelector('button');
    button.disabled = true;
    try {
      const data = await API.newsletter(input.value);
      if (data.already_subscribed && data.subscribed_at) toast(`You joined the Lixr list on ${formatSubscriptionDate(data.subscribed_at)}.`);
      else toast(data.message);
      form.reset();
    } catch (error) { toast(error.message); }
    finally { button.disabled = false; }
  });
}

let productResizeTimer = 0;
window.addEventListener('resize', () => {
  clearTimeout(productResizeTimer);
  productResizeTimer = setTimeout(() => {
    if (state.products.length && state.productPageSize !== productPageSize()) {
      state.productPage = 1;
      renderProducts(state.products);
    }
  }, 120);
});

function trackPageVisit() {
  let session = '';
  try {
    session = localStorage.getItem('lixr-visit-session') || '';
    if (!session) {
      session = globalThis.crypto?.randomUUID?.() || `visit-${Date.now()}-${Math.random().toString(36).slice(2)}`;
      localStorage.setItem('lixr-visit-session', session);
    }
  } catch { session = `visit-${Date.now()}-${Math.random().toString(36).slice(2)}`; }
  API.trackVisit({ session, path: location.pathname }).catch(() => {});
}

initTheme();
bindEvents();
initMap();
handleAuthReturn();
trackPageVisit();
$('#year').textContent = new Date().getFullYear();
Promise.allSettled([loadVisitor(), loadStore(), loadProducts(), loadRatings()]).then(updateScrollUI);
