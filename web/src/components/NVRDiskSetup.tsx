import {useCallback, useEffect, useState} from "react";
import {api} from "../api/client";
import type {BlockNode} from "../api/types";
import {StorageBrowser} from "./StorageBrowser";

export function NVRDiskSetup({onChanged}: {onChanged: () => void}) {
  const [disks, setDisks] = useState<BlockNode[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const reload = useCallback(async () => {
    try {
      const result = await api.system();
      setDisks(result.system.block_tree);
      setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось получить список физических дисков");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void reload(); }, [reload]);

  return <section aria-label="Подготовка отдельного диска NVR">
    <p className="muted">
      Здесь можно создать отдельный раздел для видеозаписей. Выберите несистемный диск,
      создайте раздел и в поле «Использовать для» укажите «NVR — видеозаписи».
      Затем отформатируйте раздел (ext4, XFS или Btrfs) и смонтируйте его.
    </p>
    <p className="notice" role="status">
      Форматирование и создание разделов могут удалить данные на выбранном устройстве.
      Проверьте имя диска перед подтверждением. Назначение само по себе не
      переносит текущие записи NVR.
    </p>
    <div className="button-row">
      <button type="button" className="button secondary" onClick={() => void reload()}>
        Обновить список физических дисков
      </button>
    </div>
    {loading && <p role="status">Проверка дисков…</p>}
    {error && <p role="alert" className="nvr-notice-error">{error}</p>}
    {!loading && !error && <StorageBrowser devices={disks} canManage onChanged={() => {
      void reload();
      onChanged();
    }} />}
  </section>;
}
