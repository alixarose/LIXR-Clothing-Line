const $ = (selector, root = document) => root.querySelector(selector);

const API = {
  async request(path, options = {}) {
    const response = await fetch(path, {
      credentials: 'same-origin',
      headers: { 'Accept': 'application/json', 'Content-Type': 'application/json', ...(options.headers || {}) },
      ...options,
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || `Request failed (${response.status})`);
    return data;
  },
  health: () => API.request('/api/health'),
  session: () => API.request('/api/auth/session'),
  login: (payload) => API.request('/api/auth/login', { method: 'POST', body: JSON.stringify(payload) }),
  logout: () => API.request('/api/auth/logout', { method: 'POST', body: '{}' }),
};

function setTheme(theme) {
  const next = theme === 'dark' ? 'dark' : 'light';
  document.documentElement.dataset.theme = next;
  try { localStorage.setItem('lixr-theme', next); } catch {}
}
function initTheme() {
  let saved = '';
  try { saved = localStorage.getItem('lixr-theme') || ''; } catch {}
  setTheme(saved || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'));
}
function toast(message) {
  const region = $('#toastRegion');
  const item = document.createElement('div');
  item.className = 'toast';
  item.textContent = message;
  region.append(item);
  setTimeout(() => item.remove(), 3400);
}
function requestedNext() {
  const next = new URLSearchParams(location.search).get('next') || '';
  return next.startsWith('/') && !next.startsWith('//') ? next : '';
}
function destinationFor(staff, serverDestination = '') {
  if (staff?.role === 'cashier') return '/pos';
  const wanted = requestedNext();
  if (wanted && wanted !== '/pos') return wanted;
  return serverDestination || '/admin';
}
function showForm() {
  $('#existingSession').hidden = true;
  $('#accessForm').hidden = false;
  requestAnimationFrame(() => $('#accessLogin').focus());
}
function showSession(staff) {
  $('#sessionName').textContent = staff.display_name || staff.username || staff.email;
  $('#sessionRole').textContent = `${staff.role || 'staff'} · @${staff.username || 'staff'}`;
  $('#existingSession').hidden = false;
  $('#accessForm').hidden = true;
  const button = $('.existing-session-button');
  button.href = destinationFor(staff);
  button.innerHTML = staff.role === 'cashier' ? 'Continue to POS <span>→</span>' : 'Continue to Admin <span>→</span>';
}
async function checkHealth() {
  const indicator = $('#serverIndicator');
  try {
    const data = await API.health();
    indicator.classList.remove('offline'); indicator.classList.add('online');
    indicator.querySelector('span').textContent = 'Ready';
  } catch {
    indicator.classList.remove('online'); indicator.classList.add('offline');
    indicator.querySelector('span').textContent = 'Unavailable';
  }
}
async function checkSession() {
  try {
    const data = await API.session();
    if (data.authenticated && data.staff) showSession(data.staff); else showForm();
  } catch { showForm(); }
}

$('#accessTheme').addEventListener('click', () => setTheme(document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark'));
$('#passwordToggle').addEventListener('click', () => {
  const input = $('#accessPassword');
  const visible = input.type === 'text';
  input.type = visible ? 'password' : 'text';
  $('#passwordToggle').textContent = visible ? 'Show' : 'Hide';
  $('#passwordToggle').setAttribute('aria-label', visible ? 'Show password' : 'Hide password');
  input.focus();
});
$('#switchAccount').addEventListener('click', async () => {
  try { await API.logout(); } catch {}
  showForm(); toast('Signed out from the previous session.');
});
$('#accessForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const status = $('#accessStatus');
  const submit = $('#accessSubmit');
  const login = $('#accessLogin').value.trim().toLowerCase();
  const password = $('#accessPassword').value;
  status.className = 'access-status';
  if (login.length < 2 || password.length < 8) {
    status.classList.add('error');
    status.textContent = 'Enter your staff username/email and password.';
    return;
  }
  submit.disabled = true; status.textContent = 'Signing you in…';
  try {
    const data = await API.login({ login, password, remember: $('#rememberMe').checked });
    status.classList.add('success');
    status.textContent = `Welcome back, ${data.staff?.display_name || data.staff?.username || 'Lixr staff'}.`;
    setTimeout(() => { location.href = destinationFor(data.staff, data.destination); }, 300);
  } catch (error) {
    status.classList.add('error'); status.textContent = error.message;
  } finally { submit.disabled = false; }
});

initTheme();
$('#accessYear').textContent = new Date().getFullYear();
checkHealth();
checkSession();
