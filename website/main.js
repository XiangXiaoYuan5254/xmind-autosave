(() => {
  'use strict';

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  const clockTime = () => new Date().toLocaleTimeString('zh-CN', { hour12: false });

  // 与 App 的 saveDelayMilliseconds 保持一致
  const SAVE_DELAY = 1200;

  /* ---------- Windows 访客：下载按钮、快捷键和安装步骤换成 Windows 的 ---------- */
  const IS_WINDOWS = /windows/i.test(navigator.userAgentData?.platform || navigator.userAgent);
  const SAVE_KEYS = IS_WINDOWS ? 'Ctrl S' : '⌘S';
  if (IS_WINDOWS) {
    document.documentElement.dataset.platform = 'windows';
    for (const key of $$('[data-mod-key]')) key.textContent = 'Ctrl';
    for (const keys of $$('[data-save-keys]')) keys.textContent = 'Ctrl+S';
    const heroDownload = $('[data-hero-download]');
    const windowsDownload = $('[data-download-windows]');
    if (heroDownload && windowsDownload) {
      heroDownload.href = windowsDownload.getAttribute('href');
      $('[data-hero-download-label]', heroDownload).textContent = '免费下载 Windows 版';
    }
    const windowsSteps = $('#install-windows');
    if (windowsSteps) windowsSteps.checked = true;
  }

  /* ---------- 导航滚动状态 ---------- */
  const nav = $('[data-nav]');
  const syncNav = () => nav.classList.toggle('is-scrolled', window.scrollY > 8);
  window.addEventListener('scroll', syncNav, { passive: true });
  syncNav();

  /* ---------- 入场动画 ---------- */
  const revealables = $$('.reveal');
  for (const el of revealables) {
    const siblings = Array.from(el.parentElement.children).filter((c) => c.classList.contains('reveal'));
    const index = siblings.indexOf(el);
    if (index > 0) el.style.setProperty('--d', `${Math.min(index, 5) * 80}ms`);
  }
  if ('IntersectionObserver' in window) {
    const io = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        entry.target.classList.add('is-in');
        io.unobserve(entry.target);
      }
    }, { rootMargin: '0px 0px -6% 0px', threshold: 0.06 });
    revealables.forEach((el) => io.observe(el));
  } else {
    revealables.forEach((el) => el.classList.add('is-in'));
  }
  window.__siteReady = true;

  /* ---------- 工具 ---------- */

  // 元素可见且标签页在前台时才让动画继续
  function visibilityGate(el) {
    let inView = false;
    const waiters = [];
    const listeners = [];
    const isOn = () => inView && !document.hidden;
    const flush = () => {
      if (!isOn()) return;
      waiters.splice(0).forEach((resolve) => resolve());
      listeners.forEach((fn) => fn());
    };
    new IntersectionObserver(([entry]) => {
      inView = entry.isIntersecting;
      flush();
    }, { threshold: 0.05 }).observe(el);
    document.addEventListener('visibilitychange', flush);
    return {
      get visible() { return isOn(); },
      wait: () => (isOn() ? Promise.resolve() : new Promise((resolve) => waiters.push(resolve))),
      onShow: (fn) => listeners.push(fn),
    };
  }

  // 与 App 相同的防抖逻辑：每次按键都重新计时，安静 SAVE_DELAY 后保存一次
  class Debouncer {
    constructor(delay, onSave) {
      this.delay = delay;
      this.onSave = onSave;
      this.timer = 0;
      this.last = 0;
      this.pending = false;
    }
    key() {
      this.last = performance.now();
      this.pending = true;
      clearTimeout(this.timer);
      this.timer = setTimeout(() => {
        this.pending = false;
        this.onSave();
      }, this.delay);
    }
    cancel() {
      clearTimeout(this.timer);
      this.pending = false;
    }
    elapsed(now = performance.now()) {
      return this.pending ? Math.min(this.delay, now - this.last) : 0;
    }
  }

  /* ---------- 首屏：窗口缩放 ---------- */
  const showcase = $('[data-showcase]');
  if (showcase) {
    const fit = () => showcase.style.setProperty('--s', Math.min(1, showcase.clientWidth / 1040).toFixed(4));
    fit();
    new ResizeObserver(fit).observe(showcase);
  }

  /* ---------- 首屏：思维导图 ---------- */
  const SVG_NS = 'http://www.w3.org/2000/svg';
  const svgEl = (tag, attrs, parent) => {
    const node = document.createElementNS(SVG_NS, tag);
    for (const [key, value] of Object.entries(attrs)) node.setAttribute(key, value);
    if (parent) parent.appendChild(node);
    return node;
  };

  function buildMindMap(svg) {
    const CX = 520;
    const CY = 280;
    const lines = svgEl('g', {}, svg);
    const nodes = svgEl('g', {}, svg);

    const makeNode = (cls, text, { padX, h, rx, minW = 0 }) => {
      const g = svgEl('g', { class: cls }, nodes);
      const rect = svgEl('rect', { height: h, rx }, g);
      const label = svgEl('text', {}, g);
      const node = { g, rect, label, x: 0, y: 0, w: 0 };
      node.measure = () => {
        node.w = Math.max(minW, Math.ceil(label.getComputedTextLength()) + padX * 2);
        return node.w;
      };
      node.place = (x, y) => {
        node.x = x;
        node.y = y;
        rect.setAttribute('x', x);
        rect.setAttribute('y', y - h / 2);
        rect.setAttribute('width', node.w);
        label.setAttribute('x', x + padX);
        label.setAttribute('y', y + 1);
      };
      label.textContent = text;
      node.measure();
      return node;
    };

    const curve = (x1, y1, x2, y2, color, width) => {
      const dx = (x2 - x1) * 0.55;
      const path = svgEl('path', {
        class: 'mm-line',
        d: `M${x1},${y1} C${x1 + dx},${y1} ${x2 - dx},${y2} ${x2},${y2}`,
        'stroke-width': width,
      }, lines);
      path.style.stroke = color;
    };

    const center = makeNode('mm-center', '产品路线图 2026', { padX: 28, h: 62, rx: 16 });
    center.place(CX - center.w / 2, CY);

    const branches = [
      { side: -1, y: 146, color: 'var(--mm-c2)', text: '用户调研', kids: ['访谈 12 位重度用户', '整理需求优先级'] },
      { side: -1, y: 410, color: 'var(--mm-c3)', text: '竞品分析', kids: ['功能对比', '定价参考', '口碑整理'] },
      { side: 1, y: 140, color: 'var(--mm-c1)', text: '核心功能', kids: ['每个文件独立开关', '停笔 1.2 秒保存'] },
      { side: 1, y: 404, color: 'var(--mm-c4)', text: '发布计划', kids: ['招募内测用户', null] },
    ];

    let typing = null;
    for (const branch of branches) {
      const { side, y, color } = branch;
      const main = makeNode('mm-main', branch.text, { padX: 18, h: 40, rx: 11 });
      main.rect.style.fill = color;
      main.place(side > 0 ? CX + center.w / 2 + 92 : CX - center.w / 2 - 92 - main.w, y);
      curve(CX + side * center.w * 0.3, CY, side > 0 ? main.x : main.x + main.w, y, color, 2.6);

      const fromX = side > 0 ? main.x + main.w : main.x;
      branch.kids.forEach((text, i) => {
        const ky = y + (i - (branch.kids.length - 1) / 2) * 50;
        const sub = makeNode('mm-sub', text ?? '', { padX: 12, h: 32, rx: 9, minW: 40 });
        sub.rect.style.fill = color;
        sub.rect.style.fillOpacity = '0.15';
        const kx = side > 0 ? fromX + 52 : fromX - 52 - sub.w;
        sub.place(kx, ky);
        curve(fromX, y, side > 0 ? kx : kx + sub.w, ky, color, 1.6);
        if (text === null) typing = sub;
      });
    }

    // 正在编辑的节点：选中框 + 光标
    const focus = svgEl('rect', { class: 'mm-focus', rx: 11 }, typing.g);
    typing.g.insertBefore(focus, typing.rect);
    const caret = svgEl('rect', { class: 'mm-caret', width: 1.6, height: 18, rx: 0.8 }, typing.g);
    const anchorX = typing.x;
    const syncTyping = () => {
      typing.measure();
      typing.place(anchorX, typing.y);
      focus.setAttribute('x', typing.x - 3.5);
      focus.setAttribute('y', typing.y - 19.5);
      focus.setAttribute('width', typing.w + 7);
      focus.setAttribute('height', 39);
      const textW = typing.label.textContent ? typing.label.getComputedTextLength() : 0;
      caret.setAttribute('x', typing.x + 12 + textW + 1.5);
      caret.setAttribute('y', typing.y - 9);
    };
    syncTyping();

    return {
      setText(text) {
        typing.label.textContent = text;
        syncTyping();
      },
      setTyping(active) {
        caret.classList.toggle('is-typing', active);
      },
      showCaret(show) {
        caret.style.display = show ? '' : 'none';
        focus.style.display = show ? '' : 'none';
      },
    };
  }

  function heroDemo() {
    const svg = $('[data-mindmap]');
    if (!svg || !showcase) return;
    const map = buildMindMap(svg);
    const edited = $('[data-edited]');
    const toast = $('[data-toast]');
    const pill = $('.pill', showcase);
    const hudIcon = $('[data-hud-icon]');
    const hudText = $('[data-hud-text]');
    const hudMeter = $('[data-hud-meter]');
    const hudTime = $('[data-hud-time]');
    const phrases = ['10 月 8 日开放 Beta', '收集首批用户反馈', '上线官网与下载页'];

    const setHud = (state, text) => {
      hudIcon.dataset.state = state;
      hudText.textContent = text;
    };

    if (reduceMotion) {
      map.setText(phrases[0]);
      map.showCaret(false);
      setHud('saved', `已自动保存 ${clockTime()}`);
      hudMeter.style.transform = 'scaleX(1)';
      hudTime.textContent = '1.2 / 1.2 秒';
      return;
    }

    const gate = visibilityGate(showcase);
    let lastSave = -Infinity;
    let onSaved = null;

    const debouncer = new Debouncer(SAVE_DELAY, () => {
      pill.classList.add('is-saving');
      setHud('saving', '正在自动保存…');
      lastSave = performance.now();
      setTimeout(() => {
        edited.classList.remove('is-on');
        toast.classList.add('is-on');
        setHud('saved', `已自动保存 ${clockTime()}`);
        setTimeout(() => {
          toast.classList.remove('is-on');
          pill.classList.remove('is-saving');
        }, 1500);
        if (onSaved) onSaved();
      }, 240);
    });

    let running = false;
    const frame = () => {
      const now = performance.now();
      const elapsed = debouncer.elapsed(now);
      const justSaved = !debouncer.pending && now - lastSave < 900;
      const shown = debouncer.pending ? elapsed : justSaved ? SAVE_DELAY : 0;
      hudMeter.style.transform = `scaleX(${shown / SAVE_DELAY})`;
      hudTime.textContent = `${(shown / 1000).toFixed(1)} / 1.2 秒`;
      if (gate.visible) requestAnimationFrame(frame);
      else running = false;
    };
    const start = () => {
      if (running || !gate.visible) return;
      running = true;
      requestAnimationFrame(frame);
    };
    gate.onShow(start);

    setHud('saved', `已自动保存 ${clockTime()}`);
    hudTime.textContent = '0.0 / 1.2 秒';

    (async () => {
      await sleep(1100);
      for (let round = 0; ; round++) {
        await gate.wait();
        const phrase = phrases[round % phrases.length];
        map.setText('');
        map.showCaret(true);
        map.setTyping(false);
        await sleep(700);

        const thinkAt = Math.floor(phrase.length / 2);
        for (let i = 1; i <= phrase.length; i++) {
          map.setText(phrase.slice(0, i));
          map.setTyping(true);
          edited.classList.add('is-on');
          setHud('editing', '检测到编辑…');
          debouncer.key();
          // 中途停顿 0.7 秒：不到 1.2 秒，计时被下一次按键重置，不会保存
          await sleep(i === thinkAt ? 700 : 70 + Math.random() * 120);
        }
        map.setTyping(false);
        await new Promise((resolve) => { onSaved = resolve; });
        onSaved = null;
        await sleep(2600);
      }
    })();
  }

  /* ---------- 工作原理：可交互演示 ---------- */
  function typingDemo() {
    const root = $('[data-demo]');
    if (!root) return;
    const input = $('[data-demo-input]', root);
    const keysEl = $('[data-demo-keys]', root);
    const savesEl = $('[data-demo-saves]', root);
    const stateEl = $('[data-demo-state]', root);
    const meter = $('[data-demo-meter]', root);
    const timeEl = $('[data-demo-time]', root);
    const canvas = $('[data-demo-canvas]', root);
    const ctx = canvas.getContext('2d');
    const gate = visibilityGate(root);

    const axisEl = $('.demo__axis', root);
    let windowMs = 10000;
    const MODIFIERS = new Set(['Shift', 'Control', 'Alt', 'Meta', 'CapsLock', 'Tab', 'Escape']);
    let events = [];
    let keys = 0;
    let saves = 0;
    let lastSave = -Infinity;

    const bump = (el) => {
      el.classList.remove('is-bump');
      void el.offsetWidth;
      el.classList.add('is-bump');
    };
    const setState = (state, text) => {
      stateEl.dataset.state = state;
      stateEl.textContent = text;
    };

    const debouncer = new Debouncer(SAVE_DELAY, () => {
      const t = performance.now();
      saves += 1;
      lastSave = t;
      savesEl.textContent = saves;
      bump(savesEl);
      events.push({ t, type: 'save' });
      setState('saved', `已自动保存 ${clockTime()}`);
      start();
    });

    const onKey = () => {
      keys += 1;
      keysEl.textContent = keys;
      events.push({ t: performance.now(), type: 'key' });
      debouncer.key();
      setState('editing', '输入中 · 计时重置');
      start();
    };

    // 自动演示：用户开始操作前，在可见时自己打字
    let auto = !reduceMotion;
    let autoToken = 0;
    const reset = () => {
      debouncer.cancel();
      events = [];
      keys = 0;
      saves = 0;
      keysEl.textContent = '0';
      savesEl.textContent = '0';
      setState('idle', '等待输入');
      draw(performance.now());
    };
    const stopAuto = () => {
      if (!auto) return;
      auto = false;
      autoToken += 1;
      input.value = '';
      reset();
    };

    input.addEventListener('pointerdown', stopAuto);
    input.addEventListener('focus', stopAuto);
    input.addEventListener('keydown', (event) => {
      if (MODIFIERS.has(event.key)) return;
      stopAuto();
      onKey();
    });

    const autoPhrases = ['周三前确认发布清单', '先把灵感记下来', '停笔之后自动保存'];
    const runAuto = async () => {
      const token = ++autoToken;
      const alive = () => auto && token === autoToken;
      for (let round = 0; alive(); round++) {
        await gate.wait();
        if (!alive()) return;
        const phrase = autoPhrases[round % autoPhrases.length];
        input.value = '';
        await sleep(600);
        for (let i = 1; i <= phrase.length && alive(); i++) {
          input.value = phrase.slice(0, i);
          onKey();
          await sleep(i % 4 === 0 ? 480 + Math.random() * 360 : 90 + Math.random() * 150);
        }
        await sleep(SAVE_DELAY + 1900);
      }
    };

    /* 时间轴 */
    let W = 0;
    let H = 0;
    let dpr = 1;
    let colors = {};
    const readColors = () => {
      const cs = getComputedStyle(document.documentElement);
      const v = (name) => cs.getPropertyValue(name).trim();
      colors = { brand: v('--brand'), ink2: v('--ink-2'), ink3: v('--ink-3'), line: v('--line'), lineStrong: v('--line-strong') };
    };

    const pill = (x, y, w, h, r) => {
      ctx.beginPath();
      if (ctx.roundRect) ctx.roundRect(x, y, w, h, Math.min(r, w / 2, h / 2));
      else ctx.rect(x, y, w, h);
    };

    function draw(now) {
      if (!W || !H) return;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, W, H);

      const padL = 18;
      const padR = 30;
      const span = W - padL - padR;
      const xOf = (t) => padL + (1 - (now - t) / windowMs) * span;
      const midY = Math.round(H * 0.64);

      // 每秒一条刻度线，随时间向左移动
      ctx.lineWidth = 1;
      ctx.strokeStyle = colors.line;
      const firstSecond = Math.floor(now / 1000) * 1000;
      for (let t = firstSecond; now - t <= windowMs; t -= 1000) {
        const gx = Math.round(xOf(t)) + 0.5;
        ctx.beginPath();
        ctx.moveTo(gx, 16);
        ctx.lineTo(gx, H - 16);
        ctx.stroke();
      }

      ctx.strokeStyle = colors.lineStrong;
      ctx.beginPath();
      ctx.moveTo(padL, midY + 0.5);
      ctx.lineTo(W - padR, midY + 0.5);
      ctx.stroke();

      events = events.filter((e) => now - e.t < windowMs + SAVE_DELAY + 500);
      const keyTimes = events.filter((e) => e.type === 'key').map((e) => e.t);

      // 计时条：从每次按键开始，直到下一次按键（被重置）或满 1.2 秒（保存）
      ctx.fillStyle = colors.brand;
      keyTimes.forEach((t0, i) => {
        const next = i + 1 < keyTimes.length ? keyTimes[i + 1] : Infinity;
        const t1 = Math.min(next, t0 + SAVE_DELAY, now);
        const completed = t1 >= t0 + SAVE_DELAY;
        ctx.globalAlpha = completed ? 0.5 : 0.2;
        pill(xOf(t0), midY - 7, Math.max(2, xOf(t1) - xOf(t0)), 14, 7);
        ctx.fill();
      });
      ctx.globalAlpha = 1;

      ctx.fillStyle = colors.ink2;
      for (const t of keyTimes) {
        pill(xOf(t) - 1.25, midY - 17, 2.5, 34, 1.25);
        ctx.fill();
      }

      for (const e of events) {
        if (e.type !== 'save') continue;
        const x = xOf(e.t);
        ctx.strokeStyle = colors.brand;
        ctx.setLineDash([3, 4]);
        ctx.beginPath();
        ctx.moveTo(x, 44);
        ctx.lineTo(x, H - 14);
        ctx.stroke();
        ctx.setLineDash([]);
        ctx.font = '600 12px -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif';
        const pillWidth = Math.max(44, Math.ceil(ctx.measureText(SAVE_KEYS).width) + 20);
        ctx.fillStyle = colors.brand;
        pill(x - pillWidth / 2, 16, pillWidth, 26, 13);
        ctx.fill();
        ctx.fillStyle = '#ffffff';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(SAVE_KEYS, x, 29.5);
      }

      // “现在”
      const nowX = W - padR;
      ctx.fillStyle = debouncer.pending ? colors.brand : colors.ink3;
      ctx.beginPath();
      ctx.arc(nowX, midY, 4, 0, Math.PI * 2);
      ctx.fill();

      // 左侧淡出
      ctx.globalCompositeOperation = 'destination-out';
      const fade = ctx.createLinearGradient(0, 0, 56, 0);
      fade.addColorStop(0, 'rgba(0,0,0,1)');
      fade.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.fillStyle = fade;
      ctx.fillRect(0, 0, 56, H);
      ctx.globalCompositeOperation = 'source-over';

      const elapsed = debouncer.elapsed(now);
      const justSaved = !debouncer.pending && now - lastSave < 900;
      const shown = debouncer.pending ? elapsed : justSaved ? SAVE_DELAY : 0;
      meter.style.transform = `scaleX(${shown / SAVE_DELAY})`;
      timeEl.textContent = `${(shown / 1000).toFixed(1)} / 1.2 秒`;
    }

    let running = false;
    const frame = () => {
      const now = performance.now();
      draw(now);
      const busy = debouncer.pending || events.some((e) => now - e.t < windowMs + 200);
      if (busy && gate.visible) requestAnimationFrame(frame);
      else running = false;
    };
    function start() {
      if (running || !gate.visible) return;
      running = true;
      requestAnimationFrame(frame);
    }

    const resize = () => {
      dpr = Math.min(window.devicePixelRatio || 1, 2);
      W = canvas.clientWidth;
      H = canvas.clientHeight;
      // 窄屏上缩短时间窗口，避免按键刻度挤在一起
      windowMs = W < 560 ? 6000 : 10000;
      axisEl.textContent = `过去 ${windowMs / 1000} 秒 → 现在`;
      canvas.width = Math.round(W * dpr);
      canvas.height = Math.round(H * dpr);
      draw(performance.now());
    };

    readColors();
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      readColors();
      draw(performance.now());
    });
    new ResizeObserver(resize).observe(canvas);
    gate.onShow(start);
    if (auto) runAuto();
  }

  /* ---------- 功能卡片里的开关 ---------- */
  for (const sw of $$('[data-files] .switch')) {
    sw.addEventListener('click', () => {
      const on = !sw.classList.contains('is-on');
      sw.classList.toggle('is-on', on);
      sw.setAttribute('aria-checked', String(on));
    });
  }

  /* ---------- 复制 ---------- */
  const toastEl = $('[data-copied]');
  let toastTimer = 0;
  const showToast = (message) => {
    toastEl.textContent = message;
    toastEl.classList.add('is-on');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => toastEl.classList.remove('is-on'), 1800);
  };
  const legacyCopy = (text) => {
    const area = document.createElement('textarea');
    area.value = text;
    area.setAttribute('readonly', '');
    area.style.cssText = 'position:fixed;opacity:0;pointer-events:none';
    document.body.appendChild(area);
    area.select();
    const ok = document.execCommand('copy');
    area.remove();
    return ok;
  };
  for (const button of $$('[data-copy]')) {
    button.addEventListener('click', async () => {
      const text = button.dataset.copy;
      let copied = false;
      try {
        if (navigator.clipboard && window.isSecureContext) {
          await navigator.clipboard.writeText(text);
          copied = true;
        }
      } catch {
        // 剪贴板权限被拒绝时退回旧接口
      }
      if (!copied) {
        try { copied = legacyCopy(text); } catch { copied = false; }
      }
      if (!copied) {
        // 选中文本，方便直接复制
        const target = button.closest('.terminal')?.querySelector('pre');
        if (target) {
          const range = document.createRange();
          range.selectNodeContents(target);
          const selection = window.getSelection();
          selection.removeAllRanges();
          selection.addRange(range);
        }
      }
      showToast(copied ? '已复制到剪贴板' : `已选中，按 ${IS_WINDOWS ? 'Ctrl+C' : '⌘C'} 复制`);
    });
  }

  heroDemo();
  typingDemo();
})();
