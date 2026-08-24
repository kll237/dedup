"use strict";

const IMG_EXT = [".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tiff", ".tif"];
const state = { exact: [], similar: [], es: null };

const $ = (id) => document.getElementById(id);

function human(b) {
  if (b < 1024) return b + " B";
  const u = ["KB", "MB", "GB", "TB", "PB"];
  let f = b / 1024, i = 0;
  while (f >= 1024 && i < u.length - 1) { f /= 1024; i++; }
  return f.toFixed(2) + " " + u[i];
}
function isImage(p) {
  const e = p.toLowerCase().slice(p.lastIndexOf("."));
  return IMG_EXT.includes(e);
}
function base(p) {
  const i = Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\"));
  return i < 0 ? p : p.slice(i + 1);
}
function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => (
    { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]
  ));
}
function thumbURL(p) { return "/api/thumb?path=" + encodeURIComponent(p); }

// hamming distance from two hex strings (64-bit safe via BigInt)
function hammingHex(a, b) {
  let x = BigInt("0x" + a) ^ BigInt("0x" + b);
  let c = 0n;
  while (x > 0n) { c += x & 1n; x >>= 1n; }
  return Number(c);
}

function setStatus(t) { $("status").textContent = t; }
function showToast(t) {
  const el = $("toast");
  el.textContent = t;
  el.classList.remove("hidden");
  clearTimeout(showToast._t);
  showToast._t = setTimeout(() => el.classList.add("hidden"), 2600);
}

function startScan() {
  if (state.es) state.es.close();
  const path = $("path").value.trim();
  if (!path) { showToast("请先填写扫描路径"); return; }

  $("scan").disabled = true;
  $("stop").disabled = false;
  setStatus("扫描中…");
  $("progressWrap").classList.remove("hidden");
  setBar(0, "准备中…");

  const params = new URLSearchParams({
    path,
    mode: $("mode").value,
    threshold: $("threshold").value || "10",
    min: $("min").value || "0",
    max: $("max").value || "0",
    skipHidden: "true",
  });

  const es = new EventSource("/api/scan?" + params.toString());
  state.es = es;

  es.addEventListener("progress", (e) => {
    const d = JSON.parse(e.data);
    if (d.phase === "scanned") {
      setStatus("已扫描 " + d.files + " 个文件，开始分析…");
      setBar(3, "已扫描 " + d.files + " 个文件");
    } else if (d.phase === "hash") {
      const pct = d.total ? Math.round((d.done / d.total) * 100) : 0;
      setBar(pct, "计算内容哈希 " + d.done + "/" + d.total + " (" + pct + "%)");
    } else if (d.phase === "phash") {
      const pct = d.total ? Math.round((d.done / d.total) * 100) : 0;
      setBar(pct, "计算感知哈希 " + d.done + "/" + d.total + " (" + pct + "%)");
    }
  });

  es.addEventListener("result", (e) => {
    render(JSON.parse(e.data));
    finishScan();
  });

  es.addEventListener("error", (e) => {
    if (e.data) {
      try { const d = JSON.parse(e.data); showToast("错误：" + d.message); } catch (_) {}
    }
    finishScan();
  });
}

function finishScan() {
  if (state.es) state.es.close();
  state.es = null;
  $("scan").disabled = false;
  $("stop").disabled = true;
  setStatus("完成");
}

function setBar(pct, text) {
  $("bar").style.width = Math.max(2, pct) + "%";
  $("ptext").textContent = text || "";
}

function render(rep) {
  // summary
  const s = rep.stats;
  $("summary").classList.remove("hidden");
  $("summary").innerHTML = [
    card("扫描文件数", s.filesScanned),
    card("精确重复组", s.exactGroups),
    card("相似图片组", s.similarGroups),
    card("可节省空间", human(s.wastedBytes), s.wastedBytes > 0),
  ].join("");

  // ext stats
  if (s.extStats && s.extStats.length) {
    $("extstats").classList.remove("hidden");
    $("extTable").querySelector("tbody").innerHTML = s.extStats.map((x) =>
      `<tr><td>${esc(x.ext)}</td><td>${x.count}</td><td>${human(x.bytes)}</td></tr>`
    ).join("");
  }

  // exact
  state.exact = rep.exact || [];
  if (state.exact.length) {
    $("exactSection").classList.remove("hidden");
    $("exactCount").textContent = state.exact.length;
    $("exactList").innerHTML = state.exact.map((g, i) => exactGroup(g, i)).join("");
  } else {
    $("exactSection").classList.add("hidden");
  }

  // similar
  state.similar = rep.similar || [];
  if (state.similar.length) {
    $("similarSection").classList.remove("hidden");
    $("similarCount").textContent = state.similar.length;
    $("similarList").innerHTML = state.similar.map((g, i) => similarGroup(g, i)).join("");
  } else {
    $("similarSection").classList.add("hidden");
  }
}

function card(k, v, good) {
  return `<div class="stat"><div class="k">${esc(k)}</div><div class="v${good ? " good" : ""}">${esc(v)}</div></div>`;
}

function fileCard(f, extra) {
  const thumb = isImage(f.path)
    ? `<img src="${esc(thumbURL(f.path))}" loading="lazy" onerror="this.parentNode.innerHTML='<span class=&quot;ico&quot;>📄</span>'">`
    : `<span class="ico">📄</span>`;
  return `<div class="file" data-path="${esc(f.path)}">
    <div class="thumb">${thumb}</div>
    <div class="meta">
      <div class="name" title="${esc(f.path)}">${esc(base(f.path))}</div>
      <div class="sz">${human(f.size)}</div>
      ${extra || ""}
    </div>
  </div>`;
}

function exactGroup(g, i) {
  const save = human(g.size * (g.files.length - 1));
  const files = g.files.map((f) => fileCard(f)).join("");
  const del = g.files.length - 1;
  return `<div class="group">
    <div class="ghead">
      <span>哈希 <b>${esc(g.hash.slice(0, 12))}…</b></span>
      <span>单文件 <b>${human(g.size)}</b></span>
      <span>副本 <b>${g.files.length}</b></span>
      <span>可节省 <b>${save}</b></span>
    </div>
    <div class="files">${files}</div>
    <div class="gfoot">
      <button class="ghost" data-del="exact" data-idx="${i}" ${del <= 0 ? "disabled" : ""}>删除其余 ${del} 个副本（保留首个）</button>
    </div>
  </div>`;
}

function similarGroup(g, i) {
  const files = g.files.map((f) => {
    const dist = f.hash ? hammingHex(g.rep, f.hash) : "";
    const extra = f.hash ? `<div class="ht">汉明距离 ${dist}</div>` : "";
    return fileCard(f, extra);
  }).join("");
  const del = g.files.length - 1;
  return `<div class="group">
    <div class="ghead">
      <span>代表指纹 <b>${esc(g.rep)}</b></span>
      <span>张数 <b>${g.files.length}</b></span>
    </div>
    <div class="files">${files}</div>
    <div class="gfoot">
      <button class="ghost" data-del="similar" data-idx="${i}" ${del <= 0 ? "disabled" : ""}>删除相似副本（保留代表图）</button>
    </div>
  </div>`;
}

// ---- delete flow ----
let pending = null;

document.addEventListener("click", (e) => {
  const btn = e.target.closest("[data-del]");
  if (!btn) return;
  const type = btn.getAttribute("data-del");
  const idx = +btn.getAttribute("data-idx");
  const group = (type === "exact" ? state.exact : state.similar)[idx];
  const toDelete = group.files.slice(1).map((f) => f.path); // keep first
  if (!toDelete.length) return;
  pending = { paths: toDelete, btn };
  $("modalList").innerHTML = toDelete.map((p) => `<li>${esc(p)}</li>`).join("");
  $("modal").classList.remove("hidden");
});

$("modalCancel").addEventListener("click", () => {
  $("modal").classList.add("hidden");
  pending = null;
});

$("modalOk").addEventListener("click", async () => {
  if (!pending) return;
  const { paths, btn } = pending;
  $("modal").classList.add("hidden");
  try {
    const r = await fetch("/api/delete", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ paths }),
    });
    const data = await r.json();
    const deleted = data.deleted || [];
    deleted.forEach((p) => markRemoved(p));
    btn.disabled = true;
    if (deleted.length) showToast("已移入回收站 " + deleted.length + " 个文件");
    const failed = Object.keys(data.failed || {});
    if (failed.length) showToast("有 " + failed.length + " 个文件删除失败");
  } catch (err) {
    showToast("删除请求失败：" + err.message);
  }
  pending = null;
});

function markRemoved(p) {
  document.querySelectorAll('.file[data-path="' + cssEsc(p) + '"]').forEach((el) => {
    el.classList.add("removed");
    const meta = el.querySelector(".meta");
    if (meta && !meta.querySelector(".gone")) {
      const g = document.createElement("div");
      g.className = "gone";
      g.textContent = "已移入回收站";
      meta.appendChild(g);
    }
  });
}
function cssEsc(s) {
  return s.replace(/["\\]/g, "\\$&");
}

// ---- controls ----
$("scan").addEventListener("click", startScan);
$("stop").addEventListener("click", () => {
  if (state.es) state.es.close();
  state.es = null;
  finishScan();
  setStatus("已停止");
});
$("useDemo").addEventListener("click", () => {
  $("path").value = "demo/input";
  $("mode").value = "both";
  $("threshold").value = "10";
});
