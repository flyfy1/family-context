import React, { useEffect, useMemo, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";

const FAMILY_ID = "our-family";
const ROUTES = ["/feed", "/space", "/elder", "/settings"];
const colors = ["#AD4C34", "#54706A", "#B47A3C", "#715A75", "#607D4F", "#35677B"];
const DEFAULT_API_BASE = (import.meta.env.VITE_API_BASE_URL || "").replace(/\/$/, "");

function loadConfig() {
  return {
    familyName: localStorage.getItem("fd.familyName") || "我们的家",
    apiBase: (localStorage.getItem("fd.apiBase") || DEFAULT_API_BASE).replace(/\/$/, ""),
    token: localStorage.getItem("fd.token") || "family-daily-local",
  };
}

function useHashRoute() {
  const read = () => ROUTES.includes(location.hash.slice(1)) ? location.hash.slice(1) : "/feed";
  const [route, setRoute] = useState(read);
  useEffect(() => {
    if (!location.hash) location.hash = "/feed";
    const change = () => setRoute(read());
    addEventListener("hashchange", change);
    return () => removeEventListener("hashchange", change);
  }, []);
  return route;
}

function App() {
  const route = useHashRoute();
  const [config, setConfig] = useState(loadConfig);
  const [members, setMembers] = useState([]);
  const [currentMemberId, setCurrentMemberId] = useState(localStorage.getItem("fd.currentMember") || "");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const [toast, setToast] = useState("");

  const api = useMemo(() => createAPI(config), [config]);
  const currentMember = members.find(member => member.id === currentMemberId) || members[0] || null;

  useEffect(() => {
    let active = true;
    setLoading(true);
    api("/api/v1/members")
      .then(data => {
        if (!active) return;
        setMembers(data.members || []);
        setError("");
      })
      .catch(err => active && setError(err.message))
      .finally(() => active && setLoading(false));
    return () => { active = false; };
  }, [api, refreshKey]);

  useEffect(() => {
    if (!currentMemberId && members[0]) setCurrentMemberId(members[0].id);
    if (currentMemberId && !members.some(member => member.id === currentMemberId) && members[0]) setCurrentMemberId(members[0].id);
  }, [members, currentMemberId]);

  useEffect(() => {
    if (currentMemberId) localStorage.setItem("fd.currentMember", currentMemberId);
  }, [currentMemberId]);

  function notify(message) {
    setToast(message);
    window.clearTimeout(notify.timer);
    notify.timer = window.setTimeout(() => setToast(""), 2800);
  }

  function refresh() { setRefreshKey(value => value + 1); }

  const pageProps = { api, members, currentMember, notify, refreshKey, refresh };

  return <div className="app-shell">
    <Sidebar route={route} familyName={config.familyName} />
    <main className="main-shell">
      <Topbar route={route} members={members} currentMember={currentMember} onMemberChange={setCurrentMemberId} />
      {error && route !== "/settings" && <ConnectionError message={error} onSettings={() => { location.hash = "/settings"; }} />}
      {route === "/settings" && <Settings api={api} config={config} setConfig={setConfig} members={members} refresh={refresh} notify={notify} />}
      {!error && !loading && members.length === 0 && route !== "/settings" && <Onboarding onStart={() => { location.hash = "/settings"; }} />}
      {!error && members.length > 0 && route !== "/settings" && <>
        {route === "/feed" && <FamilyFeed {...pageProps} />}
        {route === "/space" && <MemberSpace {...pageProps} />}
        {route === "/elder" && <ElderView {...pageProps} />}
      </>}
      {loading && <PageLoading />}
    </main>
    <MobileNav route={route} />
    <div className={`toast ${toast ? "show" : ""}`} role="status">{toast}</div>
  </div>;
}

function Sidebar({ route, familyName }) {
  return <aside className="sidebar">
    <a href="#/feed" className="brand"><span>家</span><div>Family Daily<small>{familyName}</small></div></a>
    <nav>
      <NavLink to="/feed" route={route} icon="⌂">家庭动态</NavLink>
      <NavLink to="/space" route={route} icon="◫">我的 Space</NavLink>
      <NavLink to="/elder" route={route} icon="声">老人模式</NavLink>
    </nav>
    <div className="sidebar-bottom">
      <NavLink to="/settings" route={route} icon="⚙">家庭设置</NavLink>
      <p><i /> 本地家庭服务器</p>
    </div>
  </aside>;
}

function NavLink({ to, route, icon, children }) {
  return <a href={`#${to}`} className={route === to ? "active" : ""}><span className="nav-icon">{icon}</span>{children}</a>;
}

function Topbar({ route, members, currentMember, onMemberChange }) {
  const titles = { "/feed": ["家庭动态", "看看大家最近发生了什么"], "/space": ["我的 Space", "属于你的独立记录空间"], "/elder": ["老人模式", "说一说，听听家里的今天"], "/settings": ["家庭设置", "成员和连接配置"] };
  return <header className="topbar">
    <div><p>{titles[route][1]}</p><h1>{titles[route][0]}</h1></div>
    {members.length > 0 && <label className="member-switcher"><span>当前身份</span><select value={currentMember?.id || ""} onChange={event => onMemberChange(event.target.value)}>{members.map(member => <option key={member.id} value={member.id}>{member.name}{member.role === "elder" ? " · 老人" : member.role === "child" ? " · 孩子" : ""}</option>)}</select></label>}
  </header>;
}

function FamilyFeed({ api, members, currentMember, notify, refreshKey }) {
  const [updates, setUpdates] = useState([]);
  const [summary, setSummary] = useState(null);
  const [loading, setLoading] = useState(true);
  const reload = () => Promise.all([
    api("/api/v1/updates?scope=family").then(data => setUpdates(data.updates || [])),
    api("/api/v1/daily-summaries/latest").then(data => setSummary(data.summary)),
  ]).finally(() => setLoading(false));
  useEffect(() => { reload().catch(error => notify(error.message)); }, [refreshKey, api]);

  return <div className="page-grid feed-layout">
    <section>
      <Composer api={api} currentMember={currentMember} notify={notify} onCreated={reload} />
      <SectionHeading eyebrow="SHARED CONTEXT" title="一家人的近况" count={updates.length} />
      {loading ? <CardSkeleton /> : updates.length ? <div className="update-list">{updates.map(update => <UpdateCard key={update.id} update={update} member={members.find(item => item.id === update.memberId)} api={api} notify={notify} />)}</div> : <EmptyState icon="✦" title="家庭动态还是空的" text="发布第一条家庭可见 Update，让大家知道你今天过得怎么样。" />}
    </section>
    <aside className="right-rail">
      <DailyCard api={api} summary={summary} onGenerated={setSummary} notify={notify} />
      <FamilyMembers members={members} />
      <PrivacyCard />
    </aside>
  </div>;
}

function Composer({ api, currentMember, notify, onCreated }) {
  const [text, setText] = useState("");
  const [image, setImage] = useState(null);
  const [visibility, setVisibility] = useState("family");
  const [busy, setBusy] = useState(false);
  const inputRef = useRef(null);
  const previewURL = useMemo(() => image ? URL.createObjectURL(image) : "", [image]);
  useEffect(() => () => { if (previewURL) URL.revokeObjectURL(previewURL); }, [previewURL]);
  if (!currentMember) return null;
  async function submit(event) {
    event.preventDefault();
    if (!text.trim() && !image) return;
    setBusy(true);
    try {
      if (image) {
        const body = new FormData();
        body.append("familyId", FAMILY_ID);
        body.append("memberId", currentMember.id);
        body.append("visibility", visibility);
        body.append("text", text.trim());
        body.append("image", image, image.name);
        await api("/api/v1/updates/image", { method: "POST", body });
      } else {
        await api("/api/v1/updates", { method: "POST", body: JSON.stringify({ familyId: FAMILY_ID, memberId: currentMember.id, text: text.trim(), visibility }) });
      }
      setText("");
      setImage(null);
      if (inputRef.current) inputRef.current.value = "";
      notify(visibility === "family" ? "已经分享给家人" : "已经保存到你的 Space");
      await onCreated();
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }
  return <form className="composer card" onSubmit={submit}>
    <div className="composer-person"><Avatar member={currentMember} /><div><strong>{currentMember.name}</strong><span>分享一个生活片段</span></div></div>
    <textarea value={text} onChange={event => setText(event.target.value)} maxLength="2000" placeholder="今天发生了什么？写几句话就好……" aria-label="新的家庭动态" />
    {previewURL && <div className="image-preview"><img src={previewURL} alt="待发布图片预览" /><button type="button" onClick={() => { setImage(null); if (inputRef.current) inputRef.current.value = ""; }}>移除图片</button></div>}
    <div className="composer-actions">
      <div className="composer-options">
        <Visibility value={visibility} onChange={setVisibility} />
        <label className="image-picker">▧ 添加照片<input ref={inputRef} type="file" accept="image/jpeg,image/png,image/webp,image/gif" aria-label="选择一张照片" onChange={event => setImage(event.target.files?.[0] || null)} /></label>
      </div>
      <button className="primary-button" disabled={busy || (!text.trim() && !image)}>{busy ? "正在保存…" : image ? "发布照片" : "发布 Update"}</button>
    </div>
  </form>;
}

function Visibility({ value, onChange, large = false }) {
  return <div className={`visibility ${large ? "large" : ""}`}><button type="button" className={value === "family" ? "active" : ""} onClick={() => onChange("family")}>◉ 家庭可见</button><button type="button" className={value === "private" ? "active" : ""} onClick={() => onChange("private")}>◌ 仅自己</button></div>;
}

function UpdateCard({ update, member, api, notify }) {
  const [showTranscript, setShowTranscript] = useState(false);
  return <article className="update-card card">
    <div className="update-meta"><Avatar member={member} /><div><strong>{member?.name || "家庭成员"}</strong><span>{formatTime(update.createdAt)} · {update.visibility === "private" ? "仅自己" : "家庭可见"}</span></div><span className="update-type">{update.type === "voice" ? "语音" : update.type === "image" ? "照片" : "文字"}</span></div>
    <p className="update-text">{update.text}</p>
    {update.type === "image" && update.mediaUrl && <ImageAttachment path={update.mediaUrl} api={api} notify={notify} />}
    {update.type === "voice" && <div className="voice-actions"><AudioButton path={update.audioUrl} api={api} notify={notify} />{update.transcript && <button className="text-button" onClick={() => setShowTranscript(value => !value)}>{showTranscript ? "收起转写" : "查看转写"}</button>}</div>}
    {showTranscript && <div className="transcript"><small>语音转写</small>{update.transcript}</div>}
    <div className="update-footer"><span>♡</span><span>来自 {update.source === "member_voice" ? "语音记录" : "成员分享"}</span></div>
  </article>;
}

function ImageAttachment({ path, api, notify }) {
  const [url, setURL] = useState("");
  useEffect(() => {
    let active = true;
    let objectURL = "";
    api(path, { raw: true }).then(blob => {
      if (!active) return;
      objectURL = URL.createObjectURL(blob);
      setURL(objectURL);
    }).catch(error => notify(error.message));
    return () => { active = false; if (objectURL) URL.revokeObjectURL(objectURL); };
  }, [api, path]);
  return url ? <img className="update-image" src={url} alt="家庭成员分享的照片" /> : <div className="image-loading">正在打开照片…</div>;
}

function MemberSpace({ api, members, currentMember, notify, refreshKey }) {
  const [updates, setUpdates] = useState([]);
  const [filter, setFilter] = useState("all");
  useEffect(() => {
    if (!currentMember) return;
    api(`/api/v1/updates?scope=mine&memberId=${encodeURIComponent(currentMember.id)}`).then(data => setUpdates(data.updates || [])).catch(error => notify(error.message));
  }, [api, currentMember?.id, refreshKey]);
  if (!currentMember) return null;
  const shown = updates.filter(update => filter === "all" || update.visibility === filter);
  return <div className="space-page">
    <section className="space-hero" style={{ "--member-color": currentMember.color }}><Avatar member={currentMember} large /><div><p className="eyebrow">MEMBER SPACE</p><h2>{currentMember.name}的空间</h2><p>每一条记录都先属于你，由你决定是否分享给家庭。</p></div><div className="space-stats"><strong>{updates.length}</strong><span>条记录</span><strong>{updates.filter(item => item.visibility === "family").length}</strong><span>已分享</span></div></section>
    <div className="space-toolbar"><SectionHeading eyebrow="LOCAL FILE SPACE" title="我的记录" /><div className="filter-pills">{[["all","全部"],["private","仅自己"],["family","已分享"]].map(([value,label]) => <button key={value} className={filter === value ? "active" : ""} onClick={() => setFilter(value)}>{label}</button>)}</div></div>
    {shown.length ? <div className="update-list narrow">{shown.map(update => <UpdateCard key={update.id} update={update} member={members.find(item => item.id === update.memberId)} api={api} notify={notify} />)}</div> : <EmptyState icon="◫" title="这里还没有记录" text="你发布的私人和家庭 Update 会出现在这里，并同步保存到个人文件空间。" />}
  </div>;
}

function ElderView({ api, members, currentMember, notify, refreshKey }) {
  const elder = currentMember?.role === "elder" ? currentMember : members.find(member => member.role === "elder") || currentMember;
  const [summary, setSummary] = useState(null);
  const [updates, setUpdates] = useState([]);
  const reload = () => Promise.all([api("/api/v1/daily-summaries/latest").then(data => setSummary(data.summary)), api("/api/v1/updates?scope=family").then(data => setUpdates((data.updates || []).slice(0, 3)))]);
  useEffect(() => { reload().catch(error => notify(error.message)); }, [api, refreshKey]);
  if (!elder) return null;
  return <div className="elder-page">
    <section className="elder-hero">
      <p className="elder-date">{formatFullDate(new Date())}</p>
      <h2>{elder.name}，今天过得怎么样？</h2>
      <p>按一下按钮，像平时聊天一样说就好。</p>
      <VoiceRecorder api={api} member={elder} notify={notify} onCreated={reload} />
    </section>
    <section className="elder-summary card">
      <div className="elder-summary-heading"><span>☀</span><div><p className="eyebrow">FAMILY DAILY</p><h2>我们家今天</h2></div></div>
      {summary ? <><p className="summary-content">{summary.content}</p><small>根据 {summary.updateCount} 条家庭动态整理 · {summary.date}</small></> : <p className="muted-copy">今天的家庭日报还没有生成。家人分享近况后，可以在家庭动态页生成。</p>}
    </section>
    <section><SectionHeading eyebrow="家人的消息" title="最近更新" />{updates.length ? <div className="elder-update-grid">{updates.map(update => <UpdateCard key={update.id} update={update} member={members.find(item => item.id === update.memberId)} api={api} notify={notify} />)}</div> : <EmptyState icon="☀" title="还没有新的家庭消息" text="家人发布的 Update 会显示在这里。" />}</section>
  </div>;
}

function VoiceRecorder({ api, member, notify, onCreated }) {
  const [recording, setRecording] = useState(false);
  const [busy, setBusy] = useState(false);
  const [seconds, setSeconds] = useState(0);
  const [visibility, setVisibility] = useState("family");
  const state = useRef(null);

  useEffect(() => () => state.current?.stream?.getTracks().forEach(track => track.stop()), []);
  useEffect(() => {
    if (!recording) return;
    const timer = setInterval(() => setSeconds(value => value + 1), 1000);
    return () => clearInterval(timer);
  }, [recording]);

  async function toggle() {
    if (recording) return state.current.recorder.stop();
    if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) return notify("当前浏览器不支持录音");
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const types = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4"];
      const mimeType = types.find(type => MediaRecorder.isTypeSupported(type)) || "";
      const recorder = new MediaRecorder(stream, mimeType ? { mimeType } : undefined);
      const chunks = [];
      recorder.ondataavailable = event => event.data.size && chunks.push(event.data);
      recorder.onstop = async () => {
        stream.getTracks().forEach(track => track.stop());
        setRecording(false); setBusy(true);
        const type = recorder.mimeType || "audio/webm";
        const form = new FormData();
        form.append("familyId", FAMILY_ID); form.append("memberId", member.id); form.append("visibility", visibility);
        form.append("audio", new Blob(chunks, { type }), `update.${type.includes("mp4") ? "m4a" : "webm"}`);
        try { await api("/api/v1/updates/voice", { method: "POST", body: form }); notify("语音已经整理并保存"); await onCreated(); }
        catch (error) { notify(error.message); }
        finally { setBusy(false); setSeconds(0); state.current = null; }
      };
      state.current = { recorder, stream };
      recorder.start(); setSeconds(0); setRecording(true); notify("正在录音，请自然地说话");
    } catch (error) { notify(error.name === "NotAllowedError" ? "请允许浏览器使用麦克风" : "暂时无法开始录音"); }
  }
  return <div className="recorder"><button className={`record-orb ${recording ? "recording" : ""}`} disabled={busy} onClick={toggle}><span>{busy ? "…" : recording ? "■" : "●"}</span><strong>{busy ? "AI 正在整理" : recording ? `${formatDuration(seconds)} · 结束录音` : "按住今天的故事"}</strong></button><Visibility value={visibility} onChange={setVisibility} large /></div>;
}

function DailyCard({ api, summary, onGenerated, notify }) {
  const [busy, setBusy] = useState(false);
  async function generate() {
    setBusy(true);
    try {
      const result = await api("/api/v1/daily-summaries/generate", { method: "POST", body: JSON.stringify({ familyId: FAMILY_ID, date: localDate() }) });
      onGenerated(result); notify("今天的家庭日报已经生成");
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }
  return <section className="daily-card card"><div className="daily-sun">☀</div><p className="eyebrow">FAMILY DAILY</p><h2>我们家今天</h2>{summary ? <><p>{summary.content}</p><small>{summary.date} · {summary.updateCount} 条动态</small></> : <p className="muted-copy">把今天家人分享的片段，整理成一份温暖、简短的日报。</p>}<button className="secondary-button wide" disabled={busy} onClick={generate}>{busy ? "正在整理…" : summary?.date === localDate() ? "重新生成今日摘要" : "生成今日摘要"}</button></section>;
}

function FamilyMembers({ members }) {
  return <section className="rail-card"><div className="rail-title"><strong>家庭成员</strong><a href="#/settings">管理</a></div><div className="member-row">{members.map(member => <div key={member.id} title={member.name}><Avatar member={member} /><span>{member.name}</span></div>)}</div></section>;
}

function PrivacyCard() { return <section className="privacy-card"><span>⌂</span><div><strong>本地优先</strong><p>成员 Space、原始录音、摘要和历史保存在家庭服务器。语音会发送给 Gemini 做一次性整理。</p></div></section>; }

function Settings({ api, config, setConfig, members, refresh, notify }) {
  const [form, setForm] = useState(config);
  const [newMember, setNewMember] = useState({ name: "", role: "member", color: colors[members.length % colors.length] });
  const [busy, setBusy] = useState(false);
  function saveConnection(event) {
    event.preventDefault();
    const next = { ...form, apiBase: form.apiBase.replace(/\/$/, "") };
    localStorage.setItem("fd.familyName", next.familyName); localStorage.setItem("fd.apiBase", next.apiBase); localStorage.setItem("fd.token", next.token);
    setConfig(next); notify("网页配置已保存在当前浏览器");
  }
  async function addMember(event) {
    event.preventDefault(); setBusy(true);
    try {
      await api("/api/v1/members", { method: "POST", body: JSON.stringify({ familyId: FAMILY_ID, ...newMember }) });
      setNewMember({ name: "", role: "member", color: colors[(members.length + 1) % colors.length] });
      notify("成员和独立 Space 已创建"); refresh();
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }
  return <div className="settings-page">
    <section className="settings-section card"><div><p className="eyebrow">BROWSER CONFIG</p><h2>连接设置</h2><p className="muted-copy">前端可以部署到 GitHub Pages；API 地址指向实际运行的 Go 后端。</p></div><form className="settings-form" onSubmit={saveConnection}><label>家庭名称<input value={form.familyName} onChange={event => setForm({ ...form, familyName: event.target.value })} required /></label><label>后端 API 地址<input value={form.apiBase} onChange={event => setForm({ ...form, apiBase: event.target.value })} placeholder="本地留空；远程填写 https://api.example.com" /></label><label>家庭访问令牌<input type="password" value={form.token} onChange={event => setForm({ ...form, token: event.target.value })} required /></label><button className="primary-button">保存连接</button></form></section>
    <section className="settings-section card"><div><p className="eyebrow">MEMBER SPACES</p><h2>家庭成员</h2><p className="muted-copy">每位成员会获得独立的本地文件目录。老人角色可以使用专用的大按钮页面；孩子角色可用于后续的专属配置。</p></div><div><div className="member-list">{members.map(member => <div className="member-list-item" key={member.id}><Avatar member={member} /><div><strong>{member.name}</strong><span>{member.role === "elder" ? "老人模式" : member.role === "child" ? "孩子" : "家庭成员"} · 独立 Space</span></div><code>{member.id.slice(0, 8)}</code></div>)}</div><form className="add-member" onSubmit={addMember}><input value={newMember.name} maxLength="30" onChange={event => setNewMember({ ...newMember, name: event.target.value })} placeholder="成员称呼" required /><select value={newMember.role} onChange={event => setNewMember({ ...newMember, role: event.target.value })}><option value="member">普通成员</option><option value="elder">老人</option><option value="child">孩子</option></select><div className="color-picker">{colors.map(color => <button type="button" aria-label={`选择颜色 ${color}`} key={color} className={newMember.color === color ? "active" : ""} style={{ background: color }} onClick={() => setNewMember({ ...newMember, color })} />)}</div><button className="primary-button" disabled={busy}>{busy ? "正在创建…" : "+ 添加成员"}</button></form></div></section>
    <section className="future-boundary"><strong>下一阶段边界</strong><p>MCP、成员定时分析任务和家庭摄像头/NAS 数据源会建立在这些独立 Space 上。当前 PoC 不开放任意文件系统访问，也不自动读取监控录像。</p></section>
  </div>;
}

function AudioButton({ path, api, notify }) {
  const [playing, setPlaying] = useState(false);
  async function play() {
    if (playing) return;
    setPlaying(true);
    try {
      const blob = await api(path, { raw: true });
      const url = URL.createObjectURL(blob);
      const audio = new Audio(url);
      audio.onended = () => { URL.revokeObjectURL(url); setPlaying(false); };
      audio.onerror = () => { URL.revokeObjectURL(url); setPlaying(false); notify("暂时无法播放原声"); };
      await audio.play();
    } catch (error) { setPlaying(false); notify(error.message); }
  }
  return <button className="audio-button" onClick={play}>{playing ? "▮▮ 正在播放" : "▶ 播放原声"}<i /><i /><i /><i /></button>;
}

function SectionHeading({ eyebrow, title, count }) { return <div className="section-heading"><div><p className="eyebrow">{eyebrow}</p><h2>{title}</h2></div>{count !== undefined && <span>{count} 条</span>}</div>; }
function Avatar({ member, large = false }) { return <span className={`avatar ${large ? "large" : ""}`} style={{ background: member?.color || "#AD4C34" }}>{member?.name?.slice(0, 1) || "家"}</span>; }
function EmptyState({ icon, title, text }) { return <div className="empty-state"><span>{icon}</span><strong>{title}</strong><p>{text}</p></div>; }
function PageLoading() { return <div className="page-loading"><i /><span>正在打开家庭空间…</span></div>; }
function CardSkeleton() { return <div className="card skeleton"><i /><i /><i /></div>; }
function ConnectionError({ message, onSettings }) { return <div className="connection-error"><span>!</span><div><strong>暂时无法连接家庭服务器</strong><p>{message}</p></div><button onClick={onSettings}>检查设置</button></div>; }
function Onboarding({ onStart }) { return <div className="onboarding"><span className="onboarding-mark">家</span><p className="eyebrow">WELCOME TO FAMILY DAILY</p><h2>先创建你的家庭成员</h2><p>每个人都会获得独立的本地 Space，然后就可以开始分享日常。</p><button className="primary-button" onClick={onStart}>开始配置家庭 →</button></div>; }
function MobileNav({ route }) { return <nav className="mobile-nav"><NavLink to="/feed" route={route} icon="⌂">动态</NavLink><NavLink to="/space" route={route} icon="◫">Space</NavLink><NavLink to="/elder" route={route} icon="声">老人</NavLink><NavLink to="/settings" route={route} icon="⚙">设置</NavLink></nav>; }

function createAPI(config) {
  return async (path, options = {}) => {
    const headers = new Headers(options.headers || {});
    headers.set("X-Family-Token", config.token);
    if (options.body && !(options.body instanceof FormData)) headers.set("Content-Type", "application/json");
    const response = await fetch(`${config.apiBase}${path}`, { ...options, headers });
    if (options.raw) {
      if (!response.ok) throw new Error("暂时无法读取文件");
      return response.blob();
    }
    if (response.status === 204) return null;
    const body = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(body.error || `请求失败（${response.status}）`);
    return body;
  };
}

function formatTime(value) { return new Intl.DateTimeFormat("zh-CN", { month: "long", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value)); }
function formatFullDate(value) { return new Intl.DateTimeFormat("zh-CN", { month: "long", day: "numeric", weekday: "long" }).format(value); }
function formatDuration(value) { return `${String(Math.floor(value / 60)).padStart(2, "0")}:${String(value % 60).padStart(2, "0")}`; }
function localDate() { const date = new Date(); return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`; }

createRoot(document.getElementById("root")).render(<React.StrictMode><App /></React.StrictMode>);
