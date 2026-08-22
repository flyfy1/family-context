import React, { useEffect, useMemo, useState } from "react";
import "./coreJobs.css";

const FAMILY_ID = "our-family";
const months = Array.from({ length: 12 }, (_, index) => index + 1);
const days = Array.from({ length: 31 }, (_, index) => index + 1);

function text(language, english, chinese) {
  return language === "zh" ? chinese : english;
}

async function adminRequest(apiBase, token, path, options = {}) {
  const headers = new Headers(options.headers || {});
  headers.set("X-Admin-Token", token);
  if (options.body) headers.set("Content-Type", "application/json");
  const response = await fetch(`${apiBase}${path}`, { ...options, headers });
  const body = response.status === 204 ? null : await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body?.error || `Request failed (${response.status})`);
  return body;
}

export function NotificationInbox({ api, currentMember, notify, refreshKey, language }) {
  const [notifications, setNotifications] = useState([]);

  useEffect(() => {
    if (!currentMember) return;
    let active = true;
    api("/api/v1/me/notifications")
      .then(result => active && setNotifications((result.notifications || []).filter(item => !item.readAt)))
      .catch(error => active && notify(error.message));
    return () => { active = false; };
  }, [api, currentMember?.id, refreshKey]);

  async function acknowledge(notification) {
    try {
      await api(`/api/v1/me/notifications/${notification.id}/read`, { method: "POST" });
      setNotifications(items => items.filter(item => item.id !== notification.id));
    } catch (error) { notify(error.message); }
  }

  if (!currentMember || notifications.length === 0) return null;
  return <section className="notification-inbox" aria-label="Family reminders">
    {notifications.slice(0, 3).map(notification => <article key={notification.id} className="notification-item">
      <span className="notification-icon">!</span>
      <div><strong>{language === "zh" ? (notification.title || notificationTitle(notification.type, language)) : (notification.titleEn || notificationTitle(notification.type, language))}</strong><p>{language === "zh" ? notification.message : (notification.messageEn || notification.message)}</p></div>
      <button type="button" onClick={() => acknowledge(notification)}>{text(language, "Got it", "知道了")}</button>
    </article>)}
  </section>;
}

function notificationTitle(type, language) {
  if (type === "birthday") return text(language, "Birthday reminder", "生日提醒");
  if (type === "family_activity") return text(language, "Family activity", "家庭活动");
  return text(language, "Family care reminder", "家人关怀提醒");
}

export function CoreJobSettings({ apiBase, language, members, notify, refresh }) {
  const [adminToken, setAdminToken] = useState("");
  const [selectedMemberID, setSelectedMemberID] = useState("");
  const [rules, setRules] = useState({});
  const [scheduledJobs, setScheduledJobs] = useState([]);
  const [draft, setDraft] = useState({ enabled: false, recipientMemberIds: [], inactivityHours: 24, reminderText: "" });
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const selectedMember = useMemo(() => members.find(member => member.id === selectedMemberID), [members, selectedMemberID]);

  useEffect(() => {
    if (!selectedMemberID && members.length) {
      setSelectedMemberID((members.find(member => member.role === "elder") || members[0]).id);
    }
  }, [members, selectedMemberID]);

  useEffect(() => {
    const rule = rules[selectedMemberID];
    const legacyRecipients = members.filter(member => rule?.includeTarget || member.id !== selectedMemberID).map(member => member.id);
    setDraft(rule ? { enabled: rule.enabled, recipientMemberIds: rule.recipientMemberIds?.length ? rule.recipientMemberIds : legacyRecipients, inactivityHours: rule.inactivityHours, reminderText: rule.reminderText || "" }
      : { enabled: false, recipientMemberIds: members.filter(member => member.id !== selectedMemberID).map(member => member.id), inactivityHours: 24, reminderText: "" });
  }, [selectedMemberID, rules, members]);

  async function connect() {
    if (!adminToken.trim()) return;
    setLoading(true);
    try {
      const [result, scheduled] = await Promise.all([
        adminRequest(apiBase, adminToken.trim(), `/api/v1/admin/core-job-rules?familyId=${FAMILY_ID}`),
        adminRequest(apiBase, adminToken.trim(), `/api/v1/admin/scheduled-jobs?familyId=${FAMILY_ID}`),
      ]);
      const byMember = Object.fromEntries((result.rules || []).map(rule => [rule.targetMemberId, rule]));
      setRules(byMember);
      setScheduledJobs(scheduled.jobs || []);
      notify(text(language, "Administrator access confirmed", "管理员权限已确认"));
    } catch (error) { notify(error.message); }
    finally { setLoading(false); }
  }

  async function save(event) {
    event.preventDefault();
    if (!selectedMember) return;
    setBusy(true);
    try {
      const saved = await adminRequest(apiBase, adminToken.trim(), `/api/v1/admin/core-job-rules/${encodeURIComponent(selectedMember.id)}`, {
        method: "PUT",
        body: JSON.stringify({ familyId: FAMILY_ID, ...draft, includeTarget: draft.recipientMemberIds.includes(selectedMember.id), inactivityHours: Number(draft.inactivityHours) }),
      });
      setRules(current => ({ ...current, [selectedMember.id]: saved }));
      const run = await adminRequest(apiBase, adminToken.trim(), "/api/v1/admin/core-jobs/run", { method: "POST", body: "{}" });
      refresh?.();
      notify(run.notificationsCreated > 0
        ? text(language, `${run.notificationsCreated} reminder(s) sent`, `已生成 ${run.notificationsCreated} 条家庭提醒`)
        : text(language, "Detection rule saved", "检测规则已保存"));
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }

  async function createScheduledJob(input) {
    setBusy(true);
    try {
      const saved = await adminRequest(apiBase, adminToken.trim(), "/api/v1/admin/scheduled-jobs", {
        method: "POST", body: JSON.stringify({ familyId: FAMILY_ID, enabled: true, ...input }),
      });
      setScheduledJobs(current => [saved, ...current]);
      const run = await adminRequest(apiBase, adminToken.trim(), "/api/v1/admin/core-jobs/run", { method: "POST", body: "{}" });
      refresh?.();
      notify(run.notificationsCreated > 0
        ? text(language, `${run.notificationsCreated} reminder(s) sent`, `已生成 ${run.notificationsCreated} 条家庭提醒`)
        : text(language, "Family automation saved", "家庭自动任务已保存"));
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }

  async function removeScheduledJob(job) {
    if (!window.confirm(text(language, `Delete “${job.title}”?`, `删除“${job.title}”？`))) return;
    try {
      await adminRequest(apiBase, adminToken.trim(), `/api/v1/admin/scheduled-jobs/${job.id}?familyId=${FAMILY_ID}`, { method: "DELETE" });
      setScheduledJobs(current => current.filter(item => item.id !== job.id));
      notify(text(language, "Automation deleted", "自动任务已删除"));
    } catch (error) { notify(error.message); }
  }

  async function toggleScheduledJob(job) {
    try {
      const saved = await adminRequest(apiBase, adminToken.trim(), `/api/v1/admin/scheduled-jobs/${job.id}`, {
        method: "PUT",
        body: JSON.stringify({ familyId: FAMILY_ID, ...job, enabled: !job.enabled }),
      });
      setScheduledJobs(current => current.map(item => item.id === saved.id ? saved : item));
      notify(saved.enabled ? text(language, "Automation enabled", "自动任务已开启") : text(language, "Automation paused", "自动任务已暂停"));
    } catch (error) { notify(error.message); }
  }

  return <section className="settings-section core-job-settings card">
    <div>
      <p className="eyebrow">FAMILY AUTOMATIONS</p>
      <h2>{text(language, "Family automation tasks", "家庭自动任务")}</h2>
      <p className="muted-copy">{text(language,
        "Configure care detection, annual birthdays, and one-time family tasks or activities in one place.",
        "在一个地方配置关怀检测、年度生日提醒，以及一次性的家庭任务或活动。")}</p>
    </div>
    <div className="core-job-panel">
      <div className="admin-connection">
        <label>{text(language, "Administrator token", "管理员令牌")}<input type="password" value={adminToken} onChange={event => setAdminToken(event.target.value)} /></label>
        <button type="button" className="secondary-button" disabled={loading || !adminToken.trim()} onClick={connect}>{loading ? text(language, "Checking…", "正在验证…") : text(language, "Connect", "验证权限")}</button>
      </div>
      <div className="automation-subheading"><span>1</span><div><strong>{text(language, "Care detection", "关怀检测")}</strong><small>{text(language, "Notice a long gap in a member's posts.", "发现某位成员长时间没有发布新内容。")}</small></div></div>
      <form className="core-job-form" onSubmit={save}>
        <label>{text(language, "Member to watch", "检测对象")}<select value={selectedMemberID} onChange={event => setSelectedMemberID(event.target.value)}>{members.map(member => <option key={member.id} value={member.id}>{member.name}{member.role === "elder" ? text(language, " · Elder", " · 老人") : ""}</option>)}</select></label>
        <label className="core-job-toggle"><input type="checkbox" checked={draft.enabled} onChange={event => setDraft({ ...draft, enabled: event.target.checked })} /><span><strong>{text(language, "Enable no-post detection", "开启未发帖检测")}</strong><small>{text(language, "Private posts count as activity, but their content stays private.", "私密记录也计为活跃，但其内容不会被分享。")}</small></span></label>
        <MemberCheckboxes language={language} members={members} selected={draft.recipientMemberIds} onChange={recipientMemberIds => setDraft({ ...draft, recipientMemberIds })} label={text(language, "Who should receive the reminder?", "提醒哪些成员？")} />
        <label>{text(language, "Remind after", "多久未发布后提醒")}<span className="hours-input"><input type="number" min="1" max="720" value={draft.inactivityHours} onChange={event => setDraft({ ...draft, inactivityHours: event.target.value })} /><small>{text(language, "hours", "小时")}</small></span></label>
        <label>{text(language, "Reminder message (optional)", "提醒内容（可选）")}<textarea rows="3" maxLength="300" value={draft.reminderText} onChange={event => setDraft({ ...draft, reminderText: event.target.value })} placeholder={selectedMember ? text(language, `Please check in with ${selectedMember.name}.`, `方便时请联系一下${selectedMember.name}。`) : ""} /></label>
        <button className="primary-button" disabled={busy || !adminToken.trim() || !selectedMemberID || !draft.recipientMemberIds.length}>{busy ? text(language, "Saving and checking…", "正在保存并检测…") : text(language, "Save and run detection", "保存并立即检测")}</button>
      </form>
      <div className="automation-divider" />
      <BirthdayJobForm language={language} members={members} disabled={busy || !adminToken.trim()} onCreate={createScheduledJob} />
      <div className="automation-divider" />
      <ActivityJobForm language={language} members={members} disabled={busy || !adminToken.trim()} onCreate={createScheduledJob} />
      <ScheduledJobList language={language} members={members} jobs={scheduledJobs} onDelete={removeScheduledJob} onToggle={toggleScheduledJob} />
    </div>
  </section>;
}

function BirthdayJobForm({ language, members, disabled, onCreate }) {
  const [memberID, setMemberID] = useState("");
  const [month, setMonth] = useState(1);
  const [day, setDay] = useState(1);
  const [remindDaysBefore, setRemindDaysBefore] = useState(3);
  const [recipientMemberIds, setRecipientMemberIds] = useState([]);
  const [message, setMessage] = useState("");
  useEffect(() => { if (!memberID && members[0]) setMemberID(members[0].id); }, [memberID, members]);
  useEffect(() => { setRecipientMemberIds(current => current.length ? current : members.filter(item => item.id !== memberID).map(item => item.id)); }, [members, memberID]);
  const member = members.find(item => item.id === memberID);

  function submit(event) {
    event.preventDefault();
    if (!member) return;
    onCreate({
      type: "birthday", title: text(language, `${member.name}'s birthday`, `${member.name}生日提醒`), targetMemberId: member.id,
      birthdayMonthDay: `${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")}`,
      remindDaysBefore: Number(remindDaysBefore), timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC",
      includeTarget: recipientMemberIds.includes(member.id), memberIds: recipientMemberIds, message: message.trim(),
    });
  }

  return <div className="scheduled-job-section">
    <div className="automation-subheading"><span>2</span><div><strong>{text(language, "Birthday reminder", "生日提醒")}</strong><small>{text(language, "Runs every year in this browser's family time zone.", "按照当前浏览器的家庭时区，每年提醒一次。")}</small></div></div>
    <form className="core-job-form compact-job-form" onSubmit={submit}>
      <label>{text(language, "Family member", "家庭成员")}<select value={memberID} onChange={event => setMemberID(event.target.value)}>{members.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <div className="birthday-date-fields"><label>{text(language, "Month", "月份")}<select value={month} onChange={event => setMonth(Number(event.target.value))}>{months.map(value => <option key={value} value={value}>{value}</option>)}</select></label><label>{text(language, "Day", "日期")}<select value={day} onChange={event => setDay(Number(event.target.value))}>{days.map(value => <option key={value} value={value}>{value}</option>)}</select></label></div>
      <label>{text(language, "Remind in advance", "提前提醒")}<select value={remindDaysBefore} onChange={event => setRemindDaysBefore(Number(event.target.value))}>{[0,1,3,7,14].map(value => <option key={value} value={value}>{value === 0 ? text(language, "On the birthday", "生日当天") : text(language, `${value} day(s) before`, `提前 ${value} 天`)}</option>)}</select></label>
      <MemberCheckboxes language={language} members={members} selected={recipientMemberIds} onChange={setRecipientMemberIds} label={text(language, "Who should receive the birthday reminder?", "提醒哪些成员？")} />
      <label>{text(language, "Custom message (optional)", "自定义文案（可选）")}<textarea rows="2" maxLength="300" value={message} onChange={event => setMessage(event.target.value)} /></label>
      <button className="primary-button" disabled={disabled || !memberID || !recipientMemberIds.length}>{text(language, "Add birthday reminder", "添加生日提醒")}</button>
    </form>
  </div>;
}

function ActivityJobForm({ language, members, disabled, onCreate }) {
  const [title, setTitle] = useState("");
  const [topic, setTopic] = useState("");
  const [scheduledFor, setScheduledFor] = useState(defaultLocalDateTime);
  const [message, setMessage] = useState("");
  const [memberIds, setMemberIds] = useState([]);
  useEffect(() => { setMemberIds(current => current.length ? current : members.map(member => member.id)); }, [members]);

  function submit(event) {
    event.preventDefault();
    onCreate({ type: "family_activity", title: title.trim(), topic: topic.trim(), scheduledFor: new Date(scheduledFor).toISOString(), memberIds, message: message.trim() });
  }

  return <div className="scheduled-job-section">
    <div className="automation-subheading"><span>3</span><div><strong>{text(language, "Family task or activity", "家庭任务或活动")}</strong><small>{text(language, "Create a shared Space thread for the selected members at the chosen time.", "在指定时间为勾选的成员创建一个共享 Space thread。")}</small></div></div>
    <form className="core-job-form compact-job-form" onSubmit={submit}>
      <label>{text(language, "Task name", "任务名称")}<input maxLength="80" value={title} onChange={event => setTitle(event.target.value)} placeholder={text(language, "Sunday family time", "周日家庭时光")} required /></label>
      <label>{text(language, "Interaction topic", "互动主题")}<textarea rows="2" maxLength="120" value={topic} onChange={event => setTopic(event.target.value)} placeholder={text(language, "Everyone shares a favorite old photo", "每个人分享一张最喜欢的老照片")} required /></label>
      <label>{text(language, "Send at", "提醒时间")}<input type="datetime-local" value={scheduledFor} onChange={event => setScheduledFor(event.target.value)} required /></label>
      <MemberCheckboxes language={language} members={members} selected={memberIds} onChange={setMemberIds} label={text(language, "Who can join this activity?", "哪些成员参加？")} />
      <label>{text(language, "Custom message (optional)", "自定义文案（可选）")}<textarea rows="2" maxLength="300" value={message} onChange={event => setMessage(event.target.value)} /></label>
      <button className="primary-button" disabled={disabled || !title.trim() || !topic.trim() || !scheduledFor || !memberIds.length}>{text(language, "Schedule family activity", "安排家庭活动")}</button>
    </form>
  </div>;
}

function ScheduledJobList({ language, members, jobs, onDelete, onToggle }) {
  if (!jobs.length) return null;
  return <div className="scheduled-job-list"><h3>{text(language, "Configured reminders", "已配置的提醒")}</h3>{jobs.map(job => <article key={job.id}>
    <span className={`job-kind ${job.type}`}>{job.type === "birthday" ? text(language, "Birthday", "生日") : text(language, "Activity", "活动")}</span>
    <div><strong>{job.title}</strong><small>{job.type === "birthday" ? `${job.birthdayMonthDay} · ${text(language, `${job.remindDaysBefore} day(s) before`, `提前 ${job.remindDaysBefore} 天`)}` : `${formatScheduledTime(job.scheduledFor, language)} · ${job.topic}`}{job.completedAt ? text(language, " · Completed", " · 已完成") : !job.enabled ? text(language, " · Paused", " · 已暂停") : ""}</small><small>{text(language, "Members", "成员")}：{(job.memberIds || []).map(id => members.find(member => member.id === id)?.name).filter(Boolean).join(language === "zh" ? "、" : ", ") || text(language, "Legacy family selection", "旧版家庭范围")}</small></div>
    <span className="job-actions">{!job.completedAt && <button type="button" onClick={() => onToggle(job)}>{job.enabled ? text(language, "Pause", "暂停") : text(language, "Enable", "开启")}</button>}<button type="button" onClick={() => onDelete(job)}>{text(language, "Delete", "删除")}</button></span>
  </article>)}</div>;
}

function MemberCheckboxes({ language, members, selected, onChange, label }) {
  function toggle(memberID) { onChange(selected.includes(memberID) ? selected.filter(id => id !== memberID) : [...selected, memberID]); }
  return <fieldset className="member-checkboxes"><legend>{label}</legend><div>{members.map(member => <label key={member.id}><input type="checkbox" checked={selected.includes(member.id)} onChange={() => toggle(member.id)} /><span style={{ "--member-color": member.color }}>{member.name}</span></label>)}</div><small>{text(language, `${selected.length} selected`, `已选择 ${selected.length} 人`)}</small></fieldset>;
}

function defaultLocalDateTime() {
  const value = new Date(Date.now() + 24 * 60 * 60 * 1000);
  value.setMinutes(value.getMinutes() - value.getTimezoneOffset());
  return value.toISOString().slice(0, 16);
}

function formatScheduledTime(value, language) {
  return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}
