import React, { createContext, useContext, useEffect, useMemo, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";
import LandingPage from "./LandingPage";
import { CoreJobSettings, NotificationInbox } from "./coreJobs";

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
    token: localStorage.getItem("fd.token") || "family-daily-local",
    adminToken: localStorage.getItem("fd.adminToken") || "",
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
  const [currentMemberId, setCurrentMemberId] = useState(localStorage.getItem("fd.currentMember") || "");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const [toast, setToast] = useState("");

  const api = useMemo(() => createAPI(config), [config]);
  const language = config.language;
  const tx = useMemo(() => (english, chinese) => language === "zh" ? chinese : english, [language]);
  const currentMember = members.find(member => member.id === currentMemberId) || members[0] || null;

  useEffect(() => {
    if (route === "/") { setLoading(false); return; }
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
  }, [api, refreshKey, route]);

  useEffect(() => {
    if (!currentMemberId && members[0]) setCurrentMemberId(members[0].id);
    if (currentMemberId && !members.some(member => member.id === currentMemberId) && members[0]) setCurrentMemberId(members[0].id);
  }, [members, currentMemberId]);

  useEffect(() => {
    if (currentMemberId) localStorage.setItem("fd.currentMember", currentMemberId);
  }, [currentMemberId]);

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

  const pageProps = { api, members, currentMember, notify, refreshKey, refresh };

  if (route === "/") return <LanguageContext.Provider value={{ language, tx }}><LandingPage language={language} onLanguageChange={next => setConfig(current => ({ ...current, language: next }))} /></LanguageContext.Provider>;

  return <LanguageContext.Provider value={{ language, tx }}><div className="app-shell">
    <Sidebar route={route} familyName={config.familyName} />
    <main className="main-shell">
      <Topbar route={route} members={members} currentMember={currentMember} onMemberChange={setCurrentMemberId} language={language} onLanguageChange={next => setConfig(current => ({ ...current, language: next }))} />
      {!error && <NotificationInbox api={api} currentMember={currentMember} notify={notify} refreshKey={refreshKey} />}
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
    <MobileNav route={route} />
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

function Topbar({ route, members, currentMember, onMemberChange, language, onLanguageChange }) {
  const { tx } = useLanguage();
  const titles = { "/feed": [tx("Family feed", "家庭动态"), tx("See what everyone has been up to", "看看大家最近发生了什么")], "/space": [tx("My Space", "我的 Space"), tx("Your own private record space", "属于你的独立记录空间")], "/elder": [tx("Elder mode", "老人模式"), tx("Speak, then hear about the family's day", "说一说，听听家里的今天")], "/settings": [tx("Family settings", "家庭设置"), tx("Members and connection settings", "成员和连接配置")] };
  return <header className="topbar">
    <div><p>{titles[route][1]}</p><h1>{titles[route][0]}</h1></div>
    <div className="topbar-actions"><label className="language-switcher"><span>{tx("Language", "语言")}</span><select aria-label={tx("Language", "语言")} value={language} onChange={event => onLanguageChange(event.target.value)}><option value="en">English</option><option value="zh">中文</option></select></label>{members.length > 0 && <label className="member-switcher"><span>{tx("Viewing as", "当前身份")}</span><select value={currentMember?.id || ""} onChange={event => onMemberChange(event.target.value)}>{members.map(member => <option key={member.id} value={member.id}>{member.name}{member.isAdmin ? tx(" · Administrator", " · 管理员") : member.role === "elder" ? tx(" · Elder", " · 老人") : member.role === "child" ? tx(" · Child", " · 孩子") : ""}</option>)}</select></label>}</div>
  </header>;
}

function FamilyFeed({ api, members, currentMember, notify, refreshKey }) {
  const { language, tx } = useLanguage();
  const [updates, setUpdates] = useState([]);
  const [summary, setSummary] = useState(null);
  const [loading, setLoading] = useState(true);
  const reload = () => Promise.all([
    api("/api/v1/updates?scope=family").then(data => setUpdates(data.updates || [])),
    api(`/api/v1/daily-summaries/latest?language=${language}`).then(data => setSummary(data.summary)),
  ]).finally(() => setLoading(false));
  useEffect(() => { reload().catch(error => notify(error.message)); }, [refreshKey, api]);

  return <div className="page-grid feed-layout">
    <section>
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
  const elder = currentMember?.role === "elder" ? currentMember : members.find(member => member.role === "elder") || currentMember;
  const [summary, setSummary] = useState(null);
  const [updates, setUpdates] = useState([]);
  const reload = () => Promise.all([api(`/api/v1/daily-summaries/latest?language=${language}`).then(data => setSummary(data.summary)), api("/api/v1/updates?scope=family").then(data => setUpdates((data.updates || []).slice(0, 3)))]);
  useEffect(() => { reload().catch(error => notify(error.message)); }, [api, refreshKey]);
  if (!elder) return null;
  return <div className="elder-page">
    <section className="elder-hero">
      <p className="elder-date">{formatFullDate(new Date(), language)}</p>
      <h2>{tx(`${elder.name}, how was your day?`, `${elder.name}，今天过得怎么样？`)}</h2>
      <p>{tx("Tap the button and speak just like a normal conversation.", "按一下按钮，像平时聊天一样说就好。")}</p>
      <VoiceRecorder api={api} member={elder} notify={notify} onCreated={reload} />
    </section>
    <section className="elder-summary card">
      <div className="elder-summary-heading"><span>☀</span><div><p className="eyebrow">FAMILY DAILY</p><h2>{tx("Our family today", "我们家今天")}</h2></div></div>
      {summary ? <><p className="summary-content">{summary.content}</p><small>{tx(`Based on ${summary.updateCount} family updates`, `根据 ${summary.updateCount} 条家庭动态整理`)} · {summary.date}</small></> : <p className="muted-copy">{tx("Today's family daily has not been generated yet. Once someone shares an update, it can be generated from the family feed.", "今天的家庭日报还没有生成。家人分享近况后，可以在家庭动态页生成。")}</p>}
    </section>
    <section><SectionHeading eyebrow={tx("FAMILY MESSAGES", "家人的消息")} title={tx("Recent updates", "最近更新")} />{updates.length ? <div className="elder-update-grid">{updates.map(update => <UpdateCard key={update.id} update={update} member={members.find(item => item.id === update.memberId)} api={api} notify={notify} />)}</div> : <EmptyState icon="☀" title={tx("No new family messages", "还没有新的家庭消息")} text={tx("Updates from your family will appear here.", "家人发布的 Update 会显示在这里。")} />}</section>
  </div>;
}

function VoiceRecorder({ api, member, notify, onCreated }) {
  const { tx } = useLanguage();
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
    if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) return notify(tx("This browser does not support recording", "当前浏览器不支持录音"));
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
        try { await api("/api/v1/updates/voice", { method: "POST", body: form }); notify(tx("Your voice entry has been organized and saved", "语音已经整理并保存")); await onCreated(); }
        catch (error) { notify(error.message); }
        finally { setBusy(false); setSeconds(0); state.current = null; }
      };
      state.current = { recorder, stream };
      recorder.start(); setSeconds(0); setRecording(true); notify(tx("Recording — speak naturally", "正在录音，请自然地说话"));
    } catch (error) { notify(error.name === "NotAllowedError" ? tx("Please allow microphone access", "请允许浏览器使用麦克风") : tx("Unable to start recording", "暂时无法开始录音")); }
  }
  return <div className="recorder"><button className={`record-orb ${recording ? "recording" : ""}`} disabled={busy} onClick={toggle}><span>{busy ? "…" : recording ? "■" : "●"}</span><strong>{busy ? tx("AI is organizing", "AI 正在整理") : recording ? `${formatDuration(seconds)} · ${tx("Stop recording", "结束录音")}` : tx("Tell today's story", "说说今天的故事")}</strong></button><Visibility value={visibility} onChange={setVisibility} large /></div>;
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
  return <section className="rail-card"><div className="rail-title"><strong>{tx("Family members", "家庭成员")}</strong><a href="#/settings">{tx("Manage", "管理")}</a></div><div className="member-row">{members.map(member => <div key={member.id} title={member.name}><Avatar member={member} /><span>{member.name}</span></div>)}</div></section>;
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
    localStorage.setItem("fd.familyName", next.familyName); localStorage.setItem("fd.apiBase", next.apiBase); localStorage.setItem("fd.token", next.token); localStorage.setItem("fd.adminToken", next.adminToken || "");
    setConfig(next); notify(tx("Web settings saved in this browser", "网页配置已保存在当前浏览器"));
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
    <section className="settings-section card"><div><p className="eyebrow">BROWSER CONFIG</p><h2>{tx("Connection", "连接设置")}</h2><p className="muted-copy">{tx("Administrator and family tokens are stored separately and are used only for member configuration.", "管理员令牌与家庭令牌分开保存，只用于成员配置操作。")}</p></div><form className="settings-form" onSubmit={saveConnection}><label>{tx("Family name", "家庭名称")}<input value={form.familyName} onChange={event => setForm({ ...form, familyName: event.target.value })} required /></label><label>{tx("Backend API address", "后端 API 地址")}<input value={form.apiBase} onChange={event => setForm({ ...form, apiBase: event.target.value })} placeholder={tx("Leave blank locally; use https://api.example.com remotely", "本地留空；远程填写 https://api.example.com")} /></label><label>{tx("Family access token", "家庭访问令牌")}<input type="password" value={form.token} onChange={event => setForm({ ...form, token: event.target.value })} required /></label><label>{tx("Administrator token", "管理员令牌")}<input type="password" value={form.adminToken} onChange={event => setForm({ ...form, adminToken: event.target.value })} placeholder={tx("Required only for administrators", "仅管理员需要填写")} /></label><button className="primary-button">{tx("Save connection", "保存连接")}</button></form></section>
    <section className="settings-section family-tree-section card"><div><p className="eyebrow">FAMILY TREE</p><h2>{tx("Family members", "家庭成员")}</h2><p className="muted-copy">{tx("Administrators have a dedicated badge. Both an administrator identity and administrator token are required to configure members.", "管理员会显示专属标记。只有管理员身份和管理员令牌同时就绪，才能进入成员配置。")}</p></div><div><FamilyTree members={members} />{canManageMembers ? <form className="add-member" onSubmit={addMember}><input value={newMember.name} maxLength="30" onChange={event => setNewMember({ ...newMember, name: event.target.value })} placeholder={tx("Member name", "成员称呼")} required /><select value={newMember.role} onChange={event => setNewMember({ ...newMember, role: event.target.value })}><option value="member">{tx("Family member", "普通成员")}</option><option value="elder">{tx("Elder", "老人")}</option><option value="child">{tx("Child", "孩子")}</option></select><label className="admin-option"><input type="checkbox" checked={newMember.isAdmin} onChange={event => setNewMember({ ...newMember, isAdmin: event.target.checked })} />{tx("Make administrator", "设为管理员")}</label><div className="color-picker">{colors.map(color => <button type="button" aria-label={tx(`Choose color ${color}`, `选择颜色 ${color}`)} key={color} className={newMember.color === color ? "active" : ""} style={{ background: color }} onClick={() => setNewMember({ ...newMember, color })} />)}</div><button className="primary-button" disabled={busy}>{busy ? tx("Creating…", "正在创建…") : tx("+ Add member", "+ 添加成员")}</button></form> : <div className="admin-gate"><strong>{currentMember?.isAdmin ? tx("Administrator token still required", "还需要管理员令牌") : tx("Only administrators can configure members", "只有管理员可以配置成员")}</strong><p>{currentMember?.isAdmin ? tx("Enter the separate administrator token in Connection above.", "请在上方连接设置中填写独立的管理员令牌。") : tx("Switch to a family member with the Administrator badge to reveal member configuration.", "切换到带有“管理员”标记的家庭成员后，才会显示成员配置。")}</p></div>}</div></section>
    {currentMember && <SharePolicySettings api={api} member={currentMember} notify={notify} />}
    {currentMember && <MCPSessionSettings api={api} config={config} member={currentMember} notify={notify} />}
    <CoreJobSettings apiBase={config.apiBase} familyToken={config.token} language={config.language} members={members} notify={notify} refresh={refresh} />
    <section className="future-boundary"><strong>{tx("Safety boundary", "安全边界")}</strong><p>{tx("ChatGPT OAuth and Claude Code both remain limited to each member's context folder. Broader analysis and camera/NAS sources are separate future decisions; MCP never exposes arbitrary filesystem access or surveillance footage.", "ChatGPT OAuth 和 Claude Code 都始终限制在每位成员自己的 context 目录。更广泛的分析以及摄像头/NAS 数据源属于未来的独立决策；MCP 不开放任意文件系统访问，也不读取监控录像。")}</p></section>
  </div>;
}

function MCPSessionSettings({ api, config, member, notify }) {
  const { language, tx } = useLanguage();
  const [sessions, setSessions] = useState([]);
  const [serverUrl, setServerUrl] = useState("");
  const [label, setLabel] = useState("Claude Code");
  const [credential, setCredential] = useState(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const canManage = Boolean(config.adminToken);

  async function load() {
    if (!canManage) { setSessions([]); setServerUrl(""); return; }
    setLoading(true);
    try {
      const result = await api(`/api/v1/admin/members/${member.id}/mcp-sessions`, { admin: true });
      setSessions(result.sessions || []); setServerUrl(result.serverUrl || "");
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
    <div><p className="eyebrow">MEMBER MCP</p><div className="policy-member"><Avatar member={member} /><div><h2>{tx(`${member.name}'s AI connection`, `${member.name}的 AI 连接`)}</h2><span>{tx("45-day member session", "45 天成员会话")}</span></div></div><p className="muted-copy">{tx("Each client gets its own revocable session and can reach only this member's context files. Switch identity to manage another member.", "每个客户端使用独立、可撤销的会话，并且只能访问这个成员的 context 文件。切换身份即可管理其他成员。")}</p></div>
    <div className="mcp-panel">
      <div className="mcp-setup-intro"><strong>{tx("Before connecting", "连接前确认")}</strong><p>{tx(`You are configuring ${member.name}. Create a separate session for every family member and every client; never reuse one member's credential for another.`, `当前配置的是${member.name}。每位家庭成员、每个客户端都要创建独立会话；不要把一个成员的凭证复用给另一个成员。`)}</p></div>
      {!canManage ? <div className="mcp-empty"><strong>{tx("Step 1 · Add the administrator token", "第 1 步 · 填写管理员令牌")}</strong><p>{tx("Add the separate administrator token in Connection settings above and save. Return here to create the selected member's credential; the family access token cannot issue MCP sessions.", "请在上方连接设置中填写独立管理员令牌并保存，然后回来为当前成员创建凭证；家庭访问令牌不能签发 MCP 会话。")}</p></div> : <>
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
            <p>{tx("When Family Daily opens its authorization page, paste the saved session token. Then test in a new chat: “List my context files.” ChatGPT uses a one-hour access token and a rotating 60-day refresh session.", "Family Daily 打开授权页面时，粘贴保存好的会话令牌。然后在新对话中测试：“列出我的 context 文件”。ChatGPT 使用一小时访问令牌和可轮换的 60 天刷新会话。")}</p>
          </article>
          <article className="mcp-client-guide">
            <div className="mcp-guide-heading"><span>02</span><div><strong>Claude Code</strong><small>{tx("Bearer token connection", "Bearer 令牌连接")}</small></div></div>
            <ol>
              <li>{tx("Create a named 45-day session above. The ready-to-run command appears with its one-time token.", "在上方创建有名称的 45 天会话；包含一次性令牌的可运行命令随后会出现。")}</li>
              <li>{tx("Copy the generated command and run it in Terminal on the computer that uses Claude Code.", "复制生成的命令，并在使用 Claude Code 的电脑终端里运行。")}</li>
              <li>{tx("Run “claude mcp list” or open “/mcp” in Claude Code, then ask it to list the context files.", "运行“claude mcp list”或在 Claude Code 中打开“/mcp”，再让它列出 context 文件。")}</li>
            </ol>
            {!credential && <p className="mcp-guide-waiting">{tx("Create a session to reveal the command.", "创建会话后会显示命令。")}</p>}
          </article>
        </div>
        {credential && <div className="mcp-credential"><strong>{tx("Save this token now — it will not be shown again", "请立即保存令牌——之后不会再次显示")}</strong><div className="copy-row"><code>{credential.accessToken}</code><button type="button" onClick={() => copy(credential.accessToken)}>{tx("Copy token", "复制令牌")}</button></div><label>Claude Code<div className="copy-row"><code>{claudeCommand}</code><button type="button" onClick={() => copy(claudeCommand)}>{tx("Copy command", "复制命令")}</button></div></label><p>{tx("This same token can be pasted into the Family Daily OAuth consent page for ChatGPT.", "同一个令牌也可以粘贴到 Family Daily 的 ChatGPT OAuth 授权页面。")}</p></div>}
        <div className="mcp-session-list"><div className="mcp-list-title"><strong>{tx("Active sessions", "有效会话")}</strong><span>{loading ? tx("Loading…", "读取中…") : activeSessions.length}</span></div>{!loading && activeSessions.length === 0 ? <p className="mcp-none">{tx("No active client sessions", "暂无有效的客户端会话")}</p> : activeSessions.map(session => <div className="mcp-session" key={session.id}><div><strong>{session.label}</strong><span>{tx("Expires", "到期")} {new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "medium" }).format(new Date(session.expiresAt))}</span></div><button type="button" disabled={busy} onClick={() => revoke(session.id)}>{tx("Revoke", "撤销")}</button></div>)}</div>
        {serverUrl && <p className="mcp-boundary">{tx("Filesystem boundary:", "文件边界：")} <code>spaces/members/{member.id}/context</code> · {tx("no storage root, shell, or cross-member access", "不开放存储根目录、Shell 或跨成员访问")}</p>}
        <details className="mcp-recovery"><summary>{tx("Connection failed? Check these items", "连接失败？检查这些项目")}</summary><ul><li>{tx("401 or login failed: the session may be expired, revoked, or belong to another member. Revoke it and create a new session for the selected member.", "401 或登录失败：会话可能已过期、被撤销，或属于其他成员。请撤销后为当前成员创建新会话。")}</li><li>{tx("ChatGPT cannot reach the endpoint: the backend must use a public HTTPS address.", "ChatGPT 无法访问地址：后端必须使用公网 HTTPS 地址。")}</li><li>{tx("Claude shows pending approval: open Claude Code in the trusted workspace and approve the server in /mcp.", "Claude 显示等待批准：在可信工作区中打开 Claude Code，并在 /mcp 中批准服务器。")}</li><li>{tx("Unexpected files: disconnect immediately and verify the selected family identity before issuing a fresh session.", "看到了不属于预期的文件：立即断开连接，确认当前家庭身份后重新签发会话。")}</li></ul></details>
      </>}
    </div>
  </section>;
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
      <div className="policy-member"><Avatar member={member} /><div><h2>{tx(`${member.name}'s sharing rules`, `${member.name}的分享规则`)}</h2><span>{tx("Viewing as", "当前身份")} · {member.role === "elder" ? tx("Elder", "老人") : tx("Family member", "家庭成员")}</span></div></div>
      <p className="muted-copy">{tx("These rules belong only to this member and guide AI sharing suggestions for images, video, and text. Switch identity in the top right to configure someone else.", "这套规则只属于当前成员，同时用于图片/视频和文字想法的 AI 分享判断。切换右上角身份即可配置另一个人。")}</p>
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

function createAPI(config) {
  return async (path, options = {}) => {
    const { admin = false, ...fetchOptions } = options;
    const headers = new Headers(fetchOptions.headers || {});
    if (admin) headers.set("X-Admin-Token", config.adminToken || ""); else headers.set("X-Family-Token", config.token);
    if (fetchOptions.body && !(fetchOptions.body instanceof FormData)) headers.set("Content-Type", "application/json");
    const response = await fetch(`${config.apiBase}${path}`, { ...fetchOptions, headers });
    if (options.raw) {
      if (!response.ok) throw new Error(config.language === "zh" ? "暂时无法读取文件" : "Unable to read the file");
      return response.blob();
    }
    if (response.status === 204) return null;
    const body = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(config.language === "zh" ? (body.error || `请求失败（${response.status}）`) : `Request failed (${response.status})`);
    return body;
  };
}

function formatTime(value, language) { return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { month: "long", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value)); }
function formatFileTime(value, language) { return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }).format(new Date(value)); }
function formatMediaMonth(value, language) { const [year, month] = value.split("-").map(Number); return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { year: "numeric", month: "long" }).format(new Date(year, month - 1, 1)); }
function formatFullDate(value, language) { return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { month: "long", day: "numeric", weekday: "long" }).format(value); }
function formatDuration(value) { return `${String(Math.floor(value / 60)).padStart(2, "0")}:${String(value % 60).padStart(2, "0")}`; }
function localDate() { const date = new Date(); return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`; }

createRoot(document.getElementById("root")).render(<React.StrictMode><App /></React.StrictMode>);
