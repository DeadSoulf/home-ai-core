import {useCallback, useEffect, useState} from "react";
import {api} from "../api/client";
import type {StoragePurposeAssignment} from "../api/types";
import {Panel} from "./Panel";

export function NVRStoragePanel({assigned, canManageModules, canManageStorage, onRefresh}: {
  assigned: StoragePurposeAssignment[];
  canManageModules: boolean;
  canManageStorage: boolean;
  onRefresh: () => void;
}) {
  const [attached, setAttached] = useState<{attached: boolean; path: string}>();
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const reload = useCallback(async () => {
    if (!canManageModules || !canManageStorage) return;
    try { setAttached(await api.nvrStorageStatus()); }
    catch { setAttached(undefined); }
  }, [canManageModules, canManageStorage]);
  useEffect(() => { void reload(); }, [reload]);

  async function mount(device: string) {
    if (busy) return;
    setBusy(device);
    setError(""); setMessage("");
    try {
      await api.storageOperation({operation: "mount", device});
      onRefresh();
      setMessage("Раздел смонтирован. Теперь его можно подключить для записей NVR.");
    } catch (e) { setError(e instanceof Error ? e.message : String(e)); }
    finally { setBusy(""); }
  }
  async function attach(device: string) {
    if (busy) return;
    if (!window.confirm("Подключить этот раздел к записям NVR? Контейнер будет безопасно перезапущен. Если в NVR уже есть видеозаписи, операция будет заблокирована до миграции архива.")) return;
    setBusy(device); setError(""); setMessage("");
    try {
      setAttached(await api.nvrStorageAttach(device));
      onRefresh();
      setMessage("Хранилище подключено. Записи новых камер сохраняются на выбранном диске.");
    } catch (e) { setError(e instanceof Error ? e.message : String(e)); }
    finally { setBusy(""); }
  }

  return <Panel title="Хранилище видеонаблюдения NVR">
    <p className="muted">Создайте раздел в «Система → Диски → Разделы», выберите «NVR» как назначение,
      затем отформатируйте (ext4 или xfs), смонтируйте и подключите его здесь.
      Назначение диска само по себе ещё не перемещает записи.</p>
    {attached?.attached && <p role="status" className="notice">
      Активный диск NVR: <code>{attached.path}</code>
    </p>}
    {error && <p role="alert" className="nvr-notice-error">{error}</p>}
    {message && <p role="status">{message}</p>}
    {!canManageStorage || !canManageModules ? <p className="muted">
      Для управления диском нужны права управления хранилищем и модулями.
    </p> : null}
    <div className="table-wrap"><table>
      <thead><tr><th>Устройство</th><th>Файловая система</th><th>Точка монтирования</th><th>Действие</th></tr></thead>
      <tbody>
        {assigned.map((storage) => {
          const mounted = (storage.mountpoints || []).find((p) => p.startsWith("/mnt/home-ai-core/"));
          const ready = storage.present && (storage.filesystem === "ext4" || storage.filesystem === "xfs");
          const canOperate = canManageStorage && canManageModules && !busy && ready && !attached?.attached;
          return <tr key={storage.filesystem_uuid || storage.device}>
            <td className="mono">{storage.device}</td>
            <td>{storage.filesystem || "Не отформатирован"}</td>
            <td className="mono">{(storage.mountpoints || []).join(", ") || "Не смонтирован"}</td>
            <td>
              {attached?.attached ? "NVR уже подключён" : !ready ? "Требуется ext4 или xfs" :
                !mounted && !(storage.mountpoints || []).length ?
                  <button type="button" className="button secondary compact" disabled={!canOperate}
                    onClick={() => void mount(storage.device)}>{busy ? "Подключение…" : "Смонтировать"}</button> :
                  mounted ? <button type="button" className="button primary compact" disabled={!canOperate}
                    onClick={() => void attach(storage.device)}>{busy ? "Подключение…" : "Использовать для записей NVR"}</button> :
                    "Смонтирован вне /mnt/home-ai-core"}
            </td>
          </tr>;
        })}
        {!assigned.length && <tr><td colSpan={4} className="muted">Нет разделов, назначенных модулю NVR.</td></tr>}
      </tbody>
    </table></div>
    <p className="muted small">Безопасность: существующий архив не скрывается и не удаляется.
      Для назначения нового диска NVR должен использовать версию 1.1.3 или новее;
      без подтверждённого диска модуль не начнёт запись на системный раздел.</p>
  </Panel>;
}
