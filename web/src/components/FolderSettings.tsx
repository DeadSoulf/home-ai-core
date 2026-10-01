import {useEffect, useState, type FormEvent} from "react";
import {api} from "../api/client";
import {useI18n} from "../i18n";
import {Panel} from "./Panel";

export function FolderSettings({folderID, users, onSaved, onClose}: {
  folderID: string;
  users: {id: string; username: string; display_name: string}[];
  onSaved: () => void;
  onClose: () => void;
}) {
  const {t} = useI18n();
  const [value, setValue] = useState<Awaited<ReturnType<typeof api.fileFolderSettings>>>();
  const [name, setName] = useState("");
  const [limit, setLimit] = useState("");
  const [hard, setHard] = useState(false);
  const [access, setAccess] = useState<{user_id: string; read: boolean; write: boolean}[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let live = true;
    setValue(undefined); setError("");
    api.fileFolderSettings(folderID).then((result) => {
      if (!live) return;
      setValue(result); setName(result.folder.name);
      setLimit(result.folder.quota_bytes ? String(result.folder.quota_bytes / 2 ** 30) : "");
      setHard(result.folder.hard_quota_bytes > 0); setAccess(result.access);
    }).catch((reason: unknown) => {if (live) setError(reason instanceof Error ? reason.message : t("requestFailed"));});
    return () => {live = false;};
  }, [folderID, t]);
  function toggle(id: string, permission: "read" | "write") {
    setAccess((items) => {
      const item = items.find((entry) => entry.user_id === id) || {user_id: id, read: false, write: false};
      const next = {...item, [permission]: !item[permission]};
      if (permission === "write" && next.write) next.read = true;
      if (permission === "read" && !next.read) next.write = false;
      return [...items.filter((entry) => entry.user_id !== id), next];
    });
  }
  async function save(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError("");
    try {
      const result = await api.updateFileFolderSettings(folderID, {
        name, quota_bytes: limit === "" ? 0 : Math.floor(Number(limit) * 2 ** 30),
        enforce_smb: hard, access,
      });
      onSaved();
      if (result.warning) setError(result.warning); else onClose();
      window.dispatchEvent(new CustomEvent("home-ai-core:user-access-changed"));
    } catch (reason) {setError(reason instanceof Error ? reason.message : t("requestFailed"));}
    finally {setBusy(false);}
  }
  return <Panel title={t("fileFolderSettings")} className="wide">
    {error && <div role="alert" className="form-error">{error}</div>}
    {!value ? <p>{t("loading")}</p> : <form className="user-form" onSubmit={save}>
      <label>{t("name")}<input value={name} onChange={(event) => setName(event.target.value)} required maxLength={128}/></label>
      <label>{t("quotaGiB")}<input type="number" min={0} step="any" value={limit} onChange={(event) => setLimit(event.target.value)} placeholder={t("quotaUnlimited")}/></label>
      <p className="muted small">{t("quotaAccountingNotice")}</p>
      <label className="user-access-inline-check"><input type="checkbox" checked={hard} onChange={(event) => setHard(event.target.checked)}/>{t("quotaEnforceSMB")}</label>
      <p className="muted small">{t("quotaHardNotice")}</p>
      <fieldset><legend>{t("userResourceAccess")}</legend>
        <p className="muted small">{t("folderGrantNotice")}</p>
        {users.map((user) => {
          const grant = access.find((item) => item.user_id === user.id);
          const owner = value.folder.owner_user_id === user.id;
          return <div className="resource-access-card" key={user.id}>
            <strong>{user.display_name || user.username}{owner ? ` · ${t("fileOwner")}` : ""}</strong>
            <div className="resource-access-permissions">
              <label><input type="checkbox" checked={owner || !!grant?.read} disabled={owner} onChange={() => toggle(user.id, "read")}/>{t("userResourceRead")}</label>
              <label><input type="checkbox" checked={owner || !!grant?.write} disabled={owner} onChange={() => toggle(user.id, "write")}/>{t("userResourceWrite")}</label>
            </div>
          </div>;
        })}
      </fieldset>
      <div className="network-actions"><button type="button" className="button secondary" onClick={onClose}>{t("cancel")}</button><button className="button primary" disabled={busy}>{busy ? t("working") : t("save")}</button></div>
    </form>}
  </Panel>;
}

