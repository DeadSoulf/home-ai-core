import type { Job } from "../api/types";
import { useUI } from "../ui/useUI";
import { jobMessage, progressPercent } from "../ui/moduleState";
export function JobProgress({job}: {job: Job}) {
  const u = useUI(); const percent = progressPercent(job);
  return <div className="job-progress">
    <div className="progress" role="progressbar" aria-label={u("progress")} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}>
      <span style={{width: `${percent}%`}} /></div>
    <span className="small">{percent.toFixed(0)}% — {u(jobMessage(job))}</span>
  </div>;
}
