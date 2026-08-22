import React, { useEffect, useMemo, useState } from "react";
import "./coreJobs.css";

const FAMILY_ID = "our-family";

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

export function NotificationInbox({ api, currentMember, notify, refreshKey }) {
  const [notifications, setNotifications] = useState([]);

  useEffect(() => {
    if (!currentMember) return;
    let active = true;
    api(`/api/v1/notifications?memberId=${encodeURIComponent(currentMember.id)}`)
      .then(result => active && setNotifications((result.notifications || []).filter(item => !item.readAt)))
      .catch(error => active && notify(error.message));
    return () => { active = false; };
  }, [api, currentMember?.id, refreshKey]);

  async function acknowledge(notification) {
    try {
      await api(`/api/v1/notifications/${notification.id}/read`, {
        method: "POST",
        body: JSON.stringify({ memberId: currentMember.id }),
      });
      setNotifications(items => items.filter(item => item.id !== notification.id));
    } catch (error) { notify(error.message); }
  }

  if (!currentMember || notifications.length === 0) return null;
  return <section className="notification-inbox" aria-label="Family reminders">
    {notifications.slice(0, 3).map(notification => <article key={notification.id} className="notification-item">
      <span className="notification-icon">!</span>
      <div><strong>家人关怀提醒</strong><p>{notification.message}</p></div>
      <button type="button" onClick={() => acknowledge(notification)}>知道了</button>
    </article>)}
  </section>;
}

export function CoreJobSettings({ apiBase, familyToken, language, members, notify }) {
  const [adminToken, setAdminToken] = useState(() => localStorage.getItem("fd.adminToken") || familyToken || "");
  const [selectedMemberID, setSelectedMemberID] = useState("");
  const [rules, setRules] = useState({});
  const [draft, setDraft] = useState({ enabled: false, inactivityHours: 24, reminderText: "" });
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
    setDraft(rule ? { enabled: rule.enabled, inactivityHours: rule.inactivityHours, reminderText: rule.reminderText || "" }
      : { enabled: false, inactivityHours: 24, reminderText: "" });
  }, [selectedMemberID, rules]);

  async function connect() {
    if (!adminToken.trim()) return;
    setLoading(true);
    try {
      const result = await adminRequest(apiBase, adminToken.trim(), `/api/v1/admin/core-job-rules?familyId=${FAMILY_ID}`);
      const byMember = Object.fromEntries((result.rules || []).map(rule => [rule.targetMemberId, rule]));
      setRules(byMember);
      localStorage.setItem("fd.adminToken", adminToken.trim());
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
        body: JSON.stringify({ familyId: FAMILY_ID, ...draft, inactivityHours: Number(draft.inactivityHours) }),
      });
      setRules(current => ({ ...current, [selectedMember.id]: saved }));
      const run = await adminRequest(apiBase, adminToken.trim(), "/api/v1/admin/core-jobs/run", { method: "POST", body: "{}" });
      notify(run.notificationsCreated > 0
        ? text(language, `${run.notificationsCreated} reminder(s) sent`, `已生成 ${run.notificationsCreated} 条家庭提醒`)
        : text(language, "Detection rule saved", "检测规则已保存"));
    } catch (error) { notify(error.message); }
    finally { setBusy(false); }
  }

  return <section className="settings-section core-job-settings card">
    <div>
      <p className="eyebrow">CORE JOB · ANOMALY DETECTION</p>
      <h2>{text(language, "Family care detection", "家庭关怀检测")}</h2>
      <p className="muted-copy">{text(language,
        "Detect a long gap in one member's posts and remind everyone else once per quiet period.",
        "检测某位成员长时间没有发布新内容，并在每次沉默周期中只提醒其他家庭成员一次。")}</p>
    </div>
    <div className="core-job-panel">
      <div className="admin-connection">
        <label>{text(language, "Administrator token", "管理员令牌")}<input type="password" value={adminToken} onChange={event => setAdminToken(event.target.value)} /></label>
        <button type="button" className="secondary-button" disabled={loading || !adminToken.trim()} onClick={connect}>{loading ? text(language, "Checking…", "正在验证…") : text(language, "Connect", "验证权限")}</button>
      </div>
      <form className="core-job-form" onSubmit={save}>
        <label>{text(language, "Member to watch", "检测对象")}<select value={selectedMemberID} onChange={event => setSelectedMemberID(event.target.value)}>{members.map(member => <option key={member.id} value={member.id}>{member.name}{member.role === "elder" ? text(language, " · Elder", " · 老人") : ""}</option>)}</select></label>
        <label className="core-job-toggle"><input type="checkbox" checked={draft.enabled} onChange={event => setDraft({ ...draft, enabled: event.target.checked })} /><span><strong>{text(language, "Enable no-post detection", "开启未发帖检测")}</strong><small>{text(language, "Private posts count as activity, but their content stays private.", "私密记录也计为活跃，但其内容不会被分享。")}</small></span></label>
        <label>{text(language, "Remind after", "多久未发布后提醒")}<span className="hours-input"><input type="number" min="1" max="720" value={draft.inactivityHours} onChange={event => setDraft({ ...draft, inactivityHours: event.target.value })} /><small>{text(language, "hours", "小时")}</small></span></label>
        <label>{text(language, "Reminder message (optional)", "提醒内容（可选）")}<textarea rows="3" maxLength="300" value={draft.reminderText} onChange={event => setDraft({ ...draft, reminderText: event.target.value })} placeholder={selectedMember ? text(language, `Please check in with ${selectedMember.name}.`, `方便时请联系一下${selectedMember.name}。`) : ""} /></label>
        <button className="primary-button" disabled={busy || !adminToken.trim() || !selectedMemberID}>{busy ? text(language, "Saving and checking…", "正在保存并检测…") : text(language, "Save and run detection", "保存并立即检测")}</button>
      </form>
    </div>
  </section>;
}
