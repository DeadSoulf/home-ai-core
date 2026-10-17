/** Compact navigation pictograms; labels are always displayed alongside them. */
export function NavigationIcon({path}: {path: string}) {
  const kind = path === "/" ? "home" : path.startsWith("/modules/") ? "module" : path.split("/")[1];
  return (
    <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" focusable="false"
      stroke="currentColor" fill="none" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      {kind === "home" && <><path d="M3 10.5 12 3l9 7.5"/><path d="M5 9.5V21h14V9.5M9 21v-7h6v7"/></>}
      {kind === "files" && <><path d="M3 6a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M3 10h18"/></>}
      {kind === "modules" && <><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></>}
      {kind === "module" && <><path d="m12 2 8 5v10l-8 5-8-5V7z"/><path d="m4 7 8 5 8-5M12 12v10"/></>}
      {kind === "system" && <><rect x="3" y="4" width="18" height="13" rx="2"/><path d="M8 21h8M12 17v4M7 9h5"/></>}
      {kind === "users" && <><circle cx="9" cy="8" r="3"/><path d="M3 20v-2a6 6 0 0 1 12 0v2H3zM16 5a3 3 0 0 1 0 6M17 14a5 5 0 0 1 4 5v1"/></>}
      {kind === "jobs" && <><path d="M4 4h16v16H4zM8 9h8M8 13h8M8 17h5"/></>}
      {kind === "audit" && <><path d="m12 2 8 4v6c0 6-4 9-8 10-4-1-8-4-8-10V6z"/><path d="m9 12 2 2 4-4"/></>}
    </svg>
  );
}
