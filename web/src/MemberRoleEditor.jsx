import { useEffect, useState } from "react";

export default function MemberRoleEditor({ api, members, refresh, notify, tx }) {
  const [memberId, setMemberId] = useState(members[0]?.id || "");
  const [role, setRole] = useState(members[0]?.role || "member");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const selected = members.find(member => member.id === memberId) || members[0];
    setMemberId(selected?.id || "");
    setRole(selected?.role || "member");
  }, [members, memberId]);

  function chooseMember(nextId) {
    const selected = members.find(member => member.id === nextId);
    setMemberId(nextId);
    setRole(selected?.role || "member");
  }

  async function save(event) {
    event.preventDefault();
    const member = members.find(item => item.id === memberId);
    if (!member) return;
    setBusy(true);
    try {
      await api(`/api/v1/admin/members/${member.id}`, {
        admin: true,
        method: "PUT",
        body: JSON.stringify({ name: member.name, role, isAdmin: Boolean(member.isAdmin), color: member.color }),
      });
      notify(tx(`${member.name}'s role was updated`, `${member.name}的角色已更新`));
      await refresh();
    } catch (error) {
      notify(error.message);
    } finally {
      setBusy(false);
    }
  }

  if (members.length === 0) return null;
  return <form className="add-member" onSubmit={save} aria-label={tx("Edit member role", "编辑成员角色")}>
    <select aria-label={tx("Member to edit", "要编辑的成员")} value={memberId} onChange={event => chooseMember(event.target.value)}>
      {members.map(member => <option key={member.id} value={member.id}>{member.name}</option>)}
    </select>
    <select aria-label={tx("Member role", "成员角色")} value={role} onChange={event => setRole(event.target.value)}>
      <option value="member">{tx("Family member", "普通成员")}</option>
      <option value="elder">{tx("Elder", "老人")}</option>
      <option value="child">{tx("Child", "孩子")}</option>
    </select>
    <button className="primary-button" disabled={busy || !memberId}>{busy ? tx("Saving…", "正在保存…") : tx("Save member role", "保存成员角色")}</button>
  </form>;
}
