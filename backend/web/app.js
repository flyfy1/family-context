const $ = (selector, root = document) => root.querySelector(selector);
const feed = $("#feed");
const statusBox = $("#status");
const toast = $("#toast");
let activeRecording = null;
let toastTimer = null;
let allQuestions = [];
let activeFilter = "all";
let config = {
  familyName: localStorage.getItem("familyDailyFamilyName") || "我们的家",
  memberName: localStorage.getItem("familyDailyMemberName") || "洋宇",
  elderName: localStorage.getItem("familyDailyElderName") || "爸爸",
  token: localStorage.getItem("familyDailyToken") || "family-daily-local"
};

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  headers.set("X-Family-Token", config.token);
  if (options.body && !(options.body instanceof FormData)) headers.set("Content-Type", "application/json");
  const response = await fetch(path, { ...options, headers });
  if (response.status === 204) return null;
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `请求失败（${response.status}）`);
  return body;
}

function setStatus(message) { statusBox.textContent = message; }
function notify(message) {
  toast.textContent = message;
  toast.classList.add("show");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast.classList.remove("show"), 2600);
}
function escapeHTML(value = "") {
  return value.replace(/[&<>'"]/g, character => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;" })[character]);
}
function formatDate(value) {
  return new Intl.DateTimeFormat("zh-CN", { month: "long", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value));
}

async function loadFeed() {
  setStatus("正在读取家庭动态……");
  try {
    const data = await api("/api/v1/questions");
    allQuestions = data.questions || [];
    updateFilterCounts();
    renderFilteredFeed();
    setStatus("");
  } catch (error) {
    setStatus(`暂时无法连接：${error.message}`);
  }
}

function renderFeed(questions) {
  if (!questions.length) {
    feed.innerHTML = `<div class="empty"><div><strong>还没有家庭对话</strong>从左边写下一句具体的关心开始吧。</div></div>`;
    return;
  }
  feed.innerHTML = questions.map(questionCard).join("");
  bindCardActions();
}

function questionState(question) {
  if (!question.answer) return "pending";
  if (question.answer.status === "shared") return "shared";
  return "ready";
}

function renderFilteredFeed() {
  renderFeed(activeFilter === "all" ? allQuestions : allQuestions.filter(question => questionState(question) === activeFilter));
}

function updateFilterCounts() {
  const counts = { all: allQuestions.length, pending: 0, ready: 0, shared: 0 };
  allQuestions.forEach(question => counts[questionState(question)]++);
  document.querySelectorAll("[data-filter]").forEach(button => $("span", button).textContent = counts[button.dataset.filter]);
}

function questionCard(question) {
  const answer = question.answer;
  let answerHTML = `<button class="button button-primary record-button" data-record="${question.id}" type="button">🎙 开始语音回答</button>`;
  if (answer) {
    if (answer.status === "processing_failed") {
      answerHTML = `<div class="answer-box"><div class="error-note">${escapeHTML(answer.errorMessage || "AI 暂时没有完成整理，原始录音已经安全保存。")}</div><button class="button button-secondary" data-archive="${answer.id}" type="button">归档并重新录制</button></div>`;
    } else {
      const shared = answer.status === "shared";
      const replies = (question.replies || []).map(reply => `<div class="reply"><strong>${escapeHTML(reply.authorId)}</strong>：${escapeHTML(reply.text)}</div>`).join("");
      answerHTML = `<div class="answer-box">
        <div class="answer-label">AI 整理</div>
        <p class="answer-summary">${escapeHTML(answer.aiSummary || answer.transcript || "正在整理录音……")}</p>
        <div class="answer-actions">
          <button class="button button-secondary" data-play="${escapeHTML(answer.audioUrl)}" type="button">▶ 播放原声</button>
          <button class="button button-quiet" data-transcript type="button">查看转写</button>
          <button class="button button-quiet" data-history="${question.id}" type="button">本地历史</button>
          ${answer.status === "ready" ? `<button class="button button-quiet" data-archive="${answer.id}" type="button">重新录制</button><button class="button button-primary" data-publish="${answer.id}" type="button">确认分享</button>` : ""}
        </div>
        <div class="transcript" hidden><div class="answer-label">语音转写原文</div>${escapeHTML(answer.transcript || "暂无转写内容")}</div>
        ${answer.status === "ready" ? `<div class="draft-note">这还是一份家庭草稿，确认后才会分享。</div>` : ""}
        ${shared ? `<div class="replies"><div class="answer-label">家人的回复</div>${replies || `<div class="reply">还没有回复，写一句关心吧。</div>`}<form class="reply-form" data-reply="${answer.id}"><input class="reply-input" name="reply" maxlength="500" placeholder="写一句回复……" required><button class="button button-secondary" type="submit">回复爸爸</button></form></div>` : ""}
      </div>`;
    }
  }
  return `<article class="question-card">
    <div class="question-meta"><span class="person"><span class="avatar">${escapeHTML((question.askedBy || "家").slice(0, 1))}</span>${escapeHTML(question.askedBy)} 想问 ${escapeHTML(question.askedTo)}</span><time>${formatDate(question.createdAt)}</time></div>
    <div class="question-text">${escapeHTML(question.text)}</div>
    ${answerHTML}
  </article>`;
}

function bindCardActions() {
  feed.querySelectorAll("[data-record]").forEach(button => button.addEventListener("click", () => toggleRecording(button.dataset.record, button)));
  feed.querySelectorAll("[data-publish]").forEach(button => button.addEventListener("click", () => act(button, `/api/v1/answers/${button.dataset.publish}/publish`, "回答已分享给家人")));
  feed.querySelectorAll("[data-archive]").forEach(button => button.addEventListener("click", () => archiveAnswer(button.dataset.archive, button)));
  feed.querySelectorAll("[data-play]").forEach(button => button.addEventListener("click", () => playAudio(button.dataset.play, button)));
  feed.querySelectorAll("[data-transcript]").forEach(button => button.addEventListener("click", () => toggleTranscript(button)));
  feed.querySelectorAll("[data-history]").forEach(button => button.addEventListener("click", () => showHistory(button.dataset.history)));
  feed.querySelectorAll("[data-reply]").forEach(form => form.addEventListener("submit", sendReply));
}

async function toggleRecording(questionId, button) {
  if (activeRecording?.questionId === questionId) {
    activeRecording.recorder.stop();
    return;
  }
  if (activeRecording) return notify("请先结束当前录音");
  if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) return notify("当前浏览器不支持录音，请使用最新版 Chrome 或 Safari");
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const candidates = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4"];
    const mimeType = candidates.find(type => MediaRecorder.isTypeSupported(type)) || "";
    const recorder = new MediaRecorder(stream, mimeType ? { mimeType } : undefined);
    const chunks = [];
    recorder.addEventListener("dataavailable", event => { if (event.data.size) chunks.push(event.data); });
    recorder.addEventListener("stop", async () => {
      stream.getTracks().forEach(track => track.stop());
      button.disabled = true;
      button.classList.remove("button-recording");
      button.textContent = "正在保存并由 AI 整理……";
      const blobType = recorder.mimeType || "audio/webm";
      const extension = blobType.includes("mp4") ? "m4a" : "webm";
      const form = new FormData();
      form.append("answeredBy", config.elderName);
      form.append("audio", new Blob(chunks, { type: blobType }), `answer.${extension}`);
      activeRecording = null;
      try {
        await api(`/api/v1/questions/${questionId}/answer`, { method: "POST", body: form });
        notify("录音已经整理好，请确认内容");
        await loadFeed();
      } catch (error) {
        notify(error.message);
        await loadFeed();
      }
    });
    recorder.start();
    activeRecording = { questionId, recorder, stream };
    button.classList.add("button-recording");
    button.textContent = "■ 结束录音并上传";
    notify("正在录音，请自然地说话");
  } catch (error) {
    notify(error.name === "NotAllowedError" ? "请允许浏览器使用麦克风" : "暂时无法开始录音");
  }
}

async function act(button, path, successMessage) {
  button.disabled = true;
  try {
    await api(path, { method: "POST", body: "{}" });
    notify(successMessage);
    await loadFeed();
  } catch (error) {
    button.disabled = false;
    notify(error.message);
  }
}

async function archiveAnswer(answerId, button) {
  if (!confirm("当前录音会从家庭草稿中移除，并作为本地历史版本保留。继续吗？")) return;
  await act(button, `/api/v1/answers/${answerId}/archive`, "草稿已归档，可以重新录制");
}

async function playAudio(path, button) {
  const original = button.textContent;
  button.disabled = true;
  button.textContent = "正在加载原声……";
  try {
    const response = await fetch(path, { headers: { "X-Family-Token": config.token } });
    if (!response.ok) throw new Error("暂时无法播放原声");
    const url = URL.createObjectURL(await response.blob());
    const audio = new Audio(url);
    audio.addEventListener("ended", () => { URL.revokeObjectURL(url); button.disabled = false; button.textContent = original; });
    audio.addEventListener("error", () => { URL.revokeObjectURL(url); button.disabled = false; button.textContent = original; notify("暂时无法播放原声"); });
    await audio.play();
    button.textContent = "正在播放……";
  } catch (error) {
    button.disabled = false;
    button.textContent = original;
    notify(error.message);
  }
}

async function sendReply(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const input = form.elements.reply;
  const button = $("button", form);
  button.disabled = true;
  try {
    await api(`/api/v1/answers/${form.dataset.reply}/replies`, { method: "POST", body: JSON.stringify({ authorId: config.memberName, text: input.value.trim() }) });
    notify("回复已发送");
    await loadFeed();
  } catch (error) {
    button.disabled = false;
    notify(error.message);
  }
}

$("#question-form").addEventListener("submit", async event => {
  event.preventDefault();
  const input = $("#question-input");
  const button = $("button[type=submit]", event.currentTarget);
  button.disabled = true;
  try {
    await api("/api/v1/questions", { method: "POST", body: JSON.stringify({ familyId: "our-family", askedBy: config.memberName, askedTo: config.elderName, text: input.value.trim() }) });
    input.value = "";
    notify("问题已经发给爸爸");
    await loadFeed();
  } catch (error) {
    notify(error.message);
  } finally {
    button.disabled = false;
  }
});

document.querySelectorAll("[data-prompt]").forEach(button => button.addEventListener("click", () => { $("#question-input").value = button.dataset.prompt; $("#question-input").focus(); }));
$("#refresh-button").addEventListener("click", loadFeed);
document.querySelectorAll("[data-filter]").forEach(button => button.addEventListener("click", () => {
  activeFilter = button.dataset.filter;
  document.querySelectorAll("[data-filter]").forEach(item => item.classList.toggle("active", item === button));
  renderFilteredFeed();
}));

function toggleTranscript(button) {
  const transcript = button.closest(".answer-box").querySelector(".transcript");
  transcript.hidden = !transcript.hidden;
  button.textContent = transcript.hidden ? "查看转写" : "收起转写";
}

async function showHistory(questionId) {
  const dialog = $("#history-dialog");
  const content = $("#history-content");
  content.innerHTML = `<div class="status">正在读取本地历史……</div>`;
  dialog.showModal();
  try {
    const history = await api(`/api/v1/questions/${questionId}/history`);
    const versions = [...(history.current ? [history.current] : []), ...(history.archived || [])];
    content.innerHTML = versions.length ? versions.map((answer, index) => `<article class="history-item">
      <div class="history-meta"><strong>${answer.archivedAt ? "已归档版本" : "当前版本"}</strong><time>${formatDate(answer.createdAt)}</time></div>
      <p>${escapeHTML(answer.aiSummary || answer.transcript || answer.errorMessage || "暂无整理内容")}</p>
      ${answer.transcript ? `<details><summary>查看转写原文</summary><div>${escapeHTML(answer.transcript)}</div></details>` : ""}
    </article>`).join("") : `<div class="empty"><div><strong>还没有回答历史</strong>完成第一次录音后会显示在这里。</div></div>`;
  } catch (error) {
    content.innerHTML = `<div class="error-note">${escapeHTML(error.message)}</div>`;
  }
}

function openSettings() {
  $("#family-name-input").value = config.familyName;
  $("#member-name-input").value = config.memberName;
  $("#elder-name-input").value = config.elderName;
  $("#token-input").value = config.token;
  $("#settings-dialog").showModal();
}

$("#settings-button").addEventListener("click", openSettings);
$("#settings-form").addEventListener("submit", event => {
  event.preventDefault();
  config = {
    familyName: $("#family-name-input").value.trim(),
    memberName: $("#member-name-input").value.trim(),
    elderName: $("#elder-name-input").value.trim(),
    token: $("#token-input").value.trim()
  };
  localStorage.setItem("familyDailyFamilyName", config.familyName);
  localStorage.setItem("familyDailyMemberName", config.memberName);
  localStorage.setItem("familyDailyElderName", config.elderName);
  localStorage.setItem("familyDailyToken", config.token);
  $("#family-name-label").textContent = config.familyName;
  $("#settings-dialog").close();
  notify("家庭配置已保存在当前浏览器");
  loadFeed();
});
document.querySelectorAll("[data-close-settings]").forEach(button => button.addEventListener("click", () => $("#settings-dialog").close()));
$("[data-close-history]").addEventListener("click", () => $("#history-dialog").close());
$("#family-name-label").textContent = config.familyName;
loadFeed();
