import {useCallback, useEffect, useState} from "react";
import {api} from "../api/client";
import type {NVRVideoStorageState} from "../api/types";
import {ConfirmDialog} from "../components/ConfirmDialog";
import {Panel} from "../components/Panel";

const gb = (bytes: number) => (bytes / (1024 ** 3)).toFixed(1) + " ГБ";

export function NVRStorage({canManage, canManageStorage, onNavigate}: {
  canManage: boolean;
  canManageStorage: boolean;
  onNavigate: (path: string) => void;
}) {
  const [storage, setStorage] = useState<NVRVideoStorageState>();
  const [selected, setSelected] = useState("");
  const [working, setWorking] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const reload = useCallback(async () => {
    const result = await api.nvrVideoStorage();
    setStorage(result);
    setSelected((prev) => result.devices.some((device) => device.device === prev)
      ? prev : (result.devices.find((device) => !device.active)?.device || result.devices[0]?.device || ""));
  }, []);

  useEffect(() => {
    void reload().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : "Не удалось получить сведения о дисках NVR");
    });
  }, [reload]);

  async function activate() {
    if (working || !selected || !canManage || !canManageStorage) return;
    setConfirm(false);
    setWorking(true);
    setError("");
    setMessage("");
    try {
      const result = await api.nvrActivateVideoStorage(selected);
      if (!result.ok) throw new Error("Не удалось подключить видеодиск");
      setMessage("NVR переключён на выделенный видеодиск. Исходные файлы сохранены как резервная копия.");
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Ошибка миграции видеозаписей");
    } finally { setWorking(false); }
  }

  const canActivate = canManage && canManageStorage && !working && !storage?.assigned && !!selected;
  const chosen = storage?.devices.find((disk) => disk.device === selected);
  const active = storage?.active;

  return <Panel title="Хранилище видеозаписей NVR">
    <p className="muted">
      Для видео можно выделить отдельный физический диск или раздел.
      Сначала подготовьте его в «Файлы → Диски»: создайте раздел, назначьте
      «NVR — видеозаписи», отформатируйте и смонтируйте его.
    </p>
    <div className="button-row">
      <button type="button" className="button secondary"
        onClick={() => onNavigate("/files/storage")}>Открыть управление дисками</button>
      <button type="button" className="button secondary" disabled={working}
        onClick={() => void reload().catch((err: unknown) => setError(String(err)))}>
        Обновить список
      </button>
    </div>
    {active && <div className="nvr-storage-status">
      <strong>Выбранное видеохранилище: {active.device}</strong>
      <p>{storage?.verified ? "● Подключено к работающему контейнеру" :
        "Внимание: назначенный диск не подтверждён. Проверьте, что он смонтирован."}</p>
      <small className="mono">{active.mountpoint}</small>
    </div>}
    {storage && storage.devices.length === 0 && <p className="muted">
      Подходящих смонтированных разделов NVR не найдено.
      Поддерживаются ext4, XFS и Btrfs, смонтированные под /mnt/home-ai-core.
    </p>}
    {storage && storage.devices.length > 0 && <>
      <div className="nvr-storage-selector">
        <label>Видеодиск
          <select value={selected} disabled={working || storage.assigned}
            onChange={(event) => setSelected(event.target.value)}>
            {storage.devices.map((disk) => <option value={disk.device} key={disk.filesystem_uuid}>
              {disk.label || disk.device} — {disk.mountpoint} ({gb(disk.free_bytes)} свободно)
            </option>)}
          </select>
        </label>
      </div>
      {chosen && <p className="muted small">
        Файловая система: {chosen.filesystem}; общий размер {gb(chosen.size_bytes)};
        свободно {gb(chosen.free_bytes)}.
      </p>}
      <button type="button" className="button primary" disabled={!canActivate}
        onClick={() => setConfirm(true)}>
        {working ? "Перенос записей и проверка NVR…" : storage.assigned ? "Видеодиск уже назначен" : "Подключить диск к NVR"}
      </button>
    </>}
    {!canManageStorage && <p className="muted">
      Для изменения физического хранилища требуются права управления дисками и NVR.
    </p>}
    {working && <p role="status">
      Записи копируются на выделенный диск с сохранением оригиналов. NVR
      временно останавливается, а после подключения проверяется работоспособность.
      Не выключайте сервер и не отключайте диск.
    </p>}
    {error && <p role="alert" className="nvr-notice-error">{error}</p>}
    {message && <p role="status" className="nvr-notice-success">{message}</p>}
    {confirm && chosen && <ConfirmDialog title="Перенести видеозаписи на выбранный диск?"
      confirmLabel="Перенести и переподключить NVR" onCancel={() => setConfirm(false)}
      onConfirm={() => void activate()}>
      <p>
        Камеры и запись временно остановятся. Архив будет скопирован
        на {chosen.device} ({chosen.mountpoint}), после чего NVR будет запущен с новым
        хранилищем. Исходные записи останутся на старом месте как резервная копия.
      </p>
      <p><strong>Требуется достаточно свободного места для всего архива.</strong>
        При ошибке система попытается восстановить прежний контейнер.</p>
    </ConfirmDialog>}
    <p className="muted small">
      Простое назначение диска в разделе «Файлы» не меняет место записи
      автоматически. Переключение выполняется только после подтверждения здесь.
    </p>
  </Panel>;
}
