// Statistics page: reads GET /api/stats (nginx proxies it to GET /stats of
// analytics-processor) and draws the report. All text from the server goes
// through textContent, so element ids and values from the page cannot
// inject markup.
(function () {
  'use strict';

  const ENDPOINT = '/api/stats';
  const AUTO_REFRESH_MS = 10000;
  // Columns of the time chart at most: windows are merged into buckets
  // when the period has more of them (7 days of 30 s windows is 20160).
  const MAX_BUCKETS = 90;

  const TYPE_NAMES = {
    page_view: 'просмотр страницы',
    click: 'клик',
    input: 'ввод',
    change: 'изменение',
    focus: 'фокус',
    blur: 'потеря фокуса',
    submit: 'отправка формы',
  };
  // Fixed order of event types in tooltips and the table
  const TYPE_ORDER = ['page_view', 'click', 'change', 'input', 'focus', 'blur', 'submit'];

  const STEP_NAMES = {
    visit: 'Вход',
    pizza: 'Собрал пиццу',
    contacts: 'Заполнил контакты',
    submit: 'Отправил заказ',
  };

  const CHOICE_NAMES = {
    pizza: 'Пицца',
    size: 'Размер',
    topping: 'Добавки',
    quantity: 'Количество',
  };
  const CHOICE_ORDER = ['pizza', 'size', 'topping', 'quantity'];

  const VALUE_NAMES = {
    margherita: 'Маргарита',
    pepperoni: 'Пепперони',
    'four-cheese': 'Четыре сыра',
    hawaiian: 'Гавайская',
    small: '25 см',
    medium: '30 см',
    large: '35 см',
    cheese: 'Сыр',
    mushrooms: 'Грибы',
    jalapeno: 'Халапеньо',
  };

  const nf = new Intl.NumberFormat('ru-RU');
  const pf = new Intl.NumberFormat('ru-RU', { style: 'percent', maximumFractionDigits: 1 });
  const timeFmt = new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  const dateTimeFmt = new Intl.DateTimeFormat('ru-RU', {
    day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit',
  });

  const $ = (id) => document.getElementById(id);
  const form = $('filters');
  const tooltip = $('tooltip');
  let timer = null;
  let inFlight = null;

  function el(tag, className, text) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  function svgEl(tag, attrs) {
    const node = document.createElementNS('http://www.w3.org/2000/svg', tag);
    for (const [k, v] of Object.entries(attrs || {})) node.setAttribute(k, v);
    return node;
  }

  function pct(part, whole) {
    return whole > 0 ? pf.format(part / whole) : '-';
  }

  // --- loading ---

  async function load() {
    const params = new URLSearchParams();
    params.set('period', form.period.value);
    const win = form.window_sec.value.trim();
    if (win !== '') params.set('window_sec', win);

    if (inFlight) inFlight.abort();
    inFlight = new AbortController();

    try {
      const res = await fetch(ENDPOINT + '?' + params, { signal: inFlight.signal });
      const body = await res.json().catch(() => null);
      if (!res.ok) {
        const msg = body && body.error ? body.error : 'HTTP ' + res.status;
        throw new Error(msg);
      }
      showError('');
      render(body);
    } catch (err) {
      if (err.name === 'AbortError') return;
      showError('Не удалось загрузить статистику: ' + err.message);
    }
  }

  function showError(msg) {
    const box = $('error');
    box.textContent = msg;
    box.hidden = msg === '';
  }

  function schedule() {
    clearInterval(timer);
    timer = null;
    if (form.auto.checked) timer = setInterval(load, AUTO_REFRESH_MS);
  }

  // --- rendering ---

  function render(report) {
    const from = new Date(report.from);
    const to = new Date(report.to);
    $('period-text').textContent =
      dateTimeFmt.format(from) + ' - ' + dateTimeFmt.format(to) + ', окно ' + report.window_sec + ' с';

    renderTiles(report);
    renderFunnel(report.funnel);
    renderSeries(report, from, to);
    renderChoices(report.choices);
    renderTotals(report.totals);
  }

  function stepSessions(funnel, step) {
    const row = funnel.find((f) => f.step === step);
    return row ? row.sessions : 0;
  }

  function renderTiles(report) {
    const events = report.totals.reduce((sum, t) => sum + t.events, 0);
    const visits = stepSessions(report.funnel, 'visit');
    const submits = stepSessions(report.funnel, 'submit');
    $('kpi-events').textContent = nf.format(events);
    $('kpi-visits').textContent = nf.format(visits);
    $('kpi-submits').textContent = nf.format(submits);
    $('kpi-conversion').textContent = pct(submits, visits);
  }

  function renderFunnel(funnel) {
    const box = $('funnel');
    box.replaceChildren();
    const first = funnel.length > 0 ? funnel[0].sessions : 0;
    const max = Math.max(1, ...funnel.map((f) => f.sessions));

    funnel.forEach((f, i) => {
      const row = el('div', 'funnel-row');
      row.append(el('span', 'funnel-name', STEP_NAMES[f.step] || f.step));

      const track = el('div', 'funnel-track');
      const bar = el('div', 'funnel-bar');
      bar.style.width = (100 * f.sessions / max) + '%';
      bar.style.background = 'var(--step-' + Math.min(i + 1, 4) + ')';
      track.append(bar);
      row.append(track);

      const value = el('span', 'funnel-value', nf.format(f.sessions) + ' ');
      if (i > 0) {
        const prev = funnel[i - 1].sessions;
        value.append(el('span', 'pct', pct(f.sessions, prev) + ' / ' + pct(f.sessions, first)));
      }
      row.append(value);
      box.append(row);
    });
  }

  // Merges the windows into at most MAX_BUCKETS columns of equal length, a
  // multiple of the window. Windows without events are zero columns.
  function buckets(report, from, to) {
    const windowMs = report.window_sec * 1000;
    const windows = Math.max(1, Math.ceil((to - from) / windowMs));
    const perBucket = Math.ceil(windows / MAX_BUCKETS);
    const bucketMs = perBucket * windowMs;
    // Align to the window grid, as the server does (epoch-based windows)
    const start = Math.floor(from.getTime() / windowMs) * windowMs;
    const count = Math.max(1, Math.ceil((to.getTime() - start) / bucketMs));

    const list = [];
    for (let i = 0; i < count; i++) {
      list.push({ start: start + i * bucketMs, end: start + (i + 1) * bucketMs, total: 0, byType: {} });
    }
    for (const p of report.series) {
      const i = Math.floor((new Date(p.window_start).getTime() - start) / bucketMs);
      if (i < 0 || i >= count) continue;
      list[i].total += p.events;
      list[i].byType[p.event_type] = (list[i].byType[p.event_type] || 0) + p.events;
    }
    return { list, bucketMs };
  }

  function niceMax(v) {
    if (v <= 0) return 1;
    const pow = Math.pow(10, Math.floor(Math.log10(v)));
    for (const m of [1, 2, 2.5, 5, 10]) {
      if (m * pow >= v) return m * pow;
    }
    return 10 * pow;
  }

  function renderSeries(report, from, to) {
    const box = $('series');
    box.replaceChildren();
    const { list, bucketMs } = buckets(report, from, to);

    $('series-note').textContent = 'Событий за ' + (bucketMs / 1000) + ' с, наведите на столбец для разбивки по типам.';

    if (report.series.length === 0) {
      box.append(el('p', 'empty', 'За период событий нет.'));
      return;
    }

    const W = 920;
    const H = 260;
    const m = { top: 10, right: 8, bottom: 24, left: 44 };
    const iw = W - m.left - m.right;
    const ih = H - m.top - m.bottom;
    const yMax = niceMax(Math.max(...list.map((b) => b.total)));
    const step = iw / list.length;
    const gap = Math.min(2, step * 0.2);
    const y = (v) => m.top + ih - (v / yMax) * ih;

    const svg = svgEl('svg', { viewBox: '0 0 ' + W + ' ' + H, role: 'img', 'aria-label': 'Количество событий во времени' });

    for (let i = 0; i <= 4; i++) {
      const v = (yMax / 4) * i;
      svg.append(svgEl('line', { class: 'grid-line', x1: m.left, x2: W - m.right, y1: y(v), y2: y(v) }));
      const t = svgEl('text', { class: 'axis-text', x: m.left - 6, y: y(v) + 4, 'text-anchor': 'end' });
      t.textContent = nf.format(v);
      svg.append(t);
    }

    const labelEvery = Math.ceil(list.length / 6);
    const fmt = to - from > 24 * 3600 * 1000 ? dateTimeFmt : timeFmt;

    list.forEach((b, i) => {
      const x = m.left + i * step;
      const hit = svgEl('rect', { class: 'hit', x: x, y: m.top, width: step, height: ih });
      const h = Math.max(0, y(0) - y(b.total));
      const bar = svgEl('path', { class: 'bar', d: barPath(x + gap / 2, y(0), step - gap, h, 4) });
      hit.addEventListener('mousemove', (e) => showTooltip(e, bucketTooltip(b)));
      hit.addEventListener('mouseleave', hideTooltip);
      svg.append(hit, bar);

      if (i % labelEvery === 0) {
        const t = svgEl('text', { class: 'axis-text', x: x, y: H - 6 });
        t.textContent = fmt.format(new Date(b.start));
        svg.append(t);
      }
    });

    box.append(svg);
  }

  // A column anchored to the baseline with the top corners rounded.
  function barPath(x, base, w, h, r) {
    if (h <= 0) return '';
    const rr = Math.min(r, w / 2, h);
    return 'M' + x + ',' + base +
      'V' + (base - h + rr) +
      'Q' + x + ',' + (base - h) + ' ' + (x + rr) + ',' + (base - h) +
      'H' + (x + w - rr) +
      'Q' + (x + w) + ',' + (base - h) + ' ' + (x + w) + ',' + (base - h + rr) +
      'V' + base + 'Z';
  }

  function bucketTooltip(b) {
    const box = el('div');
    box.append(el('div', 'tt-title', timeFmt.format(new Date(b.start)) + ' - ' + timeFmt.format(new Date(b.end))));
    const total = el('div', 'tt-row');
    total.append(el('span', '', 'всего'), el('span', '', nf.format(b.total)));
    box.append(total);
    for (const type of TYPE_ORDER) {
      if (!b.byType[type]) continue;
      const row = el('div', 'tt-row');
      row.append(el('span', '', TYPE_NAMES[type]), el('span', '', nf.format(b.byType[type])));
      box.append(row);
    }
    return box;
  }

  function showTooltip(e, content) {
    tooltip.replaceChildren(content);
    tooltip.hidden = false;
    const pad = 14;
    const r = tooltip.getBoundingClientRect();
    let x = e.clientX + pad;
    let y = e.clientY + pad;
    if (x + r.width > window.innerWidth) x = e.clientX - r.width - pad;
    if (y + r.height > window.innerHeight) y = e.clientY - r.height - pad;
    tooltip.style.left = x + 'px';
    tooltip.style.top = y + 'px';
  }

  function hideTooltip() {
    tooltip.hidden = true;
  }

  function renderChoices(choices) {
    const box = $('choices');
    box.replaceChildren();
    const groups = {};
    for (const c of choices) (groups[c.choice] = groups[c.choice] || []).push(c);

    const names = CHOICE_ORDER.filter((n) => groups[n]).concat(
      Object.keys(groups).filter((n) => !CHOICE_ORDER.includes(n)));
    if (names.length === 0) {
      box.append(el('p', 'empty', 'За период выбора не было.'));
      return;
    }

    for (const name of names) {
      const rows = groups[name];
      const sum = rows.reduce((s, c) => s + c.picks, 0);
      const max = Math.max(...rows.map((c) => c.picks));
      const group = el('div', 'choice-group');
      group.append(el('h3', '', CHOICE_NAMES[name] || name));

      for (const c of rows) {
        const label = name === 'quantity' ? c.value + ' шт.' : VALUE_NAMES[c.value] || c.value;
        const row = el('div', 'choice-row');
        row.append(el('span', '', label), el('span', 'value', nf.format(c.picks) + ' (' + pct(c.picks, sum) + ')'));
        const track = el('div', 'choice-track');
        const bar = el('div', 'choice-bar');
        bar.style.width = (100 * c.picks / max) + '%';
        track.append(bar);
        row.append(track);
        group.append(row);
      }
      box.append(group);
    }
  }

  function renderTotals(totals) {
    const body = $('totals').tBodies[0];
    body.replaceChildren();
    if (totals.length === 0) {
      const tr = el('tr');
      const td = el('td', 'empty', 'За период событий нет.');
      td.colSpan = 3;
      tr.append(td);
      body.append(tr);
      return;
    }
    for (const t of totals) {
      const tr = el('tr');
      tr.append(
        el('td', '', TYPE_NAMES[t.event_type] || t.event_type),
        el('td', '', t.element_id),
        el('td', 'num', nf.format(t.events)),
      );
      body.append(tr);
    }
  }

  // --- wiring ---

  form.addEventListener('submit', (e) => {
    e.preventDefault();
    load();
  });
  form.period.addEventListener('change', load);
  form.auto.addEventListener('change', schedule);

  load();
  schedule();
})();
