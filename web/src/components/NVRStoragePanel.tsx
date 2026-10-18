import type {NVRStorageDisk, NVRStorageStatus} from "../api/types";
import {Panel} from "./Panel";

export function formatNVRBytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) return "—";
  const units = ["Б", "КиБ", "ГиБ", "ТиБ", "ПиБ"];
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit++;
  }
  return `${new Intl.NumberFormat("ru-RU", {
    maximumFractionDigits: unit === 0 ? 0 : 1,
  }).format(amount)} ${units[unit]}`;
}

function diskState(disk: NVRStorageDisk): string {
  switch (disk.state) {
    case "missing": return "Не обнаружен";
    case "unmounted": return "Не смонтирован";
    case "mounted": return "Смонтирован";
  }
}

function NVRDiskCard({disk}: {disk: NVRStorageDisk}) {
  const validCapacity = disk.state === "mounted" && disk.capacity_known &&
    Number.isFinite(disk.total_bytes) && (disk.total_bytes || 0) > 0 &&
    Number.isFinite(disk.free_bytes) && (disk.free_bytes || 0) >= 0 &&
    (disk.free_bytes || 0) <= (disk.total_bytes || 0);
  const total = validCapacity ? disk.total_bytes! : 0;
  const free = validCapacity ? disk.free_bytes! : 0;
  const usedPercent = validCapacity ? Math.min(100, Math.max(0, Math.round((1 - free / total) * 100))) : 0;

  return <article className="nvr-storage-device">
    <div className="nvr-storage-device-heading">
      <div>
        <strong>{disk.label || disk.device}</strong>
        <p className="mono muted small">{disk.device} · {disk.filesystem || "Файловая система неизвестна"}</p>
      </div>
      <span className="status-badge">{diskState(disk)}</span>
    </div>
    {disk.mountpoint && <p className="muted small">Точка монтирования: <span className="mono">{disk.mountpoint}</span></p>}
    {validCapacity ? <>
      <div className="nvr-storage-device-metrics">
        <div><small>Всего</small><strong>{formatNVRBytes(total)}</strong></div>
        <div><small>Свободно</small><strong>{formatNVRBytes(free)}</strong></div>
        <div><small>Занято на разделе</small><strong>{formatNVRBytes(total - free)}</strong></div>
      </div>
      <div className="nvr-storage-progress" role="progressbar"
        aria-label={"Занято на разделе " + disk.device}
        aria-valuemin={0} aria-valuemax={100} aria-valuenow={usedPercent}>
        <div className="nvr-storage-progress-value" style={{width: usedPercent + "%"}} />
      </div>
      <p className="muted small">{usedPercent}% занято на файловой системе, включая файлы вне NVR.</p>
    </> : <>
      {disk.state === "mounted"
        ? <p className="muted small">Размер свободного места пока не удалось определить. Это не означает, что диск заполнен.</p>
        : <p className="muted small">{disk.state === "missing"
          ? "Назначенный диск отсутствует или не распознан. Проверьте подключение."
          : "Смонтируйте раздел через «Файлы → Хранилище», чтобы увидеть свободное место."}</p>}
      {disk.size_bytes && disk.size_bytes > 0 &&
        <p className="muted small">Физический размер раздела: {formatNVRBytes(disk.size_bytes)}</p>}
    </>}
  </article>;
}

export function NVRStoragePanel({storage, loading, error}: {
  storage?: NVRStorageStatus;
  loading: boolean;
  error: string;
}) {
  const aggregate = storage && storage.assigned_count > 0 && storage.capacity_known &&
    Number.isFinite(storage.free_bytes) && Number.isFinite(storage.total_bytes);
  return <Panel title="Хранилище NVR">
    {loading && !storage && <p role="status">Проверяем объём видеодиска…</p>}
    {error && <p role="alert" className="nvr-notice-error">Не удалось обновить сведения о диске: {error}</p>}
    {storage && <>
      {storage.assigned_count === 0
        ? <p className="muted">Диск для NVR пока не назначен. Выберите раздел в «Файлы → Хранилище».</p>
        : <>
          <div className="nvr-storage-summary">
            <div><small>Назначено дисков</small><strong>{storage.assigned_count}</strong></div>
            <div><small>Смонтировано</small><strong>{storage.mounted_count}</strong></div>
            <div><small>Свободно на назначенных дисках</small>
              <strong>{aggregate ? formatNVRBytes(storage.free_bytes!) : "—"}</strong></div>
            <div><small>Общая ёмкость</small>
              <strong>{aggregate ? formatNVRBytes(storage.total_bytes!) : "—"}</strong></div>
          </div>
          <div className="nvr-storage-device-list">
            {storage.disks.map((disk) => <NVRDiskCard key={disk.device} disk={disk} />)}
          </div>
        </>}
      <p className="muted small nvr-storage-warning">
        <strong>Важно:</strong> это свободное место на дисках, назначенных для NVR.
        Само назначение или монтирование ещё не подтверждает, что NVR записывает видео
        именно на них. Текущий контейнер может продолжать использовать прежний каталог
        <code>/data</code>. Не удаляйте старый архив до безопасного переноса.
      </p>
    </>}
  </Panel>;
}
