import { useEffect, useState } from "react";
import type { Job } from "../api/types";
import { useUI } from "../ui/useUI";
import { jobMessage, progressPercent } from "../ui/moduleState";

const installPhases: Record<string, string> = {
  prepare: "Подготовка Docker и проверка среды",
  download: "Скачивание Docker-образа",
  downloaded: "Образ загружен",
  build: "Сборка из проверенного исходника",
  storage: "Подготовка постоянного хранилища",
  create: "Создание контейнера",
  start: "Запуск контейнера",
  verify: "Проверка запуска контейнера",
};

function installStep(message?: string) {
  const match = /^module\.phase\.([a-z]+)(?:\|(.{1,110}))?$/.exec(message || "");
  if (!match || !installPhases[match[1]]) return undefined;
  return {title: installPhases[match[1]], detail: match[2]};
}

function knownLayerBytes(detail?: string) {
  const match = /^known:([0-9]+)\/([0-9]+) (.*)$/.exec(detail || "");
  if (!match) return undefined;
  const done = Number(match[1]);
  const total = Number(match[2]);
  if (!Number.isSafeInteger(done) || !Number.isSafeInteger(total) ||
      done < 0 || total <= 0 || done > total) return undefined;
  return {done, total, layer: match[3]};
}

function formatLayerBytes(bytes: number) {
  const units = ["Б", "КиБ", "МиБ", "ГиБ", "ТиБ"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toLocaleString("ru-RU", {maximumFractionDigits: unit === 0 ? 0 : 1})} ${units[unit]}`;
}

function elapsedLabel(job: Job, now: number) {
  const began = Date.parse(job.started_at || job.created_at);
  if (!Number.isFinite(began)) return "";
  const seconds = Math.max(0, Math.floor((now - began) / 1000));
  return Math.floor(seconds / 60) + " мин " + String(seconds % 60).padStart(2, "0") + " сек";
}

export function JobProgress({job}: {job: Job}) {
  const u = useUI();
  const percent = progressPercent(job);
  const [now, setNow] = useState(() => Date.now());
  const installing = ["module.install", "module.upgrade", "module.gpu.prepare"].includes(job.type) &&
    ["queued", "running", "cancel_requested"].includes(job.status);
  const step = installStep(job.message);
  const layerBytes = knownLayerBytes(step?.detail);
  useEffect(() => {
    if (!installing) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [installing]);

  if (installing) {
    // Percentages from Docker are per layer. The whole image and installation
    // total are unknown; an indeterminate bar is the only honest representation.
    const phase = job.status === "queued" ? "В очереди" :
      job.status === "cancel_requested" ? "Запрошена отмена" :
      step?.title || (job.message === "pulling and creating module container" ?
        "Скачивание образа или создание контейнера — уточнение недоступно в старом Core" :
        "Подготовка установки");
    const elapsed = elapsedLabel(job, now);
    return <div className="job-progress module-install-progress">
      <div className="progress module-install-progress-bar" role="progressbar"
        aria-label="Ход установки модуля" aria-valuetext={phase}>
        <span className="module-install-progress-animated" />
      </div>
      <strong className="module-install-stage">{phase}</strong>
      {layerBytes && <span className="small module-install-layer" role="status">
        Получено по известным слоям: {formatLayerBytes(layerBytes.done)} из {formatLayerBytes(layerBytes.total)}
      </span>}
      {step?.detail && <span className="small module-install-layer">
        Слой образа: {layerBytes?.layer || step.detail}
      </span>}
      {elapsed && <span className="small module-install-elapsed">Прошло: {elapsed}</span>}
      <span className="small muted">
        {layerBytes
          ? "Размер всего образа может быть больше: Docker сообщает размеры слоёв по мере загрузки."
          : step?.title === installPhases.build
            ? "Идёт сборка из Git-исходников; точный размер загрузки недоступен."
            : "Ожидаем статистику слоёв Docker. Общий процент образа пока неизвестен."}
      </span>
    </div>;
  }
  return <div className="job-progress">
    <div className="progress" role="progressbar" aria-label={u("progress")} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}>
      <span style={{width: `${percent}%`}} /></div>
    <span className="small">{percent.toFixed(0)}% — {u(jobMessage(job))}</span>
  </div>;
}
