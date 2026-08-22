import React, { useEffect, useState } from "react";
import "./activityThreads.css";

const copy = (language, english, chinese) => language === "zh" ? chinese : english;

export default function ActivityThreads({ api, members, currentMember, notify, language, refreshKey }) {
  const [threads, setThreads] = useState([]);
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState("");
  const [topic, setTopic] = useState("");
  const [memberIds, setMemberIds] = useState([]);
  const [busy, setBusy] = useState(false);

  const reload = () => api("/api/v1/me/activity-threads").then(data => setThreads(data.threads || []));
  useEffect(() => { if (currentMember) reload().catch(error => notify(error.message)); }, [api, currentMember?.id, refreshKey]);
  useEffect(() => { if (currentMember) setMemberIds(current => current.length ? current : members.map(member => member.id)); }, [currentMember?.id, members]);

  function toggleMember(id) {
    if (id === currentMember.id) return;
    setMemberIds(current => current.includes(id) ? current.filter(value => value !== id) : [...current, id]);
  }

  async function createThread(event) {
    event.preventDefault();
    setBusy(true);
    try {
      await api("/api/v1/me/activity-threads", { method: "POST", body: JSON.stringify({ title: title.trim(), topic: topic.trim(), memberIds: [...new Set([currentMember.id, ...memberIds])] }) });
      setTitle(""); setTopic(""); setCreating(false);
      await reload();
      notify(copy(language, "Activity thread created", "活动 thread 已创建"));
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }

  return <section className="activity-threads-section">
    <div className="activity-threads-heading"><div><p className="eyebrow">FAMILY ACTIVITIES</p><h2>{copy(language, "Activity threads", "家庭活动 Thread")}</h2><p>{copy(language, "Share posts, photos, and videos around one family theme.", "围绕一个家庭主题，集中分享文字、照片和视频。")}</p></div><button type="button" className="secondary-button" onClick={() => setCreating(value => !value)}>{creating ? copy(language, "Cancel", "取消") : copy(language, "New activity", "新建活动")}</button></div>
    {creating && <form className="activity-create card" onSubmit={createThread}>
      <label>{copy(language, "Activity name", "活动名称")}<input value={title} onChange={event => setTitle(event.target.value)} maxLength="80" required /></label>
      <label>{copy(language, "Theme", "活动主题")}<textarea value={topic} onChange={event => setTopic(event.target.value)} maxLength="300" rows="2" required /></label>
      <fieldset><legend>{copy(language, "Participants", "参与成员")}</legend><div>{members.map(member => <label key={member.id}><input type="checkbox" checked={memberIds.includes(member.id)} disabled={member.id === currentMember.id} onChange={() => toggleMember(member.id)} /><span>{member.name}</span></label>)}</div></fieldset>
      <button className="primary-button" disabled={busy || !title.trim() || !topic.trim()}>{busy ? copy(language, "Creating…", "正在创建…") : copy(language, "Create thread", "创建 Thread")}</button>
    </form>}
    {threads.length ? <div className="activity-thread-list">{threads.map(thread => <ActivityThreadCard key={thread.id} thread={thread} api={api} members={members} currentMember={currentMember} notify={notify} language={language} onChanged={reload} />)}</div> : <div className="activity-empty card"><span>✦</span><div><strong>{copy(language, "No activity threads yet", "还没有家庭活动")}</strong><p>{copy(language, "A scheduled activity will appear here when it starts, or create one now.", "定时活动开始时会出现在这里，也可以现在新建一个。")}</p></div></div>}
  </section>;
}

function ActivityThreadCard({ thread, api, members, currentMember, notify, language, onChanged }) {
  const [text, setText] = useState("");
  const [file, setFile] = useState(null);
  const [busy, setBusy] = useState(false);
  const participantNames = thread.memberIds.map(id => members.find(member => member.id === id)?.name).filter(Boolean);

  async function submit(event) {
    event.preventDefault();
    if (!text.trim() && !file) return;
    setBusy(true);
    try {
      if (file) {
        const form = new FormData(); form.append("text", text.trim()); form.append("media", file);
        await api(`/api/v1/me/activity-threads/${thread.id}/media`, { method: "POST", body: form });
      } else {
        await api(`/api/v1/me/activity-threads/${thread.id}/posts`, { method: "POST", body: JSON.stringify({ text: text.trim() }) });
      }
      setText(""); setFile(null); await onChanged();
      notify(copy(language, "Shared with activity participants", "已分享给活动参与成员"));
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }

  return <article className="activity-thread card">
    <header><div><span>{thread.scheduledJobId ? copy(language, "Scheduled activity", "定时活动") : copy(language, "Family activity", "家庭活动")}</span><h3>{thread.title}</h3><p>{thread.topic}</p></div><small>{participantNames.join(language === "zh" ? "、" : ", ")}</small></header>
    <div className="activity-posts">{thread.posts.length ? thread.posts.map(post => <ActivityPostItem key={post.id} post={post} api={api} language={language} notify={notify} />) : <p className="activity-first-post">{copy(language, "Be the first to share something for this activity.", "来分享这个活动的第一条内容吧。")}</p>}</div>
    <form className="activity-composer" onSubmit={submit}><textarea rows="2" maxLength="2000" value={text} onChange={event => setText(event.target.value)} placeholder={copy(language, `Share as ${currentMember.name}…`, `以${currentMember.name}的身份分享……`)} /><label className="activity-file"><input type="file" accept="image/jpeg,image/png,image/gif,image/webp,video/mp4,video/webm,video/quicktime" onChange={event => setFile(event.target.files?.[0] || null)} /><span>{file ? file.name : copy(language, "Add photo or video", "添加照片或视频")}</span></label><button className="primary-button" disabled={busy || (!text.trim() && !file)}>{busy ? copy(language, "Sharing…", "正在分享…") : copy(language, "Post", "发布")}</button></form>
  </article>;
}

function ActivityPostItem({ post, api, language, notify }) {
  return <div className="activity-post"><div><strong>{post.memberName}</strong><time>{new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(post.createdAt))}</time></div>{post.text && <p>{post.text}</p>}{post.mediaUrl && <ActivityMedia post={post} api={api} notify={notify} language={language} />}</div>;
}

function ActivityMedia({ post, api, notify, language }) {
  const [url, setUrl] = useState("");
  useEffect(() => {
    let objectURL = "", active = true;
    api(post.mediaUrl, { raw: true }).then(blob => { if (active) { objectURL = URL.createObjectURL(blob); setUrl(objectURL); } }).catch(error => notify(error.message));
    return () => { active = false; if (objectURL) URL.revokeObjectURL(objectURL); };
  }, [api, post.mediaUrl]);
  if (!url) return <div className="activity-media-loading">{copy(language, "Opening media…", "正在打开媒体…")}</div>;
  return post.type === "video" ? <video controls preload="metadata" src={url} /> : <img src={url} alt={copy(language, "Activity contribution", "活动分享图片")} />;
}
