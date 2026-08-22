import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";
import LandingPage from "./LandingPage";
import { CoreJobSettings, NotificationInbox } from "./coreJobs";
import MemberRoleEditor from "./MemberRoleEditor";
import ActivityThreads from "./ActivityThreads";
import { createAPI, isUnauthorized } from "./api";

const FAMILY_ID = "our-family";
const ROUTES = ["/", "/feed", "/space", "/elder", "/settings"];
const colors = ["#AD4C34", "#54706A", "#B47A3C", "#715A75", "#607D4F", "#35677B"];
const DEFAULT_API_BASE = (import.meta.env.VITE_API_BASE_URL || "").replace(/\/$/, "");
const LanguageContext = createContext({ language: "en", tx: en => en });

function useLanguage() { return useContext(LanguageContext); }

function loadConfig() {
  const language = localStorage.getItem("fd.language") === "en" ? "en" : "zh";
  return {
    familyName: localStorage.getItem("fd.familyName") || (language === "zh" ? "我们的家" : "Our Family"),
    apiBase: (localStorage.getItem("fd.apiBase") || DEFAULT_API_BASE).replace(/\/$/, ""),
    sessionToken: localStorage.getItem("fd.sessionToken") || "",
    adminToken: "",
    language,
  };
}

function useHashRoute() {
  const read = () => ROUTES.includes(location.hash.slice(1) || "/") ? (location.hash.slice(1) || "/") : "/";
  const [route, setRoute] = useState(read);
  useEffect(() => {
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
  const [signedInMember, setSignedInMember] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const [toast, setToast] = useState("");

  const api = useMemo(() => createAPI(config), [config]);
  const language = config.language;
  const tx = useMemo(() => (english, chinese) => language === "zh" ? chinese : english, [language]);
  const currentMember = signedInMember;

  const clearSession = useCallback(() => {
    localStorage.removeItem("fd.sessionToken");
    setConfig(current => ({ ...current, sessionToken: "", adminToken: "" }));
    setSignedInMember(null);
    setMembers([]);
    setError("");
  }, []);

  useEffect(() => {
    if (route === "/" || !config.sessionToken) { setLoading(false); return; }
    let active = true;
    setLoading(true);
    Promise.all([api("/api/v1/me"), api("/api/v1/members")])
      .then(([member, data]) => {
        if (!active) return;
        setSignedInMember(member);
        setMembers(data.members || []);
        setError("");
      })
      .catch(err => {
        if (!active) return;
        if (isUnauthorized(err)) clearSession();
        else setError(err.message);
      })
      .finally(() => active && setLoading(false));
    return () => { active = false; };
  }, [api, clearSession, config.sessionToken, refreshKey, route]);

  useEffect(() => {
    localStorage.setItem("fd.language", language);
    document.documentElement.lang = language === "zh" ? "zh-CN" : "en";
  }, [language]);

  function notify(message) {
    setToast(message);
    window.clearTimeout(notify.timer);
    notify.timer = window.setTimeout(() => setToast(""), 2800);
  }

  function refresh() { setRefreshKey(value => value + 1); }

  function signedIn(credential) {
    localStorage.setItem("fd.sessionToken", credential.accessToken);
    setConfig(current => ({ ...current, sessionToken: credential.accessToken }));
    setSignedInMember(credential.member);
    location.hash = credential.member?.role === "elder" ? "/elder" : "/feed";
  }

  async function signOut() {
    try { await api("/api/v1/auth/logout", { method: "POST" }); } catch {}
    clearSession();
  }

  const pageProps = { api, members, currentMember, notify, refreshKey, refresh };

  if (route === "/") return <LanguageContext.Provider value={{ language, tx }}><LandingPage language={language} onLanguageChange={next => setConfig(current => ({ ...current, language: next }))} /></LanguageContext.Provider>;
  if (!config.sessionToken) return <LanguageContext.Provider value={{ language, tx }}><LoginPage config={config} onSignedIn={signedIn} /></LanguageContext.Provider>;

  const elderMode = route === "/elder";
  return <LanguageContext.Provider value={{ language, tx }}><div className={`app-shell ${elderMode ? "elder-shell" : ""}`}>
    {!elderMode && <Sidebar route={route} familyName={config.familyName} />}
    <main className="main-shell">
      {!elderMode && <Topbar route={route} currentMember={currentMember} onSignOut={signOut} language={language} onLanguageChange={next => setConfig(current => ({ ...current, language: next }))} />}
      {!elderMode && !error && <NotificationInbox api={api} currentMember={currentMember} notify={notify} refreshKey={refreshKey} language={language} />}
      {error && route !== "/settings" && <ConnectionError message={error} onSettings={() => { location.hash = "/settings"; }} />}
      {route === "/settings" && <Settings api={api} config={config} setConfig={setConfig} members={members} currentMember={currentMember} refresh={refresh} notify={notify} />}
      {!error && !loading && members.length === 0 && route !== "/settings" && <Onboarding onStart={() => { location.hash = "/settings"; }} />}
      {!error && members.length > 0 && route !== "/settings" && <>
        {route === "/feed" && <FamilyFeed {...pageProps} />}
        {route === "/space" && <MemberSpace {...pageProps} />}
        {route === "/elder" && <ElderView {...pageProps} />}
      </>}
      {loading && <PageLoading />}
    </main>
    {!elderMode && <MobileNav route={route} />}
    <div className={`toast ${toast ? "show" : ""}`} role="status">{toast}</div>
  </div></LanguageContext.Provider>;
}

function Sidebar({ route, familyName }) {
  const { tx } = useLanguage();
  return <aside className="sidebar">
    <a href="#/feed" className="brand"><span>家</span><div>Family Daily<small>{familyName}</small></div></a>
    <nav>
      <NavLink to="/feed" route={route} icon="⌂">{tx("Family feed", "家庭动态")}</NavLink>
      <NavLink to="/space" route={route} icon="◫">{tx("My Space", "我的 Space")}</NavLink>
      <NavLink to="/elder" route={route} icon="声">{tx("Elder mode", "老人模式")}</NavLink>
    </nav>
    <div className="sidebar-bottom">
      <NavLink to="/settings" route={route} icon="⚙">{tx("Family settings", "家庭设置")}</NavLink>
      <p><i /> {tx("Local family server", "本地家庭服务器")}</p>
    </div>
  </aside>;
}

function NavLink({ to, route, icon, children }) {
  return <a href={`#${to}`} className={route === to ? "active" : ""}><span className="nav-icon">{icon}</span>{children}</a>;
}

function Topbar({ route, currentMember, onSignOut, language, onLanguageChange }) {
  const { tx } = useLanguage();
  const titles = { "/feed": [tx("Family feed", "家庭动态"), tx("See what everyone has been up to", "看看大家最近发生了什么")], "/space": [tx("My Space", "我的 Space"), tx("Your own private record space", "属于你的独立记录空间")], "/elder": [tx("Elder mode", "老人模式"), tx("Speak, then hear about the family's day", "说一说，听听家里的今天")], "/settings": [tx("Family settings", "家庭设置"), tx("Members and connection settings", "成员和连接配置")] };
  return <header className="topbar">
    <div><p>{titles[route][1]}</p><h1>{titles[route][0]}</h1></div>
    <div className="topbar-actions"><label className="language-switcher"><span>{tx("Language", "语言")}</span><select aria-label={tx("Language", "语言")} value={language} onChange={event => onLanguageChange(event.target.value)}><option value="en">English</option><option value="zh">中文</option></select></label>{currentMember && <div className="signed-in-member"><span>{tx("Signed in as", "当前登录")}</span><strong>{currentMember.name}{currentMember.isAdmin ? tx(" · Administrator", " · 管理员") : ""}</strong></div>}<button type="button" className="secondary-button" onClick={onSignOut}>{tx("Sign out", "退出")}</button></div>
  </header>;
}

function LoginPage({ config, onSignedIn }) {
  const { tx } = useLanguage();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(event) {
    event.preventDefault(); setBusy(true); setError("");
    try {
      const response = await fetch(`${config.apiBase}/api/v1/auth/login`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ username, password }) });
      const body = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(body.error || tx("Unable to sign in", "无法登录"));
      onSignedIn(body);
    } catch (loginError) { setError(loginError.message); }
    finally { setBusy(false); }
  }
  return <main className="login-page"><section className="login-card card"><a href="#/" className="brand"><span>家</span><div>Family Daily<small>{config.familyName}</small></div></a><div><p className="eyebrow">MEMBER LOGIN</p><h1>{tx("Welcome home", "欢迎回家")}</h1><p className="muted-copy">{tx("Sign in with your own family username and password.", "请使用你自己的家庭用户名和密码登录。")}</p></div><form className="settings-form" onSubmit={submit}><label>{tx("Username", "用户名")}<input autoComplete="username" value={username} onChange={event => setUsername(event.target.value)} required /></label><label>{tx("Password", "密码")}<input type="password" autoComplete="current-password" value={password} onChange={event => setPassword(event.target.value)} required /></label>{error && <p className="login-error" role="alert">{error}</p>}<button className="primary-button" disabled={busy}>{busy ? tx("Signing in…", "正在登录…") : tx("Sign in", "登录")}</button></form></section></main>;
}

function FamilyFeed({ api, members, currentMember, notify, refreshKey, refresh }) {
  const { language, tx } = useLanguage();
  const [updates, setUpdates] = useState([]);
  const [summary, setSummary] = useState(null);
  const [loading, setLoading] = useState(true);
  const reload = () => Promise.all([
    api("/api/v1/updates?scope=family").then(data => setUpdates(data.updates || [])),
    api(`/api/v1/daily-summaries/latest?language=${language}`).then(data => setSummary(data.summary)),
  ]).finally(() => setLoading(false));
  useEffect(() => { reload().catch(error => notify(error.message)); }, [refreshKey, api]);
  const attentionMembers = members.filter(member => member.role === "elder" && member.needsAttention);

  async function dismissAttention(member) {
    try {
      await api(`/api/v1/me/members/${encodeURIComponent(member.id)}/attention/dismiss`, { method: "POST" });
      notify(tx(`${member.name} no longer needs attention`, `已取消对${member.name}的关注状态`));
      refresh();
    } catch (error) { notify(error.message); }
  }

  return <div className="page-grid feed-layout">
    <section>
      {attentionMembers.length > 0 && <AttentionBanner members={attentionMembers} onDismiss={dismissAttention} />}
      <Composer api={api} currentMember={currentMember} notify={notify} onCreated={reload} />
      <SectionHeading eyebrow="SHARED CONTEXT" title={tx("What the family is sharing", "一家人的近况")} count={updates.length} />
      {loading ? <CardSkeleton /> : updates.length ? <div className="update-list">{updates.map(update => <UpdateCard key={update.id} update={update} member={members.find(item => item.id === update.memberId)} api={api} notify={notify} />)}</div> : <EmptyState icon="✦" title={tx("The family feed is empty", "家庭动态还是空的")} text={tx("Share the first family update and let everyone know how your day is going.", "发布第一条家庭可见 Update，让大家知道你今天过得怎么样。")} />}
    </section>
    <aside className="right-rail">
      <DailyCard api={api} summary={summary} onGenerated={setSummary} notify={notify} />
      <FamilyMembers members={members} />
      <PrivacyCard />
    </aside>
  </div>;
}

function AttentionBanner({ members, onDismiss }) {
  const { tx } = useLanguage();
  const [busyID, setBusyID] = useState("");
  async function dismiss(member) {
    setBusyID(member.id);
    try { await onDismiss(member); }
    finally { setBusyID(""); }
  }
  return <section className="attention-banner" role="status"><span aria-hidden="true">!</span><div className="attention-content"><strong>{tx("Needs attention", "需要关注")}</strong><p>{tx("These elders have had no recent activity. Check in, then clear the status with one tap.", "以下老人最近没有活动；联系确认后，可以点击一次取消状态。")}</p><div className="attention-members">{members.map(member => <div key={member.id}><b>{member.name}</b><button type="button" disabled={busyID === member.id} onClick={() => dismiss(member)}>{busyID === member.id ? tx("Clearing…", "取消中……") : tx("Checked in · Clear", "已联系 · 取消关注")}</button></div>)}</div></div></section>;
}

function Composer({ api, currentMember, notify, onCreated }) {
  const { tx } = useLanguage();
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
      notify(visibility === "family" ? tx("Shared with your family", "已经分享给家人") : tx("Saved to your Space", "已经保存到你的 Space"));
      await onCreated();
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }
  return <form className="composer card" onSubmit={submit}>
    <div className="composer-person"><Avatar member={currentMember} /><div><strong>{currentMember.name}</strong><span>{tx("Share a moment from your day", "分享一个生活片段")}</span></div></div>
    <textarea value={text} onChange={event => setText(event.target.value)} maxLength="2000" placeholder={tx("What happened today? A few words are enough…", "今天发生了什么？写几句话就好……")} aria-label={tx("New family update", "新的家庭动态")} />
    {previewURL && <div className="image-preview"><img src={previewURL} alt={tx("Photo preview", "待发布图片预览")} /><button type="button" onClick={() => { setImage(null); if (inputRef.current) inputRef.current.value = ""; }}>{tx("Remove photo", "移除图片")}</button></div>}
    <div className="composer-actions">
      <div className="composer-options">
        <Visibility value={visibility} onChange={setVisibility} />
        <label className="image-picker">▧ {tx("Add photo", "添加照片")}<input ref={inputRef} type="file" accept="image/jpeg,image/png,image/webp,image/gif" aria-label={tx("Choose a photo", "选择一张照片")} onChange={event => setImage(event.target.files?.[0] || null)} /></label>
      </div>
      <button className="primary-button" disabled={busy || (!text.trim() && !image)}>{busy ? tx("Saving…", "正在保存…") : image ? tx("Post photo", "发布照片") : tx("Post update", "发布 Update")}</button>
    </div>
  </form>;
}

function Visibility({ value, onChange, large = false }) {
  const { tx } = useLanguage();
  return <div className={`visibility ${large ? "large" : ""}`}><button type="button" className={value === "family" ? "active" : ""} onClick={() => onChange("family")}>◉ {tx("Family", "家庭可见")}</button><button type="button" className={value === "private" ? "active" : ""} onClick={() => onChange("private")}>◌ {tx("Only me", "仅自己")}</button></div>;
}

function UpdateCard({ update, member, api, notify }) {
  const { language, tx } = useLanguage();
  const [showTranscript, setShowTranscript] = useState(false);
  return <article className="update-card card">
    <div className="update-meta"><Avatar member={member} /><div><strong>{member?.name || tx("Family member", "家庭成员")}</strong><span>{formatTime(update.createdAt, language)} · {update.visibility === "private" ? tx("Only me", "仅自己") : tx("Family", "家庭可见")}</span></div><span className="update-type">{update.type === "voice" ? tx("Voice", "语音") : update.type === "image" ? tx("Photo", "照片") : update.type === "video" ? tx("Video", "视频") : tx("Text", "文字")}</span></div>
    <p className="update-text">{update.text}</p>
    {update.type === "image" && update.mediaUrl && <ImageAttachment path={update.mediaUrl} api={api} notify={notify} />}
    {update.type === "video" && update.mediaUrl && <VideoAttachment path={update.mediaUrl} api={api} notify={notify} />}
    {update.type === "voice" && <div className="voice-actions"><AudioButton path={update.audioUrl} api={api} notify={notify} />{update.transcript && <button className="text-button" onClick={() => setShowTranscript(value => !value)}>{showTranscript ? tx("Hide transcript", "收起转写") : tx("View transcript", "查看转写")}</button>}</div>}
    {showTranscript && <div className="transcript"><small>{tx("VOICE TRANSCRIPT", "语音转写")}</small>{update.transcript}</div>}
    <div className="update-footer"><span>♡</span><span>{tx("From", "来自")} {update.source === "member_voice" ? tx("voice journal", "语音记录") : tx("member sharing", "成员分享")}</span></div>
  </article>;
}

function ImageAttachment({ path, api, notify }) {
  const { tx } = useLanguage();
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
  return url ? <img className="update-image" src={url} alt={tx("Photo shared by a family member", "家庭成员分享的照片")} /> : <div className="image-loading">{tx("Opening photo…", "正在打开照片…")}</div>;
}

function VideoAttachment({ path, api, notify }) {
  const { tx } = useLanguage();
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
  return url ? <video className="update-video" src={url} controls preload="metadata">{tx("Your browser cannot play this video.", "当前浏览器无法播放这段视频。")}</video> : <div className="image-loading">{tx("Opening video…", "正在打开视频…")}</div>;
}

function MemberSpace({ api, members, currentMember, notify, refreshKey }) {
  const { language, tx } = useLanguage();
  const [updates, setUpdates] = useState([]);
  const [familyUpdates, setFamilyUpdates] = useState([]);
  const [mediaImports, setMediaImports] = useState([]);
  const [filter, setFilter] = useState("all");
  const [selected, setSelected] = useState([]);
  const [captions, setCaptions] = useState({});
  const [sharing, setSharing] = useState(false);
  const [activeFolder, setActiveFolder] = useState("all");
  const [focusedId, setFocusedId] = useState("");
  const reload = () => {
    if (!currentMember) return;
    return Promise.all([
      api(`/api/v1/updates?scope=mine&memberId=${encodeURIComponent(currentMember.id)}`).then(data => setUpdates(data.updates || [])),
      api("/api/v1/updates?scope=family").then(data => setFamilyUpdates((data.updates || []).filter(item => item.type === "image" || item.type === "video"))),
      api("/api/v1/me/media-imports", { headers: { "X-Member-ID": currentMember.id } }).then(data => setMediaImports(data.mediaImports || [])),
    ]);
  };
  useEffect(() => {
    setSelected([]);
    setActiveFolder("all");
    setFocusedId("");
    reload().catch(error => notify(error.message));
  }, [api, currentMember?.id, refreshKey]);
  if (!currentMember) return null;
  const shown = updates.filter(update => filter === "all" || update.visibility === filter);
  const selectable = mediaImports.filter(item => !item.updateId);
  const visibleMedia = mediaImports.filter(item => {
    if (activeFolder === "all") return true;
    const [mediaType, month] = activeFolder.split(":");
    if (item.mediaType !== mediaType) return false;
    return !month || mediaMonthKey(item) === month;
  });
  const visibleSelectable = visibleMedia.filter(item => !item.updateId);
  function toggleSelection(id) {
    setSelected(current => current.includes(id) ? current.filter(item => item !== id) : [...current, id]);
  }
  function selectAll() {
    const allVisibleSelected = visibleSelectable.length > 0 && visibleSelectable.every(item => selected.includes(item.id));
    setSelected(current => allVisibleSelected ? current.filter(id => !visibleSelectable.some(item => item.id === id)) : [...new Set([...current, ...visibleSelectable.map(item => item.id)])]);
  }
  async function shareSelected() {
    if (!selected.length) return;
    setSharing(true);
    let shared = 0;
    try {
      for (const id of selected) {
        const item = mediaImports.find(candidate => candidate.id === id);
        await api(`/api/v1/me/media-imports/${encodeURIComponent(id)}/decision`, {
          method: "POST",
          headers: { "X-Member-ID": currentMember.id },
          body: JSON.stringify({ visibility: "family", caption: captions[id] ?? item?.analysis?.suggestedCaption ?? "" }),
        });
        shared += 1;
      }
      setSelected([]);
      await reload();
      notify(tx(`${shared} item(s) shared to the family Space`, `已将 ${shared} 个文件分享到家庭公共 Space`));
    } catch (error) {
      await reload().catch(() => {});
      notify(shared ? tx(`${shared} shared; remaining items need another try`, `已分享 ${shared} 个，其余文件请重试`) : error.message);
    } finally { setSharing(false); }
  }
  return <div className="space-page">
    <section className="space-hero" style={{ "--member-color": currentMember.color }}><Avatar member={currentMember} large /><div><p className="eyebrow">MEMBER SPACE</p><h2>{tx(`${currentMember.name}'s Space`, `${currentMember.name}的空间`)}</h2><p>{tx("Every entry belongs to you first. You decide whether to share it with the family.", "每一条记录都先属于你，由你决定是否分享给家庭。")}</p></div><div className="space-stats"><strong>{updates.length}</strong><span>{tx("entries", "条记录")}</span><strong>{updates.filter(item => item.visibility === "family").length}</strong><span>{tx("shared", "已分享")}</span></div></section>
    <ActivityThreads api={api} members={members} currentMember={currentMember} notify={notify} language={language} refreshKey={refreshKey} />
    <section className="nas-library-section">
      <div className="nas-library-heading"><SectionHeading eyebrow="PRIVATE NAS" title={tx(`${currentMember.name}'s file space`, `${currentMember.name}的文件空间`)} count={mediaImports.length} />{selectable.length > 0 && <div className="nas-bulk-actions"><button type="button" className="text-button" disabled={!visibleSelectable.length} onClick={selectAll}>{visibleSelectable.length > 0 && visibleSelectable.every(item => selected.includes(item.id)) ? tx("Clear visible selection", "取消当前选择") : tx("Select visible files", "选择当前文件")}</button><button type="button" className="primary-button" disabled={!selected.length || sharing} onClick={shareSelected}>{sharing ? tx("Sharing…", "正在分享…") : tx(`Share selected (${selected.length})`, `分享所选（${selected.length}）`)}</button></div>}</div>
      <p className="nas-section-copy">{tx("A clear virtual folder view of this member's private NAS media. Real disk paths stay hidden.", "按文件夹清晰整理当前成员的私人 NAS 媒体；真实磁盘路径不会暴露。")}</p>
      {mediaImports.length
        ? <NasFileSystem items={mediaImports} visibleItems={visibleMedia} activeFolder={activeFolder} onFolderChange={setActiveFolder} focusedId={focusedId} onFocus={setFocusedId} selected={selected} onToggle={toggleSelection} captions={captions} onCaptionChange={(id, value) => setCaptions(current => ({ ...current, [id]: value }))} member={currentMember} api={api} notify={notify} language={language} />
        : <EmptyState icon="▧" title={tx("No synced media yet", "还没有同步媒体")} text={tx("Photos and videos backed up by the mobile app will appear in this private file space.", "手机 APP 备份的照片和视频会出现在这个私人文件空间。")}/>}
    </section>
    <section className="family-media-section"><SectionHeading eyebrow="FAMILY SPACE" title={tx("Shared with everyone", "家庭公共 Space")} count={familyUpdates.length} /><p className="nas-section-copy">{tx("Everyone in the family can see these shared photos and videos.", "这里的照片和视频对所有家庭成员可见。")}</p>{familyUpdates.length ? <div className="family-media-grid">{familyUpdates.map(update => <UpdateCard key={update.id} update={update} member={members.find(item => item.id === update.memberId)} api={api} notify={notify} />)}</div> : <EmptyState icon="✦" title={tx("The family Space is empty", "家庭公共 Space 还是空的")} text={tx("Select media from your private library above to share the first item.", "从上方私人媒体库选择文件，分享第一条内容。")}/>}</section>
    <div className="space-toolbar"><SectionHeading eyebrow="LOCAL FILE SPACE" title={tx("My entries", "我的记录")} /><div className="filter-pills">{[["all",tx("All", "全部")],["private",tx("Only me", "仅自己")],["family",tx("Shared", "已分享")]].map(([value,label]) => <button key={value} className={filter === value ? "active" : ""} onClick={() => setFilter(value)}>{label}</button>)}</div></div>
    {shown.length ? <div className="update-list narrow">{shown.map(update => <UpdateCard key={update.id} update={update} member={members.find(item => item.id === update.memberId)} api={api} notify={notify} />)}</div> : <EmptyState icon="◫" title={tx("No entries here yet", "这里还没有记录")} text={tx("Your private and family updates appear here and are saved to your personal file space.", "你发布的私人和家庭 Update 会出现在这里，并同步保存到个人文件空间。")} />}
  </div>;
}

function mediaMonthKey(item) {
  const date = new Date(item.capturedAt || item.createdAt);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}`;
}

function NasFileSystem({ items, visibleItems, activeFolder, onFolderChange, focusedId, onFocus, selected, onToggle, captions, onCaptionChange, member, api, notify, language }) {
  const { tx } = useLanguage();
  const focused = visibleItems.find(item => item.id === focusedId) || visibleItems[0] || null;
  const groups = ["image", "video"].map(mediaType => ({ mediaType, items: items.filter(item => item.mediaType === mediaType) })).filter(group => group.items.length);
  const folderLabel = activeFolder === "all" ? tx("All files", "全部文件") : activeFolder.startsWith("image") ? tx("Photos", "照片") : tx("Videos", "视频");
  return <div className="nas-explorer card">
    <aside className="nas-folder-tree"><div className="nas-tree-title">▣ <strong>{tx("My NAS", "我的 NAS")}</strong></div><button className={activeFolder === "all" ? "active" : ""} onClick={() => onFolderChange("all")}><span>▤ {tx("All files", "全部文件")}</span><small>{items.length}</small></button>{groups.map(group => { const months = [...new Set(group.items.map(mediaMonthKey))].sort().reverse(); return <div className="nas-tree-group" key={group.mediaType}><button className={activeFolder === group.mediaType ? "active" : ""} onClick={() => onFolderChange(group.mediaType)}><span>▰ {group.mediaType === "image" ? tx("Photos", "照片") : tx("Videos", "视频")}</span><small>{group.items.length}</small></button>{months.map(month => <button key={`${group.mediaType}:${month}`} className={`nested ${activeFolder === `${group.mediaType}:${month}` ? "active" : ""}`} onClick={() => onFolderChange(`${group.mediaType}:${month}`)}><span>└ {formatMediaMonth(month, language)}</span><small>{group.items.filter(item => mediaMonthKey(item) === month).length}</small></button>)}</div>; })}</aside>
    <section className="nas-file-pane"><div className="nas-breadcrumb"><span>{tx("My NAS", "我的 NAS")}</span><b>›</b><strong>{folderLabel}</strong>{activeFolder.includes(":") && <><b>›</b><strong>{formatMediaMonth(activeFolder.split(":")[1], language)}</strong></>}</div><div className="nas-file-header"><span></span><span>{tx("Name", "名称")}</span><span>{tx("Captured", "拍摄时间")}</span><span>{tx("Status", "状态")}</span></div><div className="nas-file-list">{visibleItems.map(item => { const shared = Boolean(item.updateId); return <div className={`nas-file-row ${focused?.id === item.id ? "focused" : ""}`} key={item.id}><input aria-label={tx(`Select ${item.originalName}`, `选择 ${item.originalName}`)} type="checkbox" disabled={shared} checked={selected.includes(item.id)} onChange={() => onToggle(item.id)} /><button className="nas-file-name" onClick={() => onFocus(item.id)}><span>{item.mediaType === "video" ? "▸" : "▧"}</span><strong>{item.originalName}</strong></button><time>{formatFileTime(item.capturedAt || item.createdAt, language)}</time><span className={`nas-file-status ${shared ? "shared" : ""}`}>{shared ? tx("Shared", "已分享") : tx("Private", "仅自己")}</span></div>; })}</div></section>
    <aside className="nas-inspector">{focused ? <FileInspector item={focused} member={member} api={api} notify={notify} caption={captions[focused.id]} onCaptionChange={value => onCaptionChange(focused.id, value)} /> : <div className="nas-no-preview"><span>▧</span><p>{tx("This folder is empty", "这个文件夹是空的")}</p></div>}</aside>
  </div>;
}

function FileInspector({ item, member, api, notify, caption, onCaptionChange }) {
  const { language, tx } = useLanguage();
  const shared = Boolean(item.updateId);
  const recipients = item.analysis?.suggestedRecipients || [];
  return <div className="nas-inspector-content"><div className="nas-inspector-preview">{item.mediaType === "video" ? <VideoAttachment path={item.mediaUrl} api={api} notify={notify} /> : <ImageAttachment path={item.mediaUrl} api={api} notify={notify} />}</div><strong className="nas-inspector-name">{item.originalName}</strong><dl className="nas-file-facts"><div><dt>{tx("Type", "类型")}</dt><dd>{item.mediaType === "video" ? tx("Video", "视频") : tx("Photo", "照片")}</dd></div><div><dt>{tx("Owner", "所有者")}</dt><dd>{member.name}</dd></div><div><dt>{tx("Date", "日期")}</dt><dd>{formatFileTime(item.capturedAt || item.createdAt, language)}</dd></div></dl><label className="nas-caption"><span>{tx("Description when shared", "分享时的描述")}</span><textarea disabled={shared} maxLength="2000" value={caption ?? item.analysis?.suggestedCaption ?? ""} onChange={event => onCaptionChange(event.target.value)} placeholder={tx("Add a description for the family…", "给家人写一段描述……")} /></label>{item.analysis && <details className="nas-audit"><summary>{tx("AI sharing suggestion", "查看 AI 分享建议")}</summary>{item.analysis.summary && <p>{item.analysis.summary}</p>}<dl><div><dt>{tx("Suggested", "建议对象")}</dt><dd>{recipients.length ? recipients.map(recipient => recipient.name).join("、") : tx("Only me", "仅自己")}</dd></div><div><dt>{tx("Actual", "实际对象")}</dt><dd>{shared ? tx("Family", "全体家庭") : tx("Only me", "仅自己")}</dd></div></dl></details>}</div>;
}

function MediaImportAuditCard({ item, member, api, notify }) {
  const { language, tx } = useLanguage();
  const analysis = item.analysis;
  const shared = item.shareDecision === "family" && item.updateId;
  const decidedPrivate = item.shareDecision === "private";
  const recipients = analysis?.suggestedRecipients || [];
  return <article className={`media-audit-card card ${shared ? "shared" : "private"}`}><div className="media-audit-preview"><ImageAttachment path={item.mediaUrl} api={api} notify={notify} /></div><div className="media-audit-content"><div className="media-audit-heading"><div><span className="media-decision">{shared ? tx("Shared with family", "已分享给家庭") : decidedPrivate ? tx("Kept private", "已保留私密") : analysis?.suggestedVisibility === "private" ? tx("AI suggests private", "AI 建议私密") : tx("Waiting for review", "等待审核")}</span><strong>{analysis?.suggestedCaption || item.originalName}</strong></div><small>{formatTime(item.updatedAt, language)}</small></div>{analysis?.summary && <p>{analysis.summary}</p>}<dl className="media-audit-facts"><div><dt>{tx("Shared as", "分享身份")}</dt><dd>{member.name}</dd></div><div><dt>{tx("Actual audience", "实际对象")}</dt><dd>{shared ? tx("Whole family", "全体家庭成员") : tx("Only me", "仅自己")}</dd></div><div><dt>{tx("AI suggested audience", "AI 建议对象")}</dt><dd>{recipients.length ? recipients.map(recipient => recipient.name).join(language === "zh" ? "、" : ", ") : tx("No one", "无人")}</dd></div></dl>{analysis?.recipientReason && <p className="media-recipient-reason">{analysis.recipientReason}</p>}{analysis?.ruleSnapshot && <details><summary>{tx("View the rules used for this decision", "查看本次使用的规则")}</summary><p>{analysis.ruleSnapshot}</p></details>}</div></article>;
}

function ElderView({ api, members, currentMember, notify, refreshKey }) {
  const { language, tx } = useLanguage();
  const [updates, setUpdates] = useState([]);
  const [activeIndex, setActiveIndex] = useState(0);
  const [lastUpdated, setLastUpdated] = useState(null);
  const [refreshing, setRefreshing] = useState(false);
  const touchStart = useRef(null);
  const reload = useCallback(async () => {
    setRefreshing(true);
    try {
      const data = await api("/api/v1/updates?scope=family");
      const nextUpdates = data.updates || [];
      setUpdates(nextUpdates);
      setActiveIndex(index => Math.min(index, Math.max(0, nextUpdates.length - 1)));
      setLastUpdated(new Date());
    } finally { setRefreshing(false); }
  }, [api]);
  useEffect(() => { reload().catch(error => notify(error.message)); }, [reload, refreshKey]);
  useEffect(() => {
    const timer = window.setInterval(() => reload().catch(error => notify(error.message)), 10 * 60 * 1000);
    return () => window.clearInterval(timer);
  }, [reload, notify]);
  const move = useCallback(direction => {
    reload().catch(error => notify(error.message));
    setActiveIndex(index => updates.length ? (index + direction + updates.length) % updates.length : 0);
  }, [reload, notify, updates.length]);
  useEffect(() => {
    function navigate(event) {
      if (["INPUT", "TEXTAREA", "SELECT"].includes(event.target?.tagName)) return;
      if (event.code === "ArrowLeft") { event.preventDefault(); move(-1); }
      if (event.code === "ArrowRight") { event.preventDefault(); move(1); }
    }
    addEventListener("keydown", navigate);
    return () => removeEventListener("keydown", navigate);
  }, [move]);
  if (!currentMember) return null;
  const activeUpdate = updates[activeIndex];
  const activeMember = members.find(member => member.id === activeUpdate?.memberId);
  function finishSwipe(event) {
    if (!touchStart.current) return;
    const deltaX = event.changedTouches[0].clientX - touchStart.current.x;
    const deltaY = event.changedTouches[0].clientY - touchStart.current.y;
    touchStart.current = null;
    if (Math.abs(deltaX) >= 70 && Math.abs(deltaX) > Math.abs(deltaY)) move(deltaX < 0 ? 1 : -1);
  }
  async function enterFullscreen() {
    if (!document.documentElement.requestFullscreen) return notify(tx("Fullscreen is unavailable in this browser", "当前浏览器无法进入全屏"));
    try { await document.documentElement.requestFullscreen(); }
    catch { notify(tx("Fullscreen is unavailable in this browser", "当前浏览器无法进入全屏")); }
  }
  return <div className="elder-page" onTouchStart={event => { const touch = event.touches[0]; touchStart.current = { x: touch.clientX, y: touch.clientY }; }} onTouchEnd={finishSwipe}>
    <header className="elder-controls">
      <a href="#/feed">← {tx("Standard mode", "普通模式")}</a>
      <div><button type="button" onClick={() => reload().catch(error => notify(error.message))} disabled={refreshing}>{refreshing ? tx("Updating…", "更新中…") : tx("Update now", "立即更新")}</button><button type="button" onClick={enterFullscreen}>{tx("Enter fullscreen", "进入全屏")}</button></div>
    </header>
    <section className="elder-stage" aria-live="polite">
      <div className="elder-stage-heading"><div><p className="elder-date">{formatFullDate(new Date(), language)}</p><h1>{tx("Family updates", "家人的最新动态")}</h1></div><span>{lastUpdated ? tx(`Updated ${formatClock(lastUpdated, language)}`, `${formatClock(lastUpdated, language)} 已更新`) : tx("Opening family updates…", "正在读取家人动态…")}</span></div>
      {activeUpdate
        ? <ElderUpdate update={activeUpdate} member={activeMember} api={api} notify={notify} />
        : <EmptyState icon="☀" title={tx("No new family messages", "还没有新的家庭消息")} text={tx("New updates from your family will appear here automatically.", "家人发布新动态后，会自动显示在这里。")} />}
      {updates.length > 1 && <div className="elder-pager"><button type="button" aria-label={tx("Previous update and refresh", "上一条并刷新")} onClick={() => move(-1)}>←</button><span>{activeIndex + 1} / {updates.length} · {tx("Use arrow keys or swipe left and right", "用方向键或左右滑动切换")}</span><button type="button" aria-label={tx("Next update and refresh", "下一条并刷新")} onClick={() => move(1)}>→</button></div>}
    </section>
    <HoldRecorder api={api} member={currentMember} notify={notify} onCreated={reload} />
  </div>;
}

function ElderUpdate({ update, member, api, notify }) {
  const { language, tx } = useLanguage();
  return <article className="elder-update">
    <div className="elder-update-member"><Avatar member={member} large /><div><strong>{member?.name || tx("Family member", "家人")}</strong><span>{formatTime(update.createdAt, language)}</span></div></div>
    <p>{update.text}</p>
    {update.type === "image" && update.mediaUrl && <ImageAttachment path={update.mediaUrl} api={api} notify={notify} />}
    {update.type === "video" && update.mediaUrl && <VideoAttachment path={update.mediaUrl} api={api} notify={notify} />}
    {update.type === "voice" && <AudioButton path={update.audioUrl} api={api} notify={notify} />}
  </article>;
}

function HoldRecorder({ api, member, notify, onCreated }) {
  const { tx } = useLanguage();
  const [recordingMode, setRecordingMode] = useState("");
  const [busy, setBusy] = useState(false);
  const [busyMode, setBusyMode] = useState("");
  const [seconds, setSeconds] = useState(0);
  const [previewStream, setPreviewStream] = useState(null);
  const state = useRef(null);
  const starting = useRef(false);
  const releaseRequested = useRef({ audio: false, video: false });
  const preview = useRef(null);

  useEffect(() => () => state.current?.stream?.getTracks().forEach(track => track.stop()), []);
  useEffect(() => { if (preview.current) preview.current.srcObject = previewStream; }, [previewStream]);
  useEffect(() => {
    if (!recordingMode) return;
    const timer = setInterval(() => setSeconds(value => value + 1), 1000);
    return () => clearInterval(timer);
  }, [recordingMode]);
  useEffect(() => {
    function keyDown(event) {
      if (event.repeat || ["INPUT", "TEXTAREA", "SELECT"].includes(event.target?.tagName)) return;
      const mode = event.code === "Space" ? "audio" : event.code === "KeyV" ? "video" : "";
      if (!mode) return;
      event.preventDefault(); releaseRequested.current[mode] = false; start(mode);
    }
    function keyUp(event) {
      const mode = event.code === "Space" ? "audio" : event.code === "KeyV" ? "video" : "";
      if (!mode) return;
      event.preventDefault(); stop(mode);
    }
    addEventListener("keydown", keyDown); addEventListener("keyup", keyUp);
    return () => { removeEventListener("keydown", keyDown); removeEventListener("keyup", keyUp); };
  });

  async function start(mode) {
    if (state.current || recordingMode || busy || starting.current) return;
    if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) return notify(tx("This browser does not support recording", "当前浏览器不支持录制"));
    starting.current = true;
    try {
      const stream = await navigator.mediaDevices.getUserMedia(mode === "video" ? { audio: true, video: { facingMode: "user", width: { ideal: 1280 }, height: { ideal: 720 } } } : { audio: true });
      const types = mode === "video" ? ["video/webm;codecs=vp9,opus", "video/webm;codecs=vp8,opus", "video/webm", "video/mp4"] : ["audio/webm;codecs=opus", "audio/webm", "audio/mp4"];
      const mimeType = types.find(type => MediaRecorder.isTypeSupported(type)) || "";
      const recorder = new MediaRecorder(stream, mimeType ? { mimeType } : undefined);
      const chunks = [];
      const recordingStartedAt = Date.now();
      recorder.ondataavailable = event => event.data.size && chunks.push(event.data);
      recorder.onstop = async () => {
        stream.getTracks().forEach(track => track.stop());
        setRecordingMode(""); setPreviewStream(null);
        if (Date.now() - recordingStartedAt < 800 || chunks.every(chunk => chunk.size === 0)) {
          notify(mode === "video" ? tx("Hold V while you finish the video, then release it", "请按住 V 录完视频后再松开") : tx("Hold Space while you speak, then release it", "请按住空格键说完后再松开"));
          setSeconds(0); state.current = null;
          return;
        }
        setBusy(true); setBusyMode(mode);
        const type = (recorder.mimeType || (mode === "video" ? "video/webm" : "audio/webm")).split(";")[0];
        try {
          if (mode === "video") {
            const form = new FormData();
            const clientMediaID = window.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`;
            form.append("deviceId", "elder-web"); form.append("clientMediaId", clientMediaID); form.append("capturedAt", new Date().toISOString());
            form.append("media", new Blob(chunks, { type }), `elder-update.${type.includes("mp4") ? "mp4" : "webm"}`);
            const imported = await api("/api/v1/me/media-imports", { method: "POST", body: form });
            if (!imported.updateId) await api(`/api/v1/me/media-imports/${encodeURIComponent(imported.id)}/decision`, { method: "POST", body: JSON.stringify({ visibility: "family", caption: tx("Shared a video message", "分享了一段视频留言") }) });
            notify(tx("Your video was saved to your Space and shared with the family", "视频已保存到你的 Space，并分享给家人"));
          } else {
            const form = new FormData();
            form.append("familyId", FAMILY_ID); form.append("memberId", member.id); form.append("visibility", "family");
            form.append("audio", new Blob(chunks, { type }), `elder-update.${type.includes("mp4") ? "m4a" : "webm"}`);
            const update = await api("/api/v1/updates/voice", { method: "POST", body: form });
            notify(update.source === "member_voice_processing_failed" ? tx("Your audio was saved and shared; AI organization is temporarily unavailable", "语音已保存并分享；AI 暂时未完成整理") : tx("Your audio was organized and shared with the family", "语音已整理并分享给家人"));
          }
          await onCreated();
        }
        catch (error) { notify(error.message); }
        finally { setBusy(false); setBusyMode(""); setSeconds(0); state.current = null; }
      };
      state.current = { recorder, stream, mode };
      recorder.start(1000); setSeconds(0); setPreviewStream(mode === "video" ? stream : null); setRecordingMode(mode);
      notify(mode === "video" ? tx("Recording video — release V when finished", "正在录像，松开 V 键即可结束") : tx("Recording audio — release Space when finished", "正在录音，松开空格键即可结束"));
      if (releaseRequested.current[mode]) recorder.stop();
    } catch (error) { notify(error.name === "NotAllowedError" ? (mode === "video" ? tx("Please allow camera and microphone access", "请允许浏览器使用摄像头和麦克风") : tx("Please allow microphone access", "请允许浏览器使用麦克风")) : tx("Unable to start recording", "暂时无法开始录制")); }
    finally { starting.current = false; }
  }
  function stop(mode) {
    releaseRequested.current[mode] = true;
    const recorder = state.current?.recorder;
    if (state.current?.mode === mode && recorder?.state === "recording") recorder.stop();
  }
  useEffect(() => {
    if (!recordingMode) return;
    const limit = window.setTimeout(() => stop(recordingMode), 90 * 1000);
    return () => window.clearTimeout(limit);
  }, [recordingMode]);
  function recordButton(mode, shortcut, english, chinese) {
    const active = recordingMode === mode;
    const saving = busy && busyMode === mode;
    return <button type="button" className={`hold-record-button ${mode} ${active ? "active" : ""}`} disabled={busy || Boolean(recordingMode && !active)} onPointerDown={event => { event.currentTarget.setPointerCapture?.(event.pointerId); releaseRequested.current[mode] = false; start(mode); }} onPointerUp={() => stop(mode)} onPointerCancel={() => stop(mode)} onContextMenu={event => event.preventDefault()}>
      <kbd>{shortcut}</kbd><strong>{saving ? tx("Saving and sharing…", "正在保存并分享…") : active ? `${formatDuration(seconds)} · ${tx("Release to send", "松开发送")}` : tx(english, chinese)}</strong>
    </button>;
  }
  return <section className={`elder-recorder ${recordingMode ? `recording ${recordingMode}` : ""}`}>
    {recordingMode === "video" && <video ref={preview} muted playsInline autoPlay className="elder-camera-preview" />}
    <div className="elder-record-actions">
      {recordButton("audio", "SPACE", "Hold for audio", "按住录音")}
      {recordButton("video", "V", "Hold for video", "按住录像")}
    </div>
    <p>{tx("Space records audio · V records video · release to share · maximum 90 seconds", "空格键录音 · V 键录像 · 松开后分享 · 最长 90 秒")}</p>
  </section>;
}

function DailyCard({ api, summary, onGenerated, notify }) {
  const { language, tx } = useLanguage();
  const [busy, setBusy] = useState(false);
  async function generate() {
    setBusy(true);
    try {
      const result = await api("/api/v1/daily-summaries/generate", { method: "POST", body: JSON.stringify({ familyId: FAMILY_ID, date: localDate(), language }) });
      onGenerated(result); notify(tx("Today's family daily is ready", "今天的家庭日报已经生成"));
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }
  return <section className="daily-card card"><div className="daily-sun">☀</div><p className="eyebrow">FAMILY DAILY</p><h2>{tx("Our family today", "我们家今天")}</h2>{summary ? <><p>{summary.content}</p><small>{summary.date} · {tx(`${summary.updateCount} updates`, `${summary.updateCount} 条动态`)}</small></> : <p className="muted-copy">{tx("Turn today's shared moments into a warm, concise family daily.", "把今天家人分享的片段，整理成一份温暖、简短的日报。")}</p>}<button className="secondary-button wide" disabled={busy} onClick={generate}>{busy ? tx("Organizing…", "正在整理…") : summary?.date === localDate() ? tx("Regenerate today's summary", "重新生成今日摘要") : tx("Generate today's summary", "生成今日摘要")}</button></section>;
}

function FamilyMembers({ members }) {
  const { tx } = useLanguage();
  return <section className="rail-card"><div className="rail-title"><strong>{tx("Family members", "家庭成员")}</strong><a href="#/settings">{tx("Manage", "管理")}</a></div><div className="member-row">{members.map(member => { const needsAttention = member.role === "elder" && member.needsAttention; return <div className={needsAttention ? "needs-attention" : ""} key={member.id} title={member.name}><Avatar member={member} /><span>{member.name}</span>{needsAttention && <small className="attention-badge">{tx("Attention", "需关注")}</small>}</div>; })}</div></section>;
}

function FamilyTree({ members }) {
  const { tx } = useLanguage();
  const levels = [
    { role: "elder", label: tx("Elders", "长辈"), empty: tx("Waiting for an elder to join", "等待长辈加入") },
    { role: "member", label: tx("Family", "家人"), empty: tx("Waiting for family to join", "等待家人加入") },
    { role: "child", label: tx("Children", "孩子"), empty: tx("Waiting for a child to join", "等待孩子加入") },
  ];
  return <div className="family-tree" aria-label={tx("Family member tree", "家庭成员树")}>
    {levels.map(level => {
      const levelMembers = members.filter(member => member.role === level.role);
      return <section className={`tree-generation tree-generation-${level.role}`} key={level.role} aria-label={level.label}>
        <span className="tree-generation-label">{level.label}</span>
        <div className="tree-nodes">
          {levelMembers.length ? levelMembers.map(member => <article className="tree-node" key={member.id}>
            {member.isAdmin && <span className="admin-badge">{tx("Administrator", "管理员")}</span>}
            <Avatar member={member} />
            <strong>{member.name}</strong>
            <span>{level.label} · {tx("Personal Space", "独立 Space")}</span>
          </article>) : <div className="tree-node tree-node-empty"><span>＋</span><strong>{level.empty}</strong></div>}
        </div>
      </section>;
    })}
  </div>;
}

function PrivacyCard() { const { tx } = useLanguage(); return <section className="privacy-card"><span>⌂</span><div><strong>{tx("Local first", "本地优先")}</strong><p>{tx("Member Spaces, original recordings, summaries, and history stay on the family server. Voice is sent to Gemini only for one-time processing.", "成员 Space、原始录音、摘要和历史保存在家庭服务器。语音会发送给 Gemini 做一次性整理。")}</p></div></section>; }

function Settings({ api, config, setConfig, members, currentMember, refresh, notify }) {
  const { tx } = useLanguage();
  const [form, setForm] = useState(config);
  const [newMember, setNewMember] = useState({ name: "", role: "member", isAdmin: false, color: colors[members.length % colors.length] });
  const [busy, setBusy] = useState(false);
  const canManageMembers = currentMember?.isAdmin && Boolean(config.adminToken);
  function saveConnection(event) {
    event.preventDefault();
    const next = { ...form, language: config.language, apiBase: form.apiBase.replace(/\/$/, "") };
    localStorage.setItem("fd.familyName", next.familyName); localStorage.setItem("fd.apiBase", next.apiBase);
    setConfig(next); notify(tx("Connection saved; the administrator token stays only in this tab", "连接已保存；管理员令牌只保留在当前页面会话中"));
  }
  async function addMember(event) {
    event.preventDefault(); setBusy(true);
    try {
      await api("/api/v1/members", { method: "POST", admin: true, body: JSON.stringify({ familyId: FAMILY_ID, ...newMember }) });
      setNewMember({ name: "", role: "member", isAdmin: false, color: colors[(members.length + 1) % colors.length] });
      notify(tx("Member and personal Space created", "成员和独立 Space 已创建")); refresh();
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }
  return <div className="settings-page">
    <section className="settings-section card"><div><p className="eyebrow">BROWSER CONFIG</p><h2>{tx("Connection", "连接设置")}</h2><p className="muted-copy">{tx("Your member login identifies you. Administrators enter the separate administrator token only when managing the family; it is not saved by this page.", "成员登录用于确认你的身份。管理员只在管理家庭时填写独立管理员令牌，本页面不会保存它。")}</p></div><form className="settings-form" onSubmit={saveConnection}><label>{tx("Family name", "家庭名称")}<input value={form.familyName} onChange={event => setForm({ ...form, familyName: event.target.value })} required /></label><label>{tx("Backend API address", "后端 API 地址")}<input value={form.apiBase} onChange={event => setForm({ ...form, apiBase: event.target.value })} placeholder={tx("Leave blank locally; use https://api.example.com remotely", "本地留空；远程填写 https://api.example.com")} /></label><label>{tx("Administrator token", "管理员令牌")}<input type="password" autoComplete="off" value={form.adminToken} onChange={event => setForm({ ...form, adminToken: event.target.value })} placeholder={tx("Enter when you need administrator actions", "需要管理操作时填写")} /></label><button className="primary-button">{tx("Use connection", "使用此连接")}</button></form></section>
    <section className="settings-section family-tree-section card"><div><p className="eyebrow">FAMILY TREE</p><h2>{tx("Family members", "家庭成员")}</h2><p className="muted-copy">{tx("Administrators have a dedicated badge. Both an administrator identity and administrator token are required to configure members.", "管理员会显示专属标记。只有管理员身份和管理员令牌同时就绪，才能进入成员配置。")}</p></div><div><FamilyTree members={members} />{canManageMembers ? <><MemberRoleEditor api={api} members={members} refresh={refresh} notify={notify} tx={tx} /><form className="add-member" onSubmit={addMember}><input value={newMember.name} maxLength="30" onChange={event => setNewMember({ ...newMember, name: event.target.value })} placeholder={tx("Member name", "成员称呼")} required /><select value={newMember.role} onChange={event => setNewMember({ ...newMember, role: event.target.value })}><option value="member">{tx("Family member", "普通成员")}</option><option value="elder">{tx("Elder", "老人")}</option><option value="child">{tx("Child", "孩子")}</option></select><label className="admin-option"><input type="checkbox" checked={newMember.isAdmin} onChange={event => setNewMember({ ...newMember, isAdmin: event.target.checked })} />{tx("Make administrator", "设为管理员")}</label><div className="color-picker">{colors.map(color => <button type="button" aria-label={tx(`Choose color ${color}`, `选择颜色 ${color}`)} key={color} className={newMember.color === color ? "active" : ""} style={{ background: color }} onClick={() => setNewMember({ ...newMember, color })} />)}</div><button className="primary-button" disabled={busy}>{busy ? tx("Creating…", "正在创建…") : tx("+ Add member", "+ 添加成员")}</button></form></> : <div className="admin-gate"><strong>{currentMember?.isAdmin ? tx("Administrator token still required", "还需要管理员令牌") : tx("Only administrators can configure members", "只有管理员可以配置成员")}</strong><p>{currentMember?.isAdmin ? tx("Enter the separate administrator token in Connection above.", "请在上方连接设置中填写独立的管理员令牌。") : tx("Switch to a family member with the Administrator badge to reveal member configuration.", "切换到带有“管理员”标记的家庭成员后，才会显示成员配置。")}</p></div>}</div></section>
    {canManageMembers && <MemberLoginSettings api={api} members={members} notify={notify} />}
    {currentMember && <SharePolicySettings api={api} member={currentMember} notify={notify} />}
    {currentMember && <MCPSessionSettings api={api} config={config} member={currentMember} notify={notify} />}
    <CoreJobSettings apiBase={config.apiBase} language={config.language} members={members} notify={notify} refresh={refresh} />
    <section className="future-boundary"><strong>{tx("Safety boundary", "安全边界")}</strong><p>{tx("ChatGPT OAuth and Claude Code remain limited to the selected member's own Space/context and family-visible updates. Other members' private content, arbitrary filesystem access, and surveillance footage are never exposed.", "ChatGPT OAuth 和 Claude Code 始终限制在当前成员自己的 Space/context 与全家可见动态。其他成员的私密内容、任意文件系统访问和监控录像都不会被开放。")}</p></section>
  </div>;
}

function MemberLoginSettings({ api, members, notify }) {
  const { tx } = useLanguage();
  const [memberID, setMemberID] = useState(members[0]?.id || "");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  async function save(event) {
    event.preventDefault(); setBusy(true);
    try {
      await api(`/api/v1/admin/members/${encodeURIComponent(memberID)}/login`, { admin: true, method: "PUT", body: JSON.stringify({ username, password }) });
      setPassword(""); notify(tx("Member login saved; previous web sessions were signed out", "成员登录信息已保存，旧的网页登录会话已退出"));
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }
  return <section className="settings-section card"><div><p className="eyebrow">MEMBER LOGIN</p><h2>{tx("Member usernames and passwords", "成员用户名与密码")}</h2><p className="muted-copy">{tx("Set or reset one member login at a time. Passwords are never shown again, and resetting signs that member out on the web.", "每次设置或重置一个成员账号。密码不会再次显示；重置后该成员的网页登录会话会全部退出。")}</p></div><form className="settings-form" onSubmit={save}><label>{tx("Member", "成员")}<select value={memberID} onChange={event => setMemberID(event.target.value)}>{members.map(member => <option key={member.id} value={member.id}>{member.name}</option>)}</select></label><label>{tx("Username", "用户名")}<input minLength="3" maxLength="32" pattern="[a-z0-9][a-z0-9._-]{2,31}" value={username} onChange={event => setUsername(event.target.value.toLowerCase())} required /></label><label>{tx("New password", "新密码")}<input type="password" autoComplete="new-password" minLength="10" maxLength="128" value={password} onChange={event => setPassword(event.target.value)} required /></label><button className="primary-button" disabled={busy || !memberID}>{busy ? tx("Saving…", "正在保存…") : tx("Save member login", "保存成员登录")}</button></form></section>;
}

function MCPSessionSettings({ api, config, member, notify }) {
  const { language, tx } = useLanguage();
  const [sessions, setSessions] = useState([]);
  const [serverUrl, setServerUrl] = useState("");
  const [tools, setTools] = useState([]);
  const [label, setLabel] = useState("Claude Code");
  const [credential, setCredential] = useState(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const canManage = Boolean(config.adminToken);

  async function load() {
    if (!canManage) { setSessions([]); setServerUrl(""); setTools([]); return; }
    setLoading(true);
    try {
      const result = await api(`/api/v1/admin/members/${member.id}/mcp-sessions`, { admin: true });
      setSessions(result.sessions || []); setServerUrl(result.serverUrl || ""); setTools(result.tools || []);
    } catch (error) { notify(error.message); }
    finally { setLoading(false); }
  }
  useEffect(() => { setCredential(null); load(); }, [member.id, config.adminToken, api]);

  async function create(event) {
    event.preventDefault(); setBusy(true); setCredential(null);
    try {
      const result = await api(`/api/v1/admin/members/${member.id}/mcp-sessions`, { admin: true, method: "POST", body: JSON.stringify({ label }) });
      setCredential(result); setServerUrl(result.serverUrl); setLabel("Claude Code");
      notify(tx("MCP session created. Save the token now.", "MCP 会话已创建，请立即保存令牌。"));
      await load();
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }
  async function revoke(sessionId) {
    setBusy(true);
    try {
      await api(`/api/v1/admin/members/${member.id}/mcp-sessions/${sessionId}`, { admin: true, method: "DELETE" });
      if (credential?.session?.id === sessionId) setCredential(null);
      notify(tx("MCP session revoked", "MCP 会话已撤销")); await load();
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }
  async function copy(value) {
    try { await navigator.clipboard.writeText(value); notify(tx("Copied", "已复制")); }
    catch { notify(tx("Copy failed; select the text manually", "复制失败，请手动选择文本")); }
  }

  const clientName = `family-daily-${member.id.slice(0, 8)}`;
  const claudeCommand = credential ? `claude mcp add --transport http ${clientName} ${credential.serverUrl} --header "Authorization: Bearer ${credential.accessToken}"` : "";
  const activeSessions = sessions.filter(session => !session.revokedAt && new Date(session.expiresAt) > new Date());
  return <section className="settings-section mcp-settings card">
    <div><p className="eyebrow">MEMBER MCP</p><div className="policy-member"><Avatar member={member} /><div><h2>{tx(`${member.name}'s AI connection`, `${member.name}的 AI 连接`)}</h2><span>{tx("45-day member session", "45 天成员会话")}</span></div></div><p className="muted-copy">{tx("Each client gets its own revocable session. It can read this member's Space and context files, plus family-visible updates; another member's private content stays hidden.", "每个客户端使用独立、可撤销的会话。它可以读取当前成员自己的 Space 和 context 文件，以及全家可见动态；其他成员的私密内容始终隐藏。")}</p></div>
    <div className="mcp-panel">
      <div className="mcp-setup-intro"><strong>{tx("Before connecting", "连接前确认")}</strong><p>{tx(`You are configuring ${member.name}. Create a separate session for every family member and every client; never reuse one member's credential for another.`, `当前配置的是${member.name}。每位家庭成员、每个客户端都要创建独立会话；不要把一个成员的凭证复用给另一个成员。`)}</p></div>
      {!canManage ? <div className="mcp-empty"><strong>{tx("Step 1 · Add the administrator token", "第 1 步 · 填写管理员令牌")}</strong><p>{tx("Add the separate administrator token in Connection settings above and save. Return here to create the selected member's credential; a normal member login cannot issue MCP sessions for someone else.", "请在上方连接设置中填写独立管理员令牌并保存，然后回来为当前成员创建凭证；普通成员登录不能替别人签发 MCP 会话。")}</p></div> : <>
        {tools.length > 0 && <section className="mcp-capabilities" aria-labelledby={`mcp-capabilities-${member.id}`}>
          <div className="mcp-capabilities-heading"><div><strong id={`mcp-capabilities-${member.id}`}>{tx("What this MCP can do", "这个 MCP 可以做什么")}</strong><p>{tx(`${tools.length} capabilities reported by the current Family Daily server`, `当前 Family Daily 服务器实际公布了 ${tools.length} 项能力`)}</p></div><span>{tx("LIVE SERVER", "当前服务器")}</span></div>
          <div className="mcp-capability-grid">{tools.map(tool => { const info = mcpCapabilityInfo(tool, tx); return <article className={`mcp-capability ${info.kind}`} key={tool.name}><div><span>{info.kind === "write" ? tx("WRITE", "写入") : tx("READ", "读取")}</span><code>{tool.name}</code></div><strong>{info.title}</strong><p>{info.description}</p></article>; })}</div>
          <p className="mcp-capability-note">{tx("Read access includes this member's private Space. Family Space access includes only family-visible updates. Write tools stay inside this member's context and sharing rules.", "读取范围包含当前成员自己的私密 Space；家庭 Space 只包含全家可见动态。写入工具仍受当前成员的 context 目录和分享规则限制。")}</p>
        </section>}
        <form className="mcp-create" onSubmit={create}><label>{tx("Session name", "会话名称")}<input value={label} maxLength="80" onChange={event => setLabel(event.target.value)} placeholder={tx("Claude Code on Mac", "Mac 上的 Claude Code")} required /></label><button className="primary-button" disabled={busy}>{busy ? tx("Creating…", "正在创建…") : tx("Create 45-day session", "创建 45 天会话")}</button></form>
        <div className="mcp-client-guides">
          <article className="mcp-client-guide">
            <div className="mcp-guide-heading"><span>01</span><div><strong>ChatGPT</strong><small>{tx("OAuth connection", "OAuth 连接")}</small></div></div>
            <ol>
              <li>{tx("Create a named 45-day session above and save the one-time token.", "在上方创建有名称的 45 天会话，并保存只显示一次的令牌。")}</li>
              <li>{tx("In ChatGPT, open Settings → Security and login, then enable Developer mode.", "在 ChatGPT 打开设置 → 安全与登录，然后开启开发者模式。")}</li>
              <li>{tx("Open ChatGPT Plugins, select +, and paste this server endpoint.", "打开 ChatGPT Plugins，点击 +，并粘贴下面的服务器地址。")}</li>
            </ol>
            {serverUrl && <div className="copy-row"><code>{serverUrl}</code><button type="button" onClick={() => copy(serverUrl)}>{tx("Copy endpoint", "复制地址")}</button></div>}
            <p>{tx("When Family Daily opens its authorization page, paste the saved session token. Then test in a new chat: “List the latest Family Space updates and their authors.” ChatGPT uses a one-hour access token and a rotating 60-day refresh session.", "Family Daily 打开授权页面时，粘贴保存好的会话令牌。然后在新对话中测试：“列出最近的家庭 Space 动态和作者”。ChatGPT 使用一小时访问令牌和可轮换的 60 天刷新会话。")}</p>
          </article>
          <article className="mcp-client-guide">
            <div className="mcp-guide-heading"><span>02</span><div><strong>Claude Code</strong><small>{tx("Bearer token connection", "Bearer 令牌连接")}</small></div></div>
            <ol>
              <li>{tx("Create a named 45-day session above. The ready-to-run command appears with its one-time token.", "在上方创建有名称的 45 天会话；包含一次性令牌的可运行命令随后会出现。")}</li>
              <li>{tx("Copy the generated command and run it in Terminal on the computer that uses Claude Code.", "复制生成的命令，并在使用 Claude Code 的电脑终端里运行。")}</li>
              <li>{tx("Run “claude mcp list” or open “/mcp” in Claude Code, then ask it for the latest Family Space updates.", "运行“claude mcp list”或在 Claude Code 中打开“/mcp”，再让它读取最近的家庭 Space 动态。")}</li>
            </ol>
            {!credential && <p className="mcp-guide-waiting">{tx("Create a session to reveal the command.", "创建会话后会显示命令。")}</p>}
          </article>
        </div>
        {credential && <div className="mcp-credential"><strong>{tx("Save this token now — it will not be shown again", "请立即保存令牌——之后不会再次显示")}</strong><div className="copy-row"><code>{credential.accessToken}</code><button type="button" onClick={() => copy(credential.accessToken)}>{tx("Copy token", "复制令牌")}</button></div><label>Claude Code<div className="copy-row"><code>{claudeCommand}</code><button type="button" onClick={() => copy(claudeCommand)}>{tx("Copy command", "复制命令")}</button></div></label><p>{tx("This same token can be pasted into the Family Daily OAuth consent page for ChatGPT.", "同一个令牌也可以粘贴到 Family Daily 的 ChatGPT OAuth 授权页面。")}</p></div>}
        <div className="mcp-session-list"><div className="mcp-list-title"><strong>{tx("Active sessions", "有效会话")}</strong><span>{loading ? tx("Loading…", "读取中…") : activeSessions.length}</span></div>{!loading && activeSessions.length === 0 ? <p className="mcp-none">{tx("No active client sessions", "暂无有效的客户端会话")}</p> : activeSessions.map(session => <div className="mcp-session" key={session.id}><div><strong>{session.label}</strong><span>{tx("Expires", "到期")} {new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "medium" }).format(new Date(session.expiresAt))}</span></div><button type="button" disabled={busy} onClick={() => revoke(session.id)}>{tx("Revoke", "撤销")}</button></div>)}</div>
        {serverUrl && <p className="mcp-boundary">{tx("Data boundary:", "数据边界：")} {tx("this member's own Space and", "当前成员自己的 Space 和")} <code>spaces/members/{member.id}/context</code> · {tx("family-visible updates only; no other member's private content, storage root, or shell", "仅全家可见动态；不开放其他成员私密内容、存储根目录或 Shell")}</p>}
        <details className="mcp-recovery"><summary>{tx("Connection failed? Check these items", "连接失败？检查这些项目")}</summary><ul><li>{tx("401 or login failed: the session may be expired, revoked, or belong to another member. Revoke it and create a new session for the selected member.", "401 或登录失败：会话可能已过期、被撤销，或属于其他成员。请撤销后为当前成员创建新会话。")}</li><li>{tx("ChatGPT cannot reach the endpoint: the backend must use a public HTTPS address.", "ChatGPT 无法访问地址：后端必须使用公网 HTTPS 地址。")}</li><li>{tx("Claude shows pending approval: open Claude Code in the trusted workspace and approve the server in /mcp.", "Claude 显示等待批准：在可信工作区中打开 Claude Code，并在 /mcp 中批准服务器。")}</li><li>{tx("Unexpected files: disconnect immediately and verify the selected family identity before issuing a fresh session.", "看到了不属于预期的文件：立即断开连接，确认当前家庭身份后重新签发会话。")}</li></ul></details>
      </>}
    </div>
  </section>;
}

function mcpCapabilityInfo(tool, tx) {
  const capabilities = {
    list_updates: ["read", tx("My Space timeline", "我的 Space 动态"), tx("Read this member's private and family-visible updates.", "读取当前成员自己的私密动态和全家可见动态。")],
    list_family_updates: ["read", tx("Shared Family Space", "家庭共享 Space"), tx("Read recent family-visible updates with author details; never another member's private updates.", "读取最近的全家可见动态和作者信息；不读取其他成员的私密动态。")],
    get_share_policy: ["read", tx("Personal sharing rules", "个人分享规则"), tx("Read this member's sharing mode and the prompt that guides AI sharing decisions.", "读取当前成员的分享模式，以及用于指导 AI 分享判断的 Prompt。")],
    list_context_files: ["read", tx("Context file list", "Context 文件列表"), tx("List safe files inside this member's isolated context folder.", "列出当前成员独立 context 目录中的安全文件。")],
    read_context_file: ["read", tx("Read context", "读取 Context"), tx("Read one permitted Markdown, text, or JSON context file.", "读取一个允许的 Markdown、文本或 JSON context 文件。")],
    write_context_file: ["write", tx("Save context", "保存 Context"), tx("Create or update one permitted context file, up to 512 KB.", "创建或更新一个允许的 context 文件，最大 512 KB。")],
    create_update: ["write", tx("Create a family update", "创建家庭动态"), tx("Create a private update; family sharing also requires auto mode and an AI policy check.", "创建私密动态；分享到全家还必须开启自动模式并通过 AI 规则检查。")],
  };
  const [kind, title, description] = capabilities[tool.name] || ["read", tool.name, tool.description || tx("Server-provided MCP capability", "服务器提供的 MCP 能力")];
  return { kind, title, description };
}

function SharePolicySettings({ api, member, notify }) {
  const { tx } = useLanguage();
  const [policy, setPolicy] = useState({ shareMode: "manual", sharePrompt: "" });
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let active = true;
    setLoading(true);
    api("/api/v1/me/share-policy", { headers: { "X-Member-ID": member.id } })
      .then(result => active && setPolicy({ shareMode: result.shareMode || "manual", sharePrompt: result.sharePrompt || "" }))
      .catch(error => active && notify(error.message))
      .finally(() => active && setLoading(false));
    return () => { active = false; };
  }, [api, member.id]);
  async function save(event) {
    event.preventDefault(); setBusy(true);
    try {
      const result = await api("/api/v1/me/share-policy", { method: "PUT", headers: { "X-Member-ID": member.id }, body: JSON.stringify(policy) });
      setPolicy({ shareMode: result.shareMode, sharePrompt: result.sharePrompt });
      notify(tx(`${member.name}'s sharing rules were saved`, `${member.name}的分享规则已保存`));
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }
  const example = tx(`I am ${member.name}. Everyday family life, travel, children's milestones, cooking, and hobbies may be suggested for sharing. Keep work, identity documents, accounts, medical details, finances, and exact addresses private. If a photo or text is suitable for only some relatives, name the suggested audience. When unsure, keep it private.`, `我是${member.name}。普通家庭生活、旅行、孩子成长、做饭和兴趣活动可以建议分享；工作内容、证件、账户、医疗、财务和精确地址保持私密。图片或文字只适合部分家人时，请明确建议分享对象；无法判断时先保持私密。`);
  return <section className="settings-section share-policy-section card">
    <div>
      <p className="eyebrow">PERSONAL SHARE POLICY</p>
      <div className="policy-member"><Avatar member={member} /><div><h2>{tx(`${member.name}'s sharing rules`, `${member.name}的分享规则`)}</h2><span>{tx("Signed in as", "当前登录")} · {member.role === "elder" ? tx("Elder", "老人") : tx("Family member", "家庭成员")}</span></div></div>
      <p className="muted-copy">{tx("These rules belong to your signed-in member identity and guide AI sharing suggestions for images, video, and text.", "这套规则只属于当前登录的成员身份，同时用于图片、视频和文字想法的 AI 分享判断。")}</p>
    </div>
    {loading ? <div className="policy-loading">{tx("Loading personal rules…", "正在读取个人规则…")}</div> : <form className="share-policy-form" onSubmit={save}>
      <fieldset>
        <legend>{tx("What AI may do", "AI 可以做什么")}</legend>
        <label><input type="radio" name={`share-mode-${member.id}`} checked={policy.shareMode === "manual"} onChange={() => setPolicy({ ...policy, shareMode: "manual" })} /><span><strong>{tx("Suggest only", "只给建议")}</strong><small>{tx("I always decide what gets shared", "始终由我决定是否分享")}</small></span></label>
        <label><input type="radio" name={`share-mode-${member.id}`} checked={policy.shareMode === "review"} onChange={() => setPolicy({ ...policy, shareMode: "review" })} /><span><strong>{tx("Prepare for review", "建议后审核")}</strong><small>{tx("Organize content and audience, then wait for me", "整理内容和对象，等待我确认")}</small></span></label>
        <label><input type="radio" name={`share-mode-${member.id}`} checked={policy.shareMode === "auto"} onChange={() => setPolicy({ ...policy, shareMode: "auto" })} /><span><strong>{tx("Auto-share when allowed", "符合规则时自动分享")}</strong><small>{tx("Only when safe and right for the whole family", "仅安全且适合全家时生效")}</small></span></label>
      </fieldset>
      <label className="policy-prompt"><span>{tx("What I want to share, and with whom", "我想分享什么、给谁")}</span><textarea value={policy.sharePrompt} onChange={event => setPolicy({ ...policy, sharePrompt: event.target.value })} maxLength="4000" rows="7" placeholder={tx("Describe what is safe to share, what should stay private, and which relatives may care…", "写下适合分享的内容、不想分享的内容，以及哪些家人可能会关心……")} /><small>{policy.sharePrompt.length} / 4000 · {tx("applies to images and text", "同时判断图片与文字")}</small></label>
      <div className="policy-actions"><button type="button" className="secondary-button" onClick={() => setPolicy({ ...policy, sharePrompt: example })}>{tx("Use an example", "使用身份示例")}</button><button className="primary-button" disabled={busy || (policy.shareMode === "auto" && !policy.sharePrompt.trim())}>{busy ? tx("Saving…", "正在保存…") : tx(`Save ${member.name}'s rules`, `保存${member.name}的规则`)}</button></div>
      <p className="policy-boundary">{tx("AI only makes suggestions. Sensitive or uncertain content stays private, and content suggested for selected people never becomes family-visible automatically.", "AI 只会给出建议。敏感或无法判断的内容保持私密；建议只给部分成员时，不会自动变成全家可见。")}</p>
    </form>}
  </section>;
}

function AudioButton({ path, api, notify }) {
  const { tx } = useLanguage();
  const [playing, setPlaying] = useState(false);
  async function play() {
    if (playing) return;
    setPlaying(true);
    try {
      const blob = await api(path, { raw: true });
      const url = URL.createObjectURL(blob);
      const audio = new Audio(url);
      audio.onended = () => { URL.revokeObjectURL(url); setPlaying(false); };
      audio.onerror = () => { URL.revokeObjectURL(url); setPlaying(false); notify(tx("Unable to play the original audio", "暂时无法播放原声")); };
      await audio.play();
    } catch (error) { setPlaying(false); notify(error.message); }
  }
  return <button className="audio-button" onClick={play}>{playing ? tx("▮▮ Playing", "▮▮ 正在播放") : tx("▶ Play original", "▶ 播放原声")}<i /><i /><i /><i /></button>;
}

function SectionHeading({ eyebrow, title, count }) { const { tx } = useLanguage(); return <div className="section-heading"><div><p className="eyebrow">{eyebrow}</p><h2>{title}</h2></div>{count !== undefined && <span>{count} {tx("items", "条")}</span>}</div>; }
function Avatar({ member, large = false }) { return <span className={`avatar ${large ? "large" : ""}`} style={{ background: member?.color || "#AD4C34" }}>{member?.name?.slice(0, 1) || "家"}</span>; }
function EmptyState({ icon, title, text }) { return <div className="empty-state"><span>{icon}</span><strong>{title}</strong><p>{text}</p></div>; }
function PageLoading() { const { tx } = useLanguage(); return <div className="page-loading"><i /><span>{tx("Opening family space…", "正在打开家庭空间…")}</span></div>; }
function CardSkeleton() { return <div className="card skeleton"><i /><i /><i /></div>; }
function ConnectionError({ message, onSettings }) { const { tx } = useLanguage(); return <div className="connection-error"><span>!</span><div><strong>{tx("Unable to connect to the family server", "暂时无法连接家庭服务器")}</strong><p>{message}</p></div><button onClick={onSettings}>{tx("Check settings", "检查设置")}</button></div>; }
function Onboarding({ onStart }) { const { tx } = useLanguage(); return <div className="onboarding"><span className="onboarding-mark">家</span><p className="eyebrow">WELCOME TO FAMILY DAILY</p><h2>{tx("Create your family members", "先创建你的家庭成员")}</h2><p>{tx("Everyone gets a separate local Space, then you can start sharing everyday moments.", "每个人都会获得独立的本地 Space，然后就可以开始分享日常。")}</p><button className="primary-button" onClick={onStart}>{tx("Set up the family →", "开始配置家庭 →")}</button></div>; }
function MobileNav({ route }) { const { tx } = useLanguage(); return <nav className="mobile-nav"><NavLink to="/feed" route={route} icon="⌂">{tx("Feed", "动态")}</NavLink><NavLink to="/space" route={route} icon="◫">Space</NavLink><NavLink to="/elder" route={route} icon="声">{tx("Elder", "老人")}</NavLink><NavLink to="/settings" route={route} icon="⚙">{tx("Settings", "设置")}</NavLink></nav>; }

function formatTime(value, language) { return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { month: "long", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value)); }
function formatClock(value, language) { return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { hour: "2-digit", minute: "2-digit" }).format(value); }
function formatFileTime(value, language) { return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }).format(new Date(value)); }
function formatMediaMonth(value, language) { const [year, month] = value.split("-").map(Number); return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { year: "numeric", month: "long" }).format(new Date(year, month - 1, 1)); }
function formatFullDate(value, language) { return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { month: "long", day: "numeric", weekday: "long" }).format(value); }
function formatDuration(value) { return `${String(Math.floor(value / 60)).padStart(2, "0")}:${String(value % 60).padStart(2, "0")}`; }
function localDate() { const date = new Date(); return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`; }

createRoot(document.getElementById("root")).render(<React.StrictMode><App /></React.StrictMode>);
