/* All server-provided names and messages are rendered as text, never HTML. */
"use strict";
const $ = (s) => document.querySelector(s);
const $$ = (s) => [...document.querySelectorAll(s)];
const drawings = {
  cloud: '<path d="M6 18a4 4 0 0 1-.4-8A6.5 6.5 0 0 1 18 8a5 5 0 0 1 0 10Z"/>',
  folder:
    '<path d="M3 7V5a1 1 0 0 1 1-1h5l2 3h9a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7Z"/><path d="M3 9h18"/>',
  "folder-plus": '<path d="M3 7V5h6l2 3h10v12H3Z"/><path d="M12 11v6m-3-3h6"/>',
  transfer: '<path d="M7 4v16m-4-4 4 4 4-4M17 20V4m-4 4 4-4 4 4"/>',
  layers: '<path d="m12 3 10 5-10 5L2 8Zm-10 9 10 5 10-5M2 16l10 5 10-5"/>',
  server:
    '<rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6.5h.01M7 17.5h.01M16 6.5h2M16 17.5h2"/>',
  plug: '<path d="m9 3-1 5m8-4-1 5M6 8l12 2-1 4a6 6 0 0 1-6 4l-1 4M5 8l14 2"/>',
  upload: '<path d="M12 16V3m-5 5 5-5 5 5M4 15v6h16v-6"/>',
  download: '<path d="M12 3v13m-5-5 5 5 5-5M4 15v6h16v-6"/>',
  refresh:
    '<path d="M20 7a8 8 0 0 0-14-2L3 8m0-5v5h5M4 17a8 8 0 0 0 14 2l3-3m0 5v-5h-5"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',
  lock: '<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3m-4 4v3"/>',
  up: '<path d="M12 20V4m-6 6 6-6 6 6"/>',
  left: '<path d="m15 5-7 7 7 7"/>',
  right: '<path d="m9 5 7 7-7 7"/>',
  link: '<path d="m10 13 4-4m-6 7-2 2a4 4 0 0 1-5-5l5-5a4 4 0 0 1 6 0m0 8a4 4 0 0 0 6 0l5-5a4 4 0 0 0-5-5l-2 2" transform="translate(1 0) scale(.92)"/>',
  edit: '<path d="m16 3 5 5-12 12-6 1 1-6Zm-3 3 5 5"/>',
  trash: '<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7"/>',
  close: '<path d="m6 6 12 12M6 18 18 6"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  file: '<path d="M14 2H5v20h14V7Zm0 0v6h5M8 12h8m-8 4h6"/>',
  image:
    '<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8" cy="8" r="1.5"/><path d="m21 16-6-6L4 21"/>',
  video:
    '<rect x="3" y="3" width="18" height="18" rx="3"/><path d="m10 8 6 4-6 4Z"/>',
  audio:
    '<path d="M9 17V5l11-2v12M9 9l11-2"/><ellipse cx="6" cy="18" rx="3" ry="3"/><ellipse cx="17" cy="16" rx="3" ry="3"/>',
  archive: '<path d="M4 3h16v18H4ZM10 3v3h4v3h-4v3h4v3h-4v3h4"/>',
  shield: '<path d="m12 2 8 3v7c0 5-8 10-8 10S4 17 4 12V5Zm-4 9 3 3 5-6"/>',
  check: '<path d="m5 12 4 4L20 5"/>',
  "check-circle": '<circle cx="12" cy="12" r="9"/><path d="m8 12 3 3 5-6"/>',
  alert: '<circle cx="12" cy="12" r="9"/><path d="M12 7v6m0 4h.01"/>',
  pause: '<path d="M8 5v14M16 5v14"/>',
  play: '<path d="m8 4 12 8-12 8Z"/>',
};
function icon(name) {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${drawings[name] || drawings.file}</svg>`;
}
$$("[data-icon]").forEach((el) => {
  el.innerHTML = icon(el.dataset.icon);
});
$$("[data-icon-before]").forEach((el) => {
  el.insertAdjacentHTML("afterbegin", icon(el.dataset.iconBefore));
});

const state = {
  connection: null,
  path: "/",
  pendingPath: null,
  page: 1,
  listing: { content: [], total: 0, write: false },
  selected: new Set(),
  sort: "name",
  ascending: true,
  filter: "",
  view: "files",
  loading: false,
  serial: 0,
  passwords: new Map(),
  tasks: [],
  transferFilter: "all",
  tasksSignature: "",
  pollBusy: false,
  action: null,
  actionBusy: false,
  connecting: false,
  queuing: false,
};
async function api(method, ...args) {
  if (!window.go?.main?.App?.[method])
    throw new Error(
      "请通过 云泊 · Yunbo 桌面程序打开。浏览器预览无法连接 Go 后端。",
    );
  return window.go.main.App[method](...args);
}
function element(tag, className, text) {
  const el = document.createElement(tag);
  if (className) el.className = className;
  if (text !== undefined) el.textContent = text;
  return el;
}
function toast(message, error = false) {
  const el = element("div", `toast${error ? " error" : ""}`);
  el.innerHTML = icon(error ? "alert" : "check-circle");
  el.append(element("span", "", String(message)));
  $("#toasts").append(el);
  setTimeout(() => el.remove(), error ? 9000 : 4200);
}
function errorText(err) {
  return String(err?.message || err || "操作失败");
}
function joined(dir, name) {
  return (dir === "/" ? "" : dir) + "/" + name;
}
function cleanPath(p) {
  const stack = [];
  for (const part of p.replaceAll("\\", "/").split("/")) {
    if (!part || part === ".") continue;
    if (part === "..") stack.pop();
    else stack.push(part);
  }
  return "/" + stack.join("/");
}
function passwordFor(p) {
  let result = "",
    longest = -1;
  for (const [parent, password] of state.passwords) {
    if (
      (p === parent || p.startsWith(parent === "/" ? "/" : parent + "/")) &&
      parent.length > longest
    ) {
      result = password;
      longest = parent.length;
    }
  }
  return result;
}
function bytes(n) {
  if (!Number.isFinite(n) || n < 0) return "—";
  if (n === 0) return "0 B";
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), 4);
  return `${(n / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${["B", "KB", "MB", "GB", "TB"][i]}`;
}
function date(v) {
  const d = new Date(v);
  if (!Number.isFinite(d.getTime()) || d.getFullYear() < 1970) return "—";
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(d);
}
function fileKind(f) {
  if (f.is_dir) return "folder";
  const ext = f.name.split(".").pop().toLowerCase();
  if (
    [
      "png",
      "jpg",
      "jpeg",
      "webp",
      "gif",
      "svg",
      "bmp",
      "heic",
      "avif",
    ].includes(ext)
  )
    return "image";
  if (["mp4", "mkv", "avi", "mov", "webm", "m4v"].includes(ext)) return "video";
  if (["mp3", "wav", "flac", "m4a", "aac", "ogg"].includes(ext)) return "audio";
  if (["zip", "rar", "7z", "tar", "gz", "bz2"].includes(ext)) return "archive";
  return "file";
}
function can(bit) {
  const c = state.connection;
  return (
    !!c &&
    (c.user.role === 2 ||
      !!(Number(c.user.permission) & (1 << bit)) ||
      (bit === 3 && !!state.listing.write_content_bypass))
  );
}
function shownFiles() {
  const files = state.listing.content.filter((f) =>
    f.name.toLocaleLowerCase().includes(state.filter.toLocaleLowerCase()),
  );
  return files.sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
    let c = 0;
    if (state.sort === "size") c = a.size - b.size;
    else if (state.sort === "modified")
      c = String(a.modified).localeCompare(String(b.modified));
    else c = a.name.localeCompare(b.name, "zh-CN", { numeric: true });
    return state.ascending ? c : -c;
  });
}
function selectedFiles() {
  return state.listing.content.filter((f) => state.selected.has(f.name));
}
function updateControls() {
  const connected = !!state.connection,
    ready = connected && !state.loading && !state.queuing;
  $("#upload").disabled = !ready || !state.listing.write || !can(3);
  $("#upload-folder").disabled = $("#upload").disabled;
  $("#mkdir").disabled = $("#upload").disabled;
  $("#connect-button").disabled = state.queuing;
  const dropEnabled = !$("#upload").disabled && state.view === "files";
  $("#upload-panel").classList.toggle("drop-enabled", dropEnabled);
  $("#drop-destination").textContent = state.path;
  $("#drop-guide-text").textContent = state.queuing
    ? "正在准备…"
    : dropEnabled
      ? "可拖放上传"
      : connected
        ? "拖放未启用"
        : "未连接";
  $("#drop-guide-text").title = dropEnabled
    ? `拖入文件或文件夹，上传到 ${state.path}`
    : $("#drop-guide-text").textContent;
  for (const id of ["refresh", "directory-password", "filter", "path-input"])
    $("#" + id).disabled = !ready;
  $("#up").disabled = !ready || state.path === "/";
  const files = selectedFiles(),
    single = files.length === 1;
  $("#selectionbar").hidden = !files.length;
  $("#selection-label").textContent = `已选择 ${files.length} 项`;
  $("#download").disabled =
    !ready || !files.length || files.some((f) => f.is_dir);
  $("#copy-link").disabled = !ready || !single || files[0]?.is_dir;
  $("#rename").disabled = !ready || !single || !state.listing.write || !can(4);
  $("#remove").disabled =
    !ready || !files.length || !state.listing.write || !can(7);
  const visible = shownFiles(),
    selected = visible.filter((f) => state.selected.has(f.name)).length;
  $("#select-all").checked = visible.length > 0 && selected === visible.length;
  $("#select-all").indeterminate = selected > 0 && selected < visible.length;
  $("#select-all").disabled = !ready || !visible.length;
  const pages = Math.max(1, Math.ceil(state.listing.total / 100));
  $("#prev-page").disabled = !ready || state.page <= 1;
  $("#next-page").disabled = !ready || state.page >= pages;
}
function renderFiles() {
  $("#welcome").hidden = !!state.connection;
  $("#file-browser").hidden = !state.connection;
  $("#loading").hidden = !state.loading;
  $("#path-input").value = state.path;
  const files = shownFiles(),
    body = $("#file-list");
  body.replaceChildren();
  for (const file of files) {
    const row = element("tr", state.selected.has(file.name) ? "selected" : "");
    const checkCell = element("td", "check-col"),
      checkbox = element("input");
    checkbox.type = "checkbox";
    checkbox.checked = state.selected.has(file.name);
    checkbox.setAttribute("aria-label", `选择 ${file.name}`);
    checkbox.onchange = () => {
      checkbox.checked
        ? state.selected.add(file.name)
        : state.selected.delete(file.name);
      renderFiles();
    };
    checkCell.append(checkbox);
    row.append(checkCell);
    const nameCell = element("td"),
      nameWrap = element("div", "name-cell"),
      kind = fileKind(file),
      fileIcon = element("span", "file-icon " + kind);
    fileIcon.innerHTML = icon(kind);
    const name = element("button", "name-button", file.name);
    name.title = file.name;
    name.onclick = () => {
      if (file.is_dir) loadDir(joined(state.path, file.name));
      else {
        state.selected.has(file.name)
          ? state.selected.delete(file.name)
          : state.selected.add(file.name);
        renderFiles();
      }
    };
    nameWrap.append(fileIcon, name);
    nameCell.append(nameWrap);
    row.append(
      nameCell,
      element("td", "", file.is_dir ? "—" : bytes(file.size)),
      element("td", "", date(file.modified)),
      element(
        "td",
        "",
        file.is_dir
          ? "文件夹"
          : { image: "图片", video: "视频", audio: "音频", archive: "压缩包" }[
              kind
            ] || "文件",
      ),
    );
    body.append(row);
  }
  $("#empty-files").hidden = files.length > 0;
  $("#empty-title").textContent = state.filter
    ? "没有匹配的文件"
    : "文件夹为空";
  $("#empty-description").textContent = state.filter
    ? "试试其他关键词，筛选仅在当前页中进行。"
    : "可上传文件或拖入文件夹。";
  const pages = Math.max(1, Math.ceil(state.listing.total / 100));
  $("#file-summary").textContent =
    `共 ${state.listing.total} 项${state.filter ? ` · 当前页匹配 ${files.length} 项` : ""}`;
  $("#page-label").textContent = `${state.page} / ${pages}`;
  $("#path-hint").textContent = state.connection
    ? state.listing.write
      ? "可写目录"
      : "只读目录"
    : "云端目录";
  for (const field of ["name", "size", "modified"])
    $("#sort-" + field).textContent =
      state.sort === field ? (state.ascending ? "↑" : "↓") : "";
  updateControls();
}
function renderConnection() {
  const c = state.connection;
  $("#connect-button").lastChild.textContent = c ? "连接设置" : "连接服务器";
  $("#disconnect").hidden = !c;
  $("#server-dot").classList.toggle("online", !!c);
  $("#server-name").textContent = c ? c.user.username || "访客" : "尚未连接";
  $("#server-address").textContent = c
    ? c.profile.url
    : "添加你的 OpenList 服务";
  $("#server-address").title = c?.profile.url || "";
  renderFiles();
  renderTransfers();
}
async function loadDir(target = state.path, page = 1) {
  if (!state.connection || state.loading || state.queuing) return;
  target = cleanPath(target);
  const serial = ++state.serial;
  state.loading = true;
  $("#file-error").hidden = true;
  renderFiles();
  try {
    const listing = await api("List", target, passwordFor(target), page);
    if (serial !== state.serial) return;
    state.listing = listing;
    state.path = target;
    state.page = page;
    state.selected.clear();
    state.pendingPath = null;
    state.filter = "";
    $("#filter").value = "";
  } catch (err) {
    if (serial !== state.serial) return;
    state.pendingPath = target;
    $("#file-error-text").textContent = errorText(err);
    $("#file-error").hidden = false;
  } finally {
    if (serial === state.serial) {
      state.loading = false;
      renderFiles();
    }
  }
}
function switchView(view) {
  state.view = view;
  $("#files-view").hidden = view !== "files";
  $("#transfers-view").hidden = view !== "transfers";
  $("#nav-files").classList.toggle("active", view === "files");
  $("#nav-transfers").classList.toggle("active", view === "transfers");
  $("#view-name").textContent = view === "files" ? "全部文件" : "传输中心";
  if (view === "transfers") renderTransfers();
  updateControls();
}
function openConnection() {
  $("#connection-error").hidden = true;
  $("#connection-dialog").showModal();
}
function updateAuthFields() {
  const mode = $("#connection-form").elements.auth_mode.value;
  $("#password-fields").hidden = mode !== "password";
  $("#token-fields").hidden = mode !== "token";
}
function clearCredentials() {
  const form = $("#connection-form");
  for (const name of ["password", "otp", "token", "directory_password"])
    form.elements[name].value = "";
}
$("#connection-form").onsubmit = async (event) => {
  event.preventDefault();
  if (state.connecting) return;
  state.connecting = true;
  const form = event.currentTarget,
    request = Object.fromEntries(new FormData(form));
  $("#connection-error").hidden = true;
  $("#submit-connect").disabled = true;
  $("#submit-connect").textContent = "正在连接…";
  try {
    const result = await api("Connect", request);
    state.serial++;
    state.connection = result;
    state.path = result.profile.start_path;
    state.page = 1;
    state.listing = result.listing;
    state.selected.clear();
    state.passwords.clear();
    state.pendingPath = null;
    state.filter = "";
    $("#filter").value = "";
    state.loading = false;
    if (request.directory_password)
      state.passwords.set(state.path, request.directory_password);
    $("#file-error").hidden = true;
    clearCredentials();
    $("#connection-dialog").close();
    renderConnection();
    switchView("files");
    toast(result.warning || "已连接到你的 OpenList 服务器", !!result.warning);
  } catch (err) {
    $("#connection-error").textContent = errorText(err);
    $("#connection-error").hidden = false;
  } finally {
    state.connecting = false;
    $("#submit-connect").disabled = false;
    $("#submit-connect").innerHTML = icon("plug");
    $("#submit-connect").append(document.createTextNode("连接服务器"));
  }
};
$("#disconnect").onclick = async () => {
  if (state.connecting) return;
  try {
    await api("Disconnect");
    state.serial++;
    state.connection = null;
    state.selected.clear();
    state.passwords.clear();
    state.listing = { content: [], total: 0, write: false };
    state.path = "/";
    state.loading = false;
    state.filter = "";
    $("#filter").value = "";
    clearCredentials();
    $("#connection-dialog").close();
    $("#file-error").hidden = true;
    renderConnection();
    toast("连接已断开；可续传任务已保留，普通上传已取消。");
  } catch (err) {
    toast(errorText(err), true);
  }
};
$("#connection-form").elements.auth_mode.onchange = updateAuthFields;
$("#connection-form").elements.url.oninput = (event) => {
  const http = event.target.value.trim().startsWith("http://");
  $("#http-help").classList.toggle("insecure", http);
  $("#http-help").textContent = http
    ? "HTTP 连接未加密；跨公网连接建议使用 HTTPS。"
    : "支持域名、IP 地址和子路径部署。";
};
for (const id of ["connect-button", "welcome-connect"])
  $("#" + id).onclick = openConnection;
$$(".dialog-close").forEach(
  (button) =>
    (button.onclick = () => {
      if (state.connecting || state.actionBusy) return;
      button.closest("dialog").close();
    }),
);
$("#connection-dialog").addEventListener("cancel", (event) => {
  if (state.connecting) event.preventDefault();
});
$("#connection-dialog").addEventListener("close", clearCredentials);
$("#action-dialog").addEventListener("close", () => {
  $("#action-input").value = "";
  state.action = null;
});
$("#action-dialog").addEventListener("cancel", (event) => {
  if (state.actionBusy) event.preventDefault();
});

function actionDialog({
  title,
  description = "",
  value = "",
  password = false,
  remove = false,
  submit = "确定",
  run,
}) {
  state.action = run;
  $("#action-title").textContent = title;
  $("#action-description").textContent = description;
  $("#action-field").hidden = remove;
  $("#action-field").firstChild.textContent = password
    ? "密码（留空清除）"
    : "名称";
  const input = $("#action-input");
  input.value = value;
  input.type = password ? "password" : "text";
  input.required = !remove && !password;
  $("#action-error").hidden = true;
  $("#action-submit").textContent = submit;
  $("#action-submit").className = "button " + (remove ? "danger" : "primary");
  $("#action-dialog").showModal();
  if (!remove) {
    input.focus();
    input.select();
  }
}
$("#action-form").onsubmit = async (event) => {
  event.preventDefault();
  if (state.actionBusy) return;
  state.actionBusy = true;
  $("#action-submit").disabled = true;
  $("#action-error").hidden = true;
  try {
    await state.action($("#action-input").value);
    $("#action-dialog").close();
  } catch (err) {
    $("#action-error").textContent = errorText(err);
    $("#action-error").hidden = false;
  } finally {
    state.actionBusy = false;
    $("#action-submit").disabled = false;
  }
};
$("#mkdir").onclick = () => {
  const parent = state.path;
  actionDialog({
    title: "新建文件夹",
    description: "在当前云端目录中创建文件夹。",
    run: async (name) => {
      await api("Mkdir", parent, name);
      toast("文件夹已创建");
      await loadDir(parent, state.page);
    },
  });
};
$("#rename").onclick = () => {
  const file = selectedFiles()[0],
    parent = state.path;
  if (!file) return;
  actionDialog({
    title: "重命名",
    value: file.name,
    description: "同名文件不会被覆盖。",
    run: async (name) => {
      await api("Rename", joined(parent, file.name), name);
      toast("已重命名");
      await loadDir(parent, state.page);
    },
  });
};
$("#remove").onclick = () => {
  const names = [...state.selected],
    parent = state.path;
  if (!names.length) return;
  actionDialog({
    title: `删除 ${names.length} 个项目？`,
    description: `将从云端删除所选文件和文件夹，文件夹中的内容也会删除。此操作无法在客户端撤销。\n\n${names.slice(0, 4).join("、")}${names.length > 4 ? " 等" : ""}`,
    remove: true,
    submit: "确认删除",
    run: async () => {
      await api("Remove", parent, names);
      toast("所选项目已删除");
      await loadDir(parent, 1);
    },
  });
};
function directoryPassword() {
  const target = state.pendingPath || state.path;
  actionDialog({
    title: "目录访问密码",
    description: `目标目录：${target}\n将用于访问此目录及其子目录，仅在当前会话保留。`,
    password: true,
    value: passwordFor(target),
    run: async (value) => {
      state.passwords.set(target, value);
      await loadDir(target, 1);
    },
  });
}
$("#directory-password").onclick = directoryPassword;
$("#error-password").onclick = directoryPassword;
$("#dismiss-error").onclick = () => {
  $("#file-error").hidden = true;
};
$("#refresh").onclick = () => loadDir(state.path, state.page);
$("#up").onclick = () => loadDir(cleanPath(state.path + "/.."));
$("#path-form").onsubmit = (event) => {
  event.preventDefault();
  loadDir($("#path-input").value);
};
$("#filter").oninput = (event) => {
  state.filter = event.target.value;
  renderFiles();
};
$("#select-all").onchange = (event) => {
  for (const file of shownFiles())
    event.target.checked
      ? state.selected.add(file.name)
      : state.selected.delete(file.name);
  renderFiles();
};
$("#clear-selection").onclick = () => {
  state.selected.clear();
  renderFiles();
};
$$("[data-sort]").forEach(
  (button) =>
    (button.onclick = () => {
      const field = button.dataset.sort;
      if (state.sort === field) state.ascending = !state.ascending;
      else {
        state.sort = field;
        state.ascending = true;
      }
      renderFiles();
    }),
);
$("#prev-page").onclick = () => loadDir(state.path, state.page - 1);
$("#next-page").onclick = () => loadDir(state.path, state.page + 1);
$("#copy-link").onclick = async () => {
  const file = selectedFiles()[0];
  if (!file) return;
  try {
    await api(
      "CopyDownloadLink",
      joined(state.path, file.name),
      passwordFor(state.path),
    );
    toast("下载链接已复制。链接包含访问签名，请只分享给信任的人。");
  } catch (err) {
    toast(errorText(err), true);
  }
};
async function queue(method, args) {
  if (state.queuing) return;
  const session = state.connection;
  const target = args[0];
  const isUpload = method.startsWith("Upload");
  let refreshFolders = false;
  state.queuing = true;
  updateControls();
  try {
    const result = await api(method, ...args);
    if (state.connection !== session) return;
    if (result.count) {
      toast(
        `已加入 ${result.count} 个${isUpload ? "上传" : "下载"}任务${result.folders ? `，已准备 ${result.folders} 个文件夹` : ""}`,
      );
      await pollTransfers();
      switchView("transfers");
    } else if (result.folders) {
      toast(`已处理 ${result.folders} 个文件夹`);
      refreshFolders = true;
    }
    if (result.errors?.length) {
      const messages = result.errors.slice(0, 10);
      if (result.errors.length > 10)
        messages.push(`另有 ${result.errors.length - 10} 项失败`);
      toast(messages.join("\n"), true);
    }
  } catch (err) {
    toast(errorText(err), true);
  } finally {
    state.queuing = false;
    updateControls();
    if (refreshFolders && state.connection === session && state.path === target)
      loadDir(target, state.page);
  }
}
$("#upload").onclick = () => queue("Upload", [state.path]);
$("#upload-folder").onclick = () => queue("UploadFolder", [state.path]);
$("#download").onclick = () =>
  queue("Download", [
    state.path,
    selectedFiles()
      .filter((f) => !f.is_dir)
      .map((f) => f.name),
    passwordFor(state.path),
  ]);

const statusLabels = {
  queued: "等待中",
  running: "正在传输",
  completed: "已完成",
  failed: "传输失败",
  cancelled: "已取消",
  paused: "已暂停",
  pausing: "正在暂停",
  cancelling: "正在取消",
};
function remainingTime(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) return "剩余时间未知";
  if (seconds < 60) return `剩余 ${Math.ceil(seconds)} 秒`;
  if (seconds < 3600) return `剩余 ${Math.ceil(seconds / 60)} 分钟`;
  if (seconds < 86400) {
    const minutes = Math.ceil(seconds / 60);
    return `剩余 ${Math.floor(minutes / 60)} 小时 ${minutes % 60} 分钟`;
  }
  return `剩余约 ${(seconds / 86400).toFixed(1)} 天`;
}
function transferStatus(task) {
  if (task.status === "running") {
    if (task.phase === "checking")
      return task.direction === "upload" && task.resumable
        ? `校验源文件 ${task.phase_progress || 0}%`
        : "准备中";
    if (task.phase === "committing") return "等待服务端确认";
  }
  return statusLabels[task.status] || task.status;
}
async function transferAction(task, method, password = "") {
  try {
    if (method === "ResumeTransfer") await api(method, task.id, password);
    else await api(method, task.id);
    await pollTransfers();
  } catch (err) {
    toast(errorText(err), true);
  }
}
function continueTransfer(task) {
  if (task.needs_password) {
    actionDialog({
      title: "下载访问密码",
      description: `文件：${task.name}\n密码仅在当前会话中保留。`,
      password: true,
      run: async (value) => {
        await api("ResumeTransfer", task.id, value);
        await pollTransfers();
      },
    });
  } else transferAction(task, "ResumeTransfer");
}
function renderTransfers() {
  const active = state.tasks.filter((t) =>
    ["queued", "running", "pausing", "cancelling"].includes(t.status),
  ).length;
  const pending = state.tasks.filter(
    (t) => t.status === "paused" || (t.status === "failed" && t.resumable),
  ).length;
  const completed = state.tasks.filter((t) => t.status === "completed").length;
  $("#transfer-count").textContent = active + pending;
  $("#transfer-count").title = `${active} 项进行中，${pending} 项待继续`;
  $("#stats-active").textContent = active;
  $("#stats-completed").textContent = completed;
  $("#stats-paused").textContent = pending;
  $("#transfer-notice").textContent = !state.connection
    ? "连接原服务器和账号后，可继续已保存的任务。"
    : state.connection.upload_resume
      ? "续传记录保存在本机；上传续传受服务端会话有效期限制。"
      : "当前服务器不支持上传续传，下载续传取决于文件源站。";
  const tasks = state.tasks.filter(
    (t) =>
      state.transferFilter === "all" || t.direction === state.transferFilter,
  );
  const list = $("#transfer-list");
  const existing = new Map(
    [...list.children].map((row) => [Number(row.dataset.id), row]),
  );
  const wanted = new Set(tasks.map((t) => t.id));
  for (const [id, row] of existing) if (!wanted.has(id)) row.remove();
  $("#empty-transfers").hidden = tasks.length > 0;
  tasks.forEach((task, index) => {
    let row = existing.get(task.id);
    if (!row) {
      row = element("div", "transfer-row");
      row.dataset.id = String(task.id);
      const fileIcon = element("span", "transfer-file");
      fileIcon.innerHTML = icon(
        task.direction === "upload" ? "upload" : "download",
      );
      const info = element("div", "transfer-info");
      const title = element("div", "transfer-title");
      title.append(
        element("strong", "transfer-name"),
        element("span", "status"),
      );
      const meta = element("div", "transfer-meta");
      meta.append(
        element("span", "transfer-amount"),
        element("span", "transfer-speed"),
      );
      const track = element("div", "progress-track");
      track.append(element("div", "progress-fill"));
      info.append(
        title,
        meta,
        track,
        element("div", "transfer-path"),
        element("div", "transfer-error"),
        element("div", "transfer-resume-note"),
      );
      row.append(fileIcon, info, element("div", "transfer-actions"));
    }
    row.className = "transfer-row " + task.status;
    const name = row.querySelector(".transfer-name");
    name.textContent = task.name;
    name.title = task.name;
    const status = row.querySelector(".status");
    status.className = "status " + task.status;
    status.textContent =
      (!task.resumable &&
      task.direction === "upload" &&
      task.status === "running"
        ? "普通上传 · "
        : "") + transferStatus(task);
    status.title = task.resume_reason || "";
    const percent =
      task.status === "completed"
        ? 100
        : task.total > 0
          ? Math.min(100, Math.floor((task.done / task.total) * 100))
          : 0;
    row.querySelector(".transfer-amount").textContent =
      `${task.direction === "upload" ? "上传" : "下载"} · ${bytes(task.done)}${task.total > 0 ? " / " + bytes(task.total) : ""}`;
    const speed = row.querySelector(".transfer-speed");
    speed.textContent =
      task.status === "running" && task.phase === "transferring"
        ? `${bytes(task.speed_bps || 0)}/s · ${remainingTime(task.eta_seconds)} · ${percent}%`
        : `${percent}%`;
    row.querySelector(".progress-fill").style.width = percent + "%";
    const path = row.querySelector(".transfer-path");
    path.textContent = task.local_path || task.remote_path;
    path.title = `${task.server || ""}\n${task.remote_path}\n${task.local_path || ""}`;
    const error = row.querySelector(".transfer-error");
    error.textContent = task.error || "";
    error.hidden = !task.error;
    const note = row.querySelector(".transfer-resume-note");
    note.textContent =
      task.status === "paused" && !task.can_resume
        ? task.resume_reason || "连接原服务器和账号后继续"
        : "";
    note.hidden = !note.textContent;
    const actions = row.querySelector(".transfer-actions");
    const actionKey = [
      task.status,
      task.can_pause,
      task.can_resume,
      task.resumable,
      task.needs_password,
      task.resume_reason,
    ].join("|");
    if (actions.dataset.key !== actionKey) {
      actions.dataset.key = actionKey;
      actions.replaceChildren();
      if (task.can_pause) {
        const pause = element("button", "text-button task-control", "暂停");
        pause.title = "保留进度并暂停";
        pause.setAttribute("aria-label", `暂停传输 ${task.name}`);
        pause.onclick = () => transferAction(task, "PauseTransfer");
        actions.append(pause);
      }
      if (task.resumable && ["paused", "failed"].includes(task.status)) {
        const resume = element("button", "text-button task-control", "继续");
        resume.disabled = !task.can_resume;
        resume.title = task.can_resume
          ? "从已确认的位置继续"
          : task.resume_reason || "请先连接原服务器";
        resume.setAttribute("aria-label", `继续传输 ${task.name}`);
        resume.onclick = () => continueTransfer(task);
        actions.append(resume);
      }
      if (["running", "queued", "paused", "failed"].includes(task.status)) {
        const cancel = element("button", "icon-button");
        cancel.innerHTML = icon("close");
        cancel.title = "取消任务并清理临时进度";
        cancel.setAttribute("aria-label", `取消传输 ${task.name}`);
        cancel.onclick = () => transferAction(task, "CancelTransfer");
        actions.append(cancel);
      }
    }
    if (list.children[index] !== row)
      list.insertBefore(row, list.children[index] || null);
  });
}
async function pollTransfers() {
  if (state.pollBusy || !window.go?.main?.App) return;
  state.pollBusy = true;
  try {
    const tasks = await api("Transfers"),
      signature = JSON.stringify(tasks);
    if (signature === state.tasksSignature) return;
    const previous = new Map(state.tasks.map((t) => [t.id, t]));
    const uploadsFinished = tasks.some(
      (t) =>
        t.direction === "upload" &&
        t.status === "completed" &&
        previous.get(t.id)?.status !== "completed" &&
        cleanPath(t.remote_path + "/..") === state.path,
    );
    state.tasks = tasks;
    state.tasksSignature = signature;
    renderTransfers();
    if (
      uploadsFinished &&
      state.connection &&
      state.view === "files" &&
      !state.loading
    )
      loadDir(state.path, state.page);
  } catch {
    /* Keep polling without turning transient bridge errors into repeated notifications. */
  } finally {
    state.pollBusy = false;
  }
}
$("#clear-finished").onclick = async () => {
  try {
    await api("ClearFinishedTransfers");
    await pollTransfers();
  } catch (err) {
    toast(errorText(err), true);
  }
};
$$("[data-transfer-filter]").forEach(
  (button) =>
    (button.onclick = () => {
      state.transferFilter = button.dataset.transferFilter;
      $$("[data-transfer-filter]").forEach((b) =>
        b.classList.toggle("active", b === button),
      );
      renderTransfers();
    }),
);
$("#nav-files").onclick = () => {
  switchView("files");
  if (state.connection) loadDir(state.path, state.page);
};
$("#nav-transfers").onclick = () => switchView("transfers");
$("#back-files").onclick = () => switchView("files");
document.addEventListener("keydown", (event) => {
  if (
    event.key === "/" &&
    !document.querySelector("dialog[open]") &&
    !["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement.tagName) &&
    state.connection &&
    state.view === "files"
  ) {
    event.preventDefault();
    $("#filter").focus();
  }
  if (event.key === "F5" && state.connection) {
    event.preventDefault();
    loadDir(state.path, state.page);
  }
});
async function init() {
  // Wails resolves real OS paths through WebView2. Browser File objects alone
  // cannot provide these paths and must not be used as a fallback.
  if (window.runtime?.OnFileDrop) {
    window.runtime.OnFileDrop((x, y, paths) => {
      if (!state.connection || state.view !== "files") return;
      if (
        state.loading ||
        state.queuing ||
        state.connecting ||
        state.actionBusy ||
        document.querySelector("dialog[open]")
      ) {
        toast("请先完成当前操作，再拖入文件。", true);
        return;
      }
      if (!state.listing.write || !can(3)) {
        toast("当前云端目录或账号没有上传权限。", true);
        return;
      }
      if (Array.isArray(paths) && paths.length)
        queue("UploadDroppedFiles", [state.path, paths]);
    }, true);
  }
  // Prevent opening local files as pages, including drops outside the upload area.
  for (const eventName of ["dragover", "drop"]) {
    window.addEventListener(eventName, (event) => {
      if ([...(event.dataTransfer?.types || [])].includes("Files"))
        event.preventDefault();
    });
  }
  try {
    if (window.go?.main?.App) {
      const profile = await api("LoadProfile"),
        form = $("#connection-form");
      for (const name of ["url", "username", "start_path", "auth_mode"])
        if (profile[name]) form.elements[name].value = profile[name];
      updateAuthFields();
    }
  } catch (err) {
    toast("读取上次连接信息失败：" + errorText(err), true);
  }
  renderConnection();
  setInterval(pollTransfers, 900);
}
init();
