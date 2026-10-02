# Changelog

## 0.1.128-dev

- Add permanent cleanup for finished AI conversations.
- Add **Delete chat / Удалить чат** for the currently selected finished conversation.
- Add **Clear finished / Очистить завершённые** to remove all finished conversations owned by the current user.
- Refuse deletion of an active conversation; it must be finished first.
- Delete conversation messages and related AI tool-action records through existing SQLite foreign-key cascades.
- Keep cleanup user-scoped and protected by the existing authenticated session/CSRF boundary.
- Audit single-chat deletion and bulk finished-chat cleanup without copying chat content into audit metadata.
- Add state, API and Web client regression coverage for ownership, active-chat protection, CSRF and cascade cleanup.
## 0.1.127-dev

- Keep the AI Agent page mounted after its first visit so the current chat does not disappear when navigating between Home-AI sections.
- Preserve the selected conversation, visible/optimistic messages, draft text, pending tool actions and in-flight model request while the AI page is hidden.
- Do not eagerly mount the AI page before the user visits it, avoiding unnecessary AI API requests for sessions that never open the agent.
- Add regression coverage for the persistent AI-page mount policy.
- Preserve the 0.1.125 durable-chat fix and 0.1.126 Finish-chat network hardening.
## 0.1.126-dev

- Harden the Web **Finish chat / Завершить чат** flow after the live `Failed to fetch` report.
- Register an explicit method-aware `POST /api/v1/ai/conversations/{conversationID}/close` route while preserving the existing authenticated/CSRF-protected close implementation.
- When a close request loses the connection, reload authoritative conversation state before displaying an error; if Core already committed `closed_at`, accept the close instead of reporting a false network failure.
- Replace raw browser `Failed to fetch` text with the localized generic request error when the server state cannot be reconciled.
- Add Web API regression coverage for the exact close path, POST method, same-origin credentials and CSRF header.
- Preserve AI public web access, server-tool approval boundaries and the 0.1.125 chat persistence fix.
## 0.1.125-dev

- Fix AI chat history accumulating duplicate/orphan user messages when model generation fails, times out, is cancelled, or the AI runtime is restarted.
- Do not persist a user chat message before the provider has successfully produced an assistant response.
- Keep the current user turn in transient model context during generation so tool calling and normal model behavior remain unchanged.
- Persist the user/assistant turn only after successful generation; failed/cancelled attempts no longer become durable chat history.
- Add regression coverage proving provider failure and runtime cancellation do not persist messages.
- Preserve the 0.1.124 public web tools, controlled server-tool approval boundary and Web chat lifecycle.
## 0.1.124-dev

- Keep the Web AI Agent **Finish chat / Завершить чат** action visible in the conversation header even when no active chat is selected; disable it only when closing is not applicable.
- Preserve the existing authenticated conversation-close API and persistent read-only chat history from 0.1.122-dev.
- Add read-only `web.search` and `web.fetch` tools to the AI Agent so a local Ollama model can retrieve current public Internet information through the existing typed Tool Registry.
- Run web tools automatically only for users with `system.read`; no new state-changing authority is granted to the model.
- Use a bounded DuckDuckGo HTML search adapter for public search results and a bounded HTTP/HTTPS fetcher for readable public content.
- Block localhost, RFC1918/private, link-local, CGNAT, documentation/benchmark and other non-public targets; restrict explicit ports to 80/443 and re-check DNS at dial time to prevent the web tool from becoming an SSRF path into HOME AI or the LAN.
- Limit redirects, response bytes and model-visible characters; allow only readable text/HTML/JSON/XML content.
- Mark fetched web content as untrusted in the agent system prompt so external pages cannot become instructions to the Home-AI agent.
- Add regression coverage for permission-filtered tool discovery, private-target rejection, public-IP classification and search-result decoding.
- Full PR CI covers Web typecheck/tests/production build, Go tests/vet/Core build/integration smoke, Linux/Windows cross-builds, Debian installer build and Windows client tests.
## 0.1.123-dev

- Give the local AI Agent controlled access to Home-AI server configuration through typed tools instead of an unrestricted root shell.
- Add native Ollama tool-calling support: Home-AI tool descriptors are sent as function JSON Schemas and Ollama `tool_calls` are mapped back to stable Core tool IDs.
- Add a bounded multi-round agent loop: permitted read tools may execute automatically and their results are returned to the model for follow-up reasoning.
- Add migration 021 with persistent per-user AI server actions and single-use approval state.
- Convert every `change` or `sensitive` model tool call into a pending server-action card; no server change executes until an authenticated user explicitly approves it.
- Re-check the user's current effective permissions at approval time and fail closed when access was revoked.
- Add approval/rejection APIs under `/api/v1/ai/conversations/{conversationID}/actions` with existing session/CSRF boundaries.
- Add Web approval cards with RU/EN labels, exact bounded action input, result/error state and secret-like field redaction.
- Add controlled server tools:
  - read network profiles and WireGuard state;
  - read storage/helper inspection;
  - save persistent DHCP/static network profiles with approval;
  - bring network links up/down with approval;
  - mount storage under `/mnt/home-ai-core` with approval;
  - unmount storage with sensitive approval.
- Reuse existing `network.read/manage` and `storage.read/manage` permissions plus the existing privileged helper validation/audit path.
- Add explicit limits for model tool rounds, automatic read calls, action input size and stored action result size.
- Add regression coverage for Ollama tool mapping, persistent action ownership/single-use behavior, automatic read-tool loops, approval-only writes and permission re-check at execution time.
- Arbitrary shell/root command execution remains intentionally unavailable.

## 0.1.122-dev

- Add **Finish chat / Завершить чат** to the Web AI Agent conversation header.
- Add migration 020 with persistent `closed_at` state for AI conversations.
- Keep finished chats in per-user history while making them read-only.
- Add `POST /api/v1/ai/conversations/{conversationID}/close` with the existing authentication/CSRF boundary.
- Reject new messages to finished conversations with explicit `ai_conversation_closed` / HTTP 409.
- Audit successful chat completion as `ai.conversation.close` without copying conversation text into audit metadata.
- Show **Finished / Завершён** in the conversation list/header and replace the composer with a read-only finished state.
- Offer **New conversation / Новый диалог** directly from a finished chat.
- Add state/service/API regression coverage for persistence, read-only history, ownership isolation and post-close message rejection.

## 0.1.121-dev

- Fix AI Agent **Restart** failing with `failed to persist module state` on real installations.
- Root cause: `0.1.120-dev` attempted to persist a transient `restarting` status, while the existing SQLite `modules.status` constraint only permits `registered / enabled / disabled / error`.
- Keep restart as a runtime-only transition and persist only the durable `enabled` state.
- Remove `restarting` from the Module Registry persistent-status contract and Web status type.
- Remove startup recovery for an impossible persisted `restarting` state.
- Add a regression test that drives the restart API through the real SQLite-backed Module Registry so schema/status mismatches cannot be hidden by fakes.
- Add registry coverage that explicitly rejects attempts to persist `restarting`.

## 0.1.120-dev

- Add runtime management for the first-party **AI Agent** directly from **Modules**.
- Add **Enable / Disable / Restart** controls for `ai.agent` in the Web Modules page.
- Add the `modules.manage` permission; only administrator-profile users may receive it and the owner role receives it through migration 019.
- Persist module runtime state in SQLite so an intentionally disabled AI Agent remains disabled after Home-AI-Core restarts.
- Normalize a newly registered AI Agent to `enabled`; interrupted `restarting` state is recovered to `enabled` during Core startup.
- Disabling the agent removes its provided capabilities, blocks chat/tools, and cancels active AI provider/tool requests without stopping Home-AI-Core or other modules.
- Restarting the agent cancels active AI requests and creates a fresh AI runtime context without restarting the Core process.
- Add `POST /api/v1/modules/{moduleID}/control` with `enable / disable / restart` operations, CSRF protection, `modules.manage` authorization and fail-closed unsupported-module handling.
- Audit successful module control operations as `module.runtime.control` and publish `module.runtime.changed` realtime events.
- Add explicit `ai_agent_disabled` API errors while keeping `/api/v1/ai/status` available to report the disabled state.
- Add state/registry/service/API regression coverage for persistent disable, capability removal, runtime cancellation/restart and permission denial.

## 0.1.119-dev

- Add the first persistent **local AI conversation** layer on top of the published AI read-tool/API foundation.
- Add migration 018 with per-user AI conversations and messages; conversation ownership is enforced in the state store.
- Add an optional local Ollama provider adapter using the standard library HTTP client, bounded response size and server-side request timeout.
- Keep the AI provider optional: Core starts normally with chat disabled when no provider/model is configured.
- Add `HOME_AI_AI_PROVIDER`, `HOME_AI_AI_ENDPOINT` and `HOME_AI_AI_MODEL` configuration plus Debian environment examples.
- Add authenticated conversation list/create/history/message APIs under `/api/v1/ai/conversations`.
- Persist user and assistant messages, derive a bounded first-message title, and cap model context/message sizes.
- Audit AI generation metadata as `ai.chat.generate` without storing prompt or response text in audit metadata.
- Add the Web **AI Agent** section with per-user conversation history, provider/model status and a compact local chat UI.
- Add RU/EN AI chat localization and responsive styling.
- Add regression coverage for migration/state isolation, provider HTTP contract, conversation service/audit redaction and API user isolation.
- Keep automatic AI tool execution disabled in this slice; existing typed tools remain available only through their explicit permission-checked API.
- Streaming responses and model-driven tool orchestration remain the next controlled AI slice.
- Preserve the Windows rendering fixes already staged in `0.1.118-dev`.

## 0.1.118-dev

- Fix live Windows rendering defects found in the 0.1.116 compact Figma dashboard screenshot.
- Clip native owner-drawn BUTTON child windows to the Figma 10px rounded geometry so the Win32 button-face background no longer leaks through as white corner artifacts.
- Clear each button client area with its real parent surface before custom painting; hero actions now blend into the hero and sidebar navigation into `#070B14`.
- Remove the unintended light border from primary actions and replace XOR `DrawFocusRect` with a deterministic rounded cyan keyboard-focus ring.
- Use dedicated hero primary/secondary button roles so the white secondary action and blue primary action render correctly on the gradient hero.
- Match the selected sidebar row more closely to the Figma alpha-composited fill/border and left-align nav labels at the specified 30px inset.
- Do not auto-focus the Overview primary action at startup, preventing a focus outline from appearing in the default dashboard view while preserving Tab keyboard navigation.
- Fix the clipped `HOME AI` sidebar wordmark with a dedicated brand font and wider text slot.
- Preserve sync, Credential Manager, background agent, tray, updater, AI Agent Foundation/read-tool API and Debian installer behavior.
- Native Windows tests, Windows cross-build, Debian installer checks and full pull-request CI pass on the code slice; a new live Windows screenshot is still required for visual acceptance.

## 0.1.117-dev

- Add the first callable **AI Agent read-tool layer** on top of the `0.1.115-dev` contracts while preserving the published compact Windows UI from `0.1.116-dev`.
- Register `core.system.status`, `core.jobs.list` and `core.modules.list` as typed read-only AI tools.
- Re-check the current actor's effective Core permissions for every tool execution and expose only permitted tools from the AI API.
- Keep recent-job AI output intentionally redacted from raw job input/result payloads while preserving status, progress and error summaries.
- Add authenticated `/api/v1/ai/status`, `/api/v1/ai/tools` and `/api/v1/ai/tools/<tool>/execute` endpoints.
- Require normal cookie-session CSRF protection for AI tool POST requests.
- Bound every AI tool execution with a server-side timeout.
- Audit successful, denied and failed AI tool executions as `ai.tool.execute` without storing raw tool input in audit metadata.
- Add regression coverage for permission-filtered tool discovery, successful execution, fail-closed denial, audit recording and job-payload redaction.
- Keep model/provider chat disabled in this slice; the next step is local provider integration and streaming Web chat.

## 0.1.116-dev

- Implement the approved Figma `HOME AI Windows Client — Compact Main Window` design in the native Win32 client.
- Reduce the native window from 1180×760 to 976×635 so its usable client area closely matches the 960×596 Figma content below the standard Windows title bar.
- Replace the light sidebar with the logo-derived `#070B14` navy surface, blue/cyan accents, selected navigation dots and the existing embedded HOME AI icon.
- Render the Overview hero as a rounded navy→blue GDI gradient with cyan outline and green synchronization status check.
- Match the Figma compact geometry for the four status cards, Recent activity, Backups and HOME AI storage panels.
- Compact Connection, Synchronization, Backups and Settings pages so all existing controls remain usable inside the smaller window.
- Split compact Overview values/hints so connection state, server host, local folder and folder count fit the smaller cards without multiline overflow.
- Keep RU/EN switching, Tab / Shift+Tab / Ctrl+Tab, sidebar arrow navigation, Credential Manager, sync profiles, queue/copy, background agent, tray, updater and Debian installer behavior unchanged.
- Reuse the existing embedded HOME AI brand icon instead of temporary Figma-export assets.
- Native Windows tests, Windows cross-build, Debian installer checks and full pull-request CI pass on the compact UI code; live Windows visual/DPI acceptance remains pending.

## 0.1.115-dev

- Start the **AI Agent Foundation** as a first-party `ai.agent` module so agent contracts can evolve together with NAS, Smart Home and NVR.
- Register the built-in AI Agent module at Core startup and expose `ai.agent` / `ai.tools` capabilities through the existing module registry.
- Add a typed AI Tool Registry with stable tool/module IDs, JSON input metadata, required Core permissions, optional exact resource scopes and `read / change / sensitive` side-effect classes.
- Re-check effective permissions immediately before every tool execution; scoped tools use the existing exact resource-grant contract.
- Require explicit approval before any `change` or `sensitive` tool can execute.
- Add a model-provider interface plus deterministic provider for contract/regression testing without introducing a cloud or model-vendor dependency.
- Add tests for module-manifest validation, authorized/denied/scoped tool execution, approval enforcement and provider cancellation.
- Add [AI Agent Foundation](docs/AI_AGENT_FOUNDATION.md) and ADR-0036; keep generic shell/root access explicitly out of the agent contract.
- Record successful live standalone Storage Preflight acceptance for `0.1.114-dev`.

## 0.1.114-dev

- Add a standalone **Safety check / Проверить безопасность** action to the physical-disk page.
- Add a dedicated `preflight` API mode that is forcibly mapped server-side to the existing destructive storage planner with `dry_run=true`.
- Make the standalone preflight path incapable of formatting, deleting partitions or mutating the disk even when the client omits the dry-run flag.
- Reuse existing system-disk, Home-AI pool usage, mount, active swap and cross-disk LVM safety checks and return the resulting read-only plan to the UI.
- Record standalone checks as `storage.preflight.plan` audit events.
- Add RU/EN copy explaining that the check is read-only and makes no disk changes.
- Add regression coverage proving the preflight API mode always forces dry-run.
- Keep the existing automatic preflight before format/create/delete operations unchanged.

## 0.1.113-dev

- Fix the live-accepted Windows dashboard after the 0.1.111 screenshot exposed mixed Russian/English labels, oversized card copy, classic heavy buttons and a clipped status check.
- Restore the complete Russian dashboard dictionary and add an RU/EN translation-key parity regression test so dashboard labels cannot silently fall back to English again.
- Replace the Overview and sidebar buttons with owner-drawn rounded controls, keep keyboard focus rendering, and preserve Tab / Shift+Tab / Ctrl+Tab plus arrow-key sidebar navigation.
- Draw the green synchronization status circle and white check directly with GDI so no child control covers or clips the indicator.
- Reduce dashboard typography, shorten card values, compact the displayed server host, hide the technical Status/Ready strip on Overview and show the empty recent-activity state only once.
- Silently validate the saved HOME AI connection on startup through Credential Manager; successful refresh populates server folder usage and changes the connection card from configured to connected without requiring an extra click.
- Distinguish an unconfirmed connection from a connected server that cannot report folder usage.
- Preserve the 0.1.112 TexNik attribution and all existing sync, queue/copy, background-agent, updater and Debian-installer behavior.
- Native Windows tests, Windows cross-build, Debian installer checks and the full pull-request CI pass on the current main base; real visual/DPI/tray acceptance of this polish remains pending.

## 0.1.112-dev

- Add project-wide copyright/author attribution for **TexNik**.
- Add a root `NOTICE` file and align the Apache-2.0 copyright notice with `Copyright 2026 TexNik`.
- Add `TexNik` author/license metadata to the Web package and show `© 2026 TexNik` in the authenticated Web shell and authentication screens.
- Show the same copyright attribution in the native Windows client sidebar.
- Add Debian copyright metadata and ship it as `/usr/share/doc/home-ai-core/copyright` in initial installer packages.
- Preserve the existing Apache-2.0 licensing terms and all runtime behavior.

## 0.1.111-dev

- Implement the approved HOME AI Windows dashboard design with a Windows 11-style light layout, left navigation, rounded status/cards, recent synchronization activity, backup status and HOME AI storage usage.
- Add a compact HOME AI tray status popup on normal left click while keeping the right-click context menu and existing single-agent synchronization engine.
- Preserve the existing keyboard navigation, Credential Manager authentication, sync profiles, queue/copy engine, background agent, Explorer folder access and client update path.
- Extend Windows folder metadata handling with optional used/quota values so the dashboard can show storage usage when the server provides it.
- Add native Windows coverage for the visual resources, dashboard formatting and tray-popup snapshot, alongside the existing Windows client test suite and cross-build.
- Build and inspect the initial Debian installer for amd64 and arm64 in normal CI.
- Build version-matched Debian initial installers on every VERSION release and publish each `.deb` plus its SHA-256 alongside Core update bundles and the Windows client.
- Keep the existing Core update checksum signing step intact; Debian-installer publication does not weaken the 0.1.110-dev Ed25519 update trust path.
- Real Windows visual/tray acceptance and installation of the published Debian packages on a physical Debian node remain live acceptance steps.

## 0.1.110-dev

- Add Apache-2.0 licensing for the public repository.
- Add detached Ed25519 authentication for Core update checksum metadata; non-development releases require a signature and fail closed when it is absent.
- Add release-workflow signing from the `HOME_AI_UPDATE_SIGNING_KEY` GitHub Actions secret without storing the private key in the repository or on Home-AI nodes.
- Document the trusted public-key path, signing-key provisioning and rotation contract.
- Align generated update manifests with updater-helper protocol v5 instead of the stale protocol v2 value.
- Add read-only storage dry-run/preflight support through the Core/helper protocol.
- Run dry-run preflight before Web format, partition create/delete and delete-all operations; preflight checks system-disk protection, mounts, active swap, cross-disk LVM, filesystem tools and partition capacity without mutating the host.
- Close obsolete PRs #34, #55 and #67 after confirming their functionality is superseded by newer implementations already on `main`.

## 0.1.109-dev

- Start the explicit **Core 1.0 readiness** hardening milestone without expanding the product scope.
- Add negative update-bundle regression tests for tampered payloads, unexpected archive files and path traversal, alongside a valid-bundle acceptance case.
- Add a single readiness checklist separating automated engineering gates from live rollback/network/WireGuard checks that require an installed server.
- Keep live reboot/rollback checks and stable-channel signing open; this release does not claim them as passed.

## 0.1.108-dev

- Fix Files folder **Used / limit** staying unavailable even when pool capacity is already known.
- Keep the existing in-process logical usage calculation as the preferred path.
- When Core cannot read a managed folder from its service sandbox, fall back to the privileged helper and calculate the folder's apparent file size with host-namespace `du -B1`.
- Exclude active Home-AI upload temporary files from the host `du` fallback so incomplete upload parts are not counted as normal folder contents.
- Reuse the same folder-usage fallback for folder quota checks and private-user quota totals.
- Bump the storage-helper protocol for the new read-only folder-usage operation.
- Add regression coverage for host `du` parsing and the API fallback path.

## 0.1.107-dev

- Rework the native Windows client into a lighter cloud-drive-style interface with **Overview**, **Connection**, **Synchronization**, **Backups** and **Settings** sections.
- Fix keyboard navigation by routing the Win32 message loop through dialog navigation, so **Tab / Shift+Tab** move between controls; add **Ctrl+Tab / Ctrl+Shift+Tab** for section switching and keep arrow-key navigation for the sidebar radio group.
- Reuse the existing HOME AI brand icon in the main window and system tray instead of the generic Windows application icon.
- Simplify the tray menu, allow opening it with a normal left click, and add direct access to the primary synchronized local folder in Windows Explorer.
- Preserve the existing sync-profile format, Credential Manager authentication, background agent, manual sync, queue/copy engine and update path.
- Keep the current scheduler interval-based in this slice; exact clock/day scheduling and Explorer virtual-drive integration are not claimed yet.
- Native Windows tests and the full pull-request CI cover the code slice; real Windows visual/keyboard/tray acceptance remains pending.

## 0.1.106-dev

- Replace indirect Files capacity discovery with host-level `df -B1` for mounted filesystems.
- The privileged helper resolves the real mountpoint with host `findmnt`, then returns exact total and available bytes from the mounted filesystem.
- Extend filesystem inspection with `total_bytes`, `total_known` and the resolved mountpoint.
- Keep `lsblk SIZE/FSAVAIL` and offline filesystem inspection only as fallback paths.
- Invalidate the 30-second storage inspection cache immediately after successful mount, unmount, format or other storage mutations.
- Prefer helper-reported mounted filesystem totals/free space when calculating Files pool capacity and reserve thresholds.

## 0.1.105-dev

- Fix Files storage capacity inspection after host-namespace mounting.
- Run the helper's `lsblk PATH,TYPE,FSTYPE,FSAVAIL,MOUNTPOINTS` inspection in PID 1's host mount namespace, matching the namespace used for mount/umount/findmnt.
- This lets `FSAVAIL` observe the real mounted `/mnt/home-ai-core/...` filesystem instead of the helper service's sandbox view.
- Keep offline filesystem free-space inspection as a fallback when `FSAVAIL` is unavailable.

## 0.1.104-dev

- Replace ambiguous dashes in Files storage capacity fields with an explicit unavailable-capacity label.
- Russian UI now shows «Объём недоступен» when pool or assigned-storage capacity is unknown.
- Keep known capacity values unchanged as Free / Total for pools and free-space values for assigned storage.

## 0.1.103-dev

- Fix empty “Free / Total” capacity for Files storage pools when the unprivileged core process cannot read capacity with statfs.
- Fall back to the pool's saved backing device and filesystem UUID, using block-device size plus privileged storage inspection for free space.
- Keep statfs as the preferred live capacity source when it is available.
- Use the same fallback for file-pool reserve enforcement so uploads are not blocked solely because statfs is unavailable.
- Preserve reserve and warning state calculations with the recovered capacity values.

## 0.1.102-dev

- Fix the root cause of storage mounts being visible to the privileged updater helper but not to Home-AI-Core.
- Execute mount, unmount and findmnt operations in PID 1's host mount namespace with `nsenter` instead of inside the helper service's systemd sandbox.
- Keep the updater helper's existing systemd hardening while allowing storage mounts to become host mounts that propagate into the Home-AI-Core service namespace.
- Preserve quota-enabled mount attempts, plain-mount fallback, mount verification and filesystem diagnostics.
- Add regression tests for host mount-namespace command construction.

## 0.1.101-dev

- Fix file-pool creation when the Files storage mount is real but the previous stat-based device check cannot correlate it with the assigned block device.
- Read Linux `/proc/self/mountinfo` to verify that the selected pool root is an exact mount point and obtain its kernel-reported major:minor device ID.
- Compare that mount ID with the assigned block device using Linux device-number helpers, while retaining the existing `lsblk` fast path.
- Decode escaped mount-point paths from mountinfo so spaces and special characters do not break validation.
- Keep the requirement that the storage device is explicitly assigned to Files.

## 0.1.100-dev

- Fix file-pool creation immediately after mounting storage when `lsblk` has not yet refreshed its MOUNTPOINTS field.
- Validate the selected pool root directly against the mounted filesystem device ID and its assigned block device, while still accepting the normal `lsblk` mountpoint path.
- Require the selected root to be the actual mount boundary, not an arbitrary subdirectory on the filesystem.
- Keep the requirement that backing storage is explicitly assigned to Files.
- Localize the file-pool backing-storage validation error in the Russian Web UI.

## 0.1.99-dev

- Make storage mounting idempotent: mounting a device that is already mounted at the requested Home-AI path now returns success instead of `device is already mounted`.
- Wait briefly for `findmnt` to observe a successful mount before reporting a verification failure, avoiding false errors immediately after the kernel mounts the filesystem.
- Return the canonical mountpoint from the storage operation API and reflect it immediately in Files → Storage, including the pool selector.
- Keep the mounted path visible even if the first storage inventory refresh lags behind the successful mount response.
- Localize common Files storage mount errors for the Russian Web UI instead of exposing raw English helper messages as the primary error.

## 0.1.98-dev

- Add read-only filesystem diagnostics when both quota-enabled and plain storage mounts fail.
- Diagnose ext2/ext3/ext4 with `e2fsck -n`, XFS with `xfs_repair -n`, and FAT with `fsck.fat -n`.
- Append the filesystem-check result to the mount error so bad superblocks, journal/inode damage and unsupported filesystem features are visible from the Web UI.
- Keep diagnostics non-destructive: failed mounts never trigger an automatic filesystem repair or reformat.
- Bound and normalize diagnostic output so storage errors remain readable in the UI.

## 0.1.97-dev

- Add a direct Mount action to Files → Storage for assigned filesystems that are present but not mounted.
- Mount file storage into the managed /mnt/home-ai-core path without requiring a detour through System → Storage.
- Automatically select the newly mounted filesystem in the Create storage pool form.
- Retry ext4/XFS mounting without quota mount options when the host kernel or an existing filesystem rejects those options, while keeping the first quota-enabled attempt.
- Report both quota-enabled and plain-mount failures when neither attempt succeeds, making filesystem problems easier to distinguish from mount-option compatibility.
- Keep the storage selector limited to mounted, managed filesystems so pool roots always reference usable storage.

## 0.1.96-dev

- Fix mojibake characters in the Files UI where UTF-8 em dashes and middle dots were rendered as `вЂ—` / `В·`.
- Restore readable placeholders and separators across folders, Windows access and storage tables.

## 0.1.95-dev

- Fix a blank Files → Storage page when an assigned storage device has no active mountpoints.
- Keep the storage-purpose API contract stable by serializing missing mountpoints as an empty array instead of null.
- Make the Web storage view tolerate legacy/null mountpoint responses so older persisted device states cannot crash React rendering.
- Preserve the existing pool, assigned-storage and pool-creation controls for unmounted storage.

## 0.1.94-dev

- Make update-bundle downloads resilient to transient GitHub/CDN failures such as HTTP 502, 503 and 504, plus HTTP 408 and 429.
- Retry transient bundle-download failures with bounded backoff instead of failing the update after the first gateway error.
- Use a dedicated two-minute HTTP timeout for release bundles while retaining the existing shorter timeout for update metadata.
- Preserve strict bundle size and SHA-256 verification after retries.
- Add updater regression tests covering successful recovery from repeated HTTP 504 responses and no retry for permanent HTTP errors.

## 0.1.93-dev

- Replace Files hash-only navigation with stable direct routes: `/files/storage` and `/files/windows`.
- Keep legacy `/files#storage` and `/files#windows` links compatible by canonicalizing them to the new routes.
- Keep the Files sidebar entry active on nested routes and preserve access checks for management-only sections.
- Add Web navigation, SPA handler and integration smoke coverage for direct `/files/storage` loading.

## 0.1.92-dev

- Fix Files section navigation so Folders, Storage and Windows are addressable as stable deep links.
- Preserve `/files#storage` and `/files#windows` across reloads, browser Back/Forward navigation and direct links.
- Keep Storage/Windows management sections restricted to actors with `files.manage`, while limited file users stay on Folders.
- Add navigation regression coverage for Files section routing.

## 0.1.91-dev

- Complete user identity editing, administrative password reset and self-service password changes; credential changes and account disable revoke previous sessions. Protect the last enabled administrator in the same state transaction.
- Add migration 017, folder names/access/usage and folder/private-user quotas, including trash and pending transfers. Serialize reservations across pools; preserve private ownership and independent grants.
- Add verified ext4/XFS filesystem project quotas for folder caps and personal SMB allocations, retaining the existing kernel pool reserve. Unlimited folders retain existing SMB behavior; finite limits without verified filesystem caps are read-only over SMB.
- Reconcile managed Samba access after account, folder and capacity changes and at startup. Suspend active SMB handles before revocation; expose failures instead of retaining old access.
- Separate Files into Folders, Storage and Windows access; add a dedicated Account page and simplify the user editor. Preserve the disk-first storage UI from 0.1.90-dev.
- Protect uploaded content with download headers and filter private realtime/history events; refresh WebSocket authentication and bound cumulative subscriptions.
- Add real-session, concurrent quota/admin and Samba transition regressions, plus opt-in disposable-browser and kernel quota acceptance fixtures.

## 0.1.90-dev

- Moved physical-disk actions out of the partition table and into the selected disk summary card.
- Removed the Actions column from the selected-disk partition/LVM table.
- Added expandable per-partition management rows directly beneath each partition/LVM entry.
- Kept mount/unmount, label, purpose, format and delete controls together inside the expanded partition management panel.
- Preserved the existing system-disk and in-use Files storage protections without changing storage API behavior.
- Reduced the selected-disk table width further by removing the dedicated actions column.

## 0.1.89-dev

- Made the selected-disk partition/LVM table substantially more compact.
- Reduced the default table width from roughly 1540 px to roughly 1100 px.
- Reduced the Actions column from 330 px to 180 px and tightened the remaining storage columns.
- Reduced table cell, button and purpose-selector spacing while preserving all existing controls.
- Allow long table headings to wrap instead of forcing the whole storage table wider.
- Versioned the saved column-width preference so existing browsers receive the new compact defaults.

## 0.1.88-dev

- Reworked **System → Storage** into a disk-first workflow: the overview now shows only physical disks and their high-level state.
- Selecting a physical disk opens a focused disk page while preserving the existing partition/LVM management controls underneath.
- Removed destructive storage actions from the all-disks overview so formatting, deletion, mounting and purpose changes are scoped to one selected disk.
- Added a responsive disk summary with model, transport, capacity, partition table, unallocated space and SMART state.
- Reused the existing storage operation APIs and usage-lock protections without changing the server-side storage safety model.

## 0.1.87-dev

- Added kernel-enforced free-space reserve protection for direct SMB writes on quota-ready Home-AI storage.
- Extended privileged helper protocol v3 so managed Samba shares carry their NAS pool root and reserve policy.
- Reused the existing `force user = home-ai-core` Samba boundary so filesystem user quotas cover every managed SMB write.
- Compute the service-UID hard limit from current Home-AI usage plus only live free space above the configured pool reserve, accounting for pre-existing data outside `.home-ai`.
- Verify quota usage and hard limits through `repquota` and apply changes through `setquota`.
- Make `smb.apply` fail closed before Samba configuration activation when a non-zero reserve cannot be kernel-enforced.
- Synchronize kernel quota best-effort whenever an administrator changes a pool reserve and expose explicit SMB hard-quota readiness/error state in the Files UI.
- Install Linux quota tools alongside Samba.
- Prepare newly created ext4 storage with embedded user quotas and no separate ext4 root reserve; mount ext4 with `usrquota` and XFS with `uquota`.
- Keep legacy ext4/XFS conversion non-destructive: existing filesystems are not automatically reformatted or live-remounted when quota accounting is unavailable.
- Added ADR-0034 plus quota calculation, parser and SMB metadata tests; PR #65 passed the full CI suite.

## 0.1.86-dev

- Added per-NAS-pool free-space capacity policies with migration 016.
- Existing and new pools default to a 5% hard reserve and a 10% low-space warning threshold.
- Read live filesystem capacity with Linux `statfs` and expose free/total, reserve/warning bytes and capacity state through the Files API.
- Added a Web editor for pool reserve and warning percentages with visible OK / low-space / reserve-reached states.
- Enforce the hard reserve for direct uploads before their temporary file can cross the configured limit.
- Reject resumable uploads that cannot fit above the reserve and re-check live free space before every chunk.
- Return HTTP 507 `file_pool_reserve_reached` for capacity-blocked Core/Windows-client writes and fail capacity-consuming API writes closed when capacity cannot be read.
- Keep same-filesystem moves, recycle-bin operations and restores available because they do not materially increase occupied bytes.
- Documented the SMB boundary: direct Samba writes still bypass Core and need a later filesystem/Samba quota layer for hard enforcement.
- Added ADR-0033 plus state, filedata, API and Web client coverage; PR #63 passed the full CI suite.

## 0.1.85-dev

- Hardened physical storage that backs active Home-AI file pools against accidental destructive operations.
- Storage-purpose responses now expose whether a device is in use and which file pools depend on it.
- Block purpose clearing/reassignment, unmount, formatting and partition deletion while the target storage backs a Home-AI file pool.
- Added explicit Web status for storage used by Files and disabled destructive controls while it is active.
- Added migration 015 so new NAS pools persist their backing device path and filesystem UUID.
- New file pools can only be created on the exact mounted filesystem explicitly assigned to the `files` purpose.
- Prefer filesystem UUID over Linux device path for pool ownership and usage locks, preserving protection across unmounts and `/dev/...` renumbering.
- Keep a mount-path fallback for pools created before migration 015.
- Added storage identity, legacy compatibility and reformatted-filesystem safety tests; full CI passed for PRs #60 and #61.

## 0.1.84-dev

- Added `client update` to discover the newest compatible Home-AI Windows client release automatically.
- Require the exact versioned amd64 `.exe` and `.sha256` release asset pair before an update is accepted.
- Bound release metadata, checksum and executable downloads and require HTTPS outside loopback-only tests.
- Verify the published SHA-256 before activating the cached executable and verify the cached file again afterward.
- Launch only the verified downloaded executable into the existing per-user `client install` handoff.
- Allow the bounded Windows replacement handoff to wait for the old parent process to exit while preserving tray-agent stop/restart behavior.
- Cache verified update payloads under the current user's Home-AI cache directory without storing credentials.
- Added ADR-0032, release/download tests, native Windows tests and full cross-platform CI coverage.

## 0.1.83-dev

- Unified Home-AI household identities across the Core instead of module-specific user models.
- Added household profiles: Administrator, Parent, Child, Guest and Friend.
- Administrator retains full access; the final enabled Administrator cannot be disabled or demoted.
- Added direct per-user global permissions in addition to existing role permissions.
- Added a central access catalog so the Users Web page can show and edit current Core capabilities.
- Added exact per-user resource grants for NAS file folders, with separate read/write access.
- Converted shared-folder access from the legacy member-role behavior to explicit per-user assignment while preserving existing grants during migration.
- Aligned SMB share ACL generation with the same effective Core permissions and scoped file-folder grants used by Web/API.
- Added protection against self-escalation and against assigning administrator-only user-management permissions to non-administrator profiles.
- Added migration 014, ADR-0031, RU/EN Web UI, security/state/API tests and full CI coverage.

## 0.1.82-dev

- Added Russian and English localization to the native Windows settings client in one executable.
- Added first-launch Windows locale detection: Russian locales default to Russian, other locales default to English.
- Added a live **Русский / English** selector and persisted the non-secret language preference in `windows-client.json`.
- Localized settings labels, buttons, conflict policies, profile status, client-side validation, confirmations and status messages.
- Localized tray menu, sync summary, tooltip and success/failure notifications using the same saved language.
- Refresh the running tray immediately when the language changes without restarting the sync scheduler.
- Keep passwords exclusively in Windows Credential Manager; language selection does not affect credentials or sync profiles.
- Keep low-level server/OS error details verbatim inside localized error framing; the diagnostic CLI remains English.
- Added ADR-0029, localization persistence/translation tests, native Windows tests and full cross-platform CI coverage.
## 0.1.81-dev

- Added a native Win32 settings window to the existing Windows client while preserving all CLI commands.
- Launching the Windows client without arguments now opens settings; Explorer-created private console windows are hidden.
- Added server/account setup, writable Home-AI folder discovery, a native local-folder picker, sync interval and conflict-policy controls.
- Added sync profile create/edit/enable/disable/delete; edits preserve profile identity, enabled state and previous sync result metadata.
- Keep passwords exclusively in Windows Credential Manager; the new client settings JSON stores only the canonical server URL and username.
- Added GUI controls for manual **Sync now** and background-agent/autostart management; running agents receive sync requests through the existing single scheduler.
- Added **Settings** to the tray menu and made tray double-click open the settings window.
- Scope discovered NAS folders to the connected server/account to prevent accidental reuse after changing connection details.
- Added ADR-0028, settings persistence tests, profile-edit tests, Windows-native tests and full cross-platform CI coverage.
## 0.1.80-dev

- Added a bounded Windows self-update handoff so `client install` and `agent install` can replace a stable installed executable while the tray agent is running.
- Added strict parsing of the Home-AI-managed HKCU Run command and restart of the updated agent without accepting arbitrary shell commands.
- Preserve the existing temp-file, durable write, atomic activation and SHA-256 verification path during executable replacement.
- Added tray health status showing enabled profile count and the latest `OK` / `FAILED` sync result.
- Added Windows notifications for failed sync cycles and manual **Sync now** completion while keeping successful scheduled cycles silent.
- Added ADR-0027, parser/status tests, and a native Windows test that replaces a genuinely locked executable through the tray `WM_CLOSE` handoff.

## 0.1.79-dev

- Added a stable per-user Windows client install path at `%LOCALAPPDATA%\\HomeAI\\bin\\home-ai-windows-client.exe`.
- Added `client install/status`; installation uses a temporary file, durable write, atomic replacement and SHA-256 verification.
- Changed `agent install` to register the stable installed executable instead of an arbitrary downloaded release path.
- Added a native Win32 notification-area tray without a third-party GUI dependency.
- Added tray actions for `Sync now`, opening the agent log, opening sync profiles, and clean agent exit.
- Routed manual tray sync through the existing sequential scheduler and single-instance agent rather than starting a second process.
- Added ADR-0026, stable-path/native Windows tests, and full core CI coverage.
## 0.1.78-dev

- Added a per-user Windows background sync agent with `agent install/status/remove/run` commands.
- Added current-user HKCU Run autostart after Windows logon; the startup command contains no password or bearer token.
- Require enabled sync profiles and matching Windows Credential Manager entries before enabling autostart.
- Added a single-instance OS lock, hidden Windows console runtime, and rotating per-user sync-agent log.
- Reuse the existing scheduled sync watcher and Credential Manager authentication without introducing a LocalSystem service.
- Added ADR-0025 and native Windows HKCU Run round-trip tests.

## 0.1.77-dev

- Added Windows Credential Manager storage for Home-AI passwords without putting secrets in queue/sync JSON or command-line arguments.
- Added `credentials save/status/delete` CLI commands; `HOME_AI_PASSWORD` remains the explicit highest-priority one-shot override.
- Added Credential Manager fallback for Windows `folders`, copy, queue and scheduled sync authentication after process restart.
- Added native Windows tests that round-trip a real generic credential and authenticate the Home-AI client without an environment password.
- Added ADR-0024 and clarified that the future background launcher must run in the user's logon session rather than as LocalSystem.

## 0.1.76-dev

- Added persistent Windows push-sync profiles with explicit intervals and enable/disable state; profiles store no password or bearer token.
- Added `sync add/list/run/watch/enable/disable/remove` CLI commands and fresh source rescanning on every sync run.
- Added explicit destination conflict policies: `stop`, `skip` and recoverable `replace-to-trash` using the existing Home-AI recycle bin.
- Refuse automatic replacement of conflicting ancestor paths, and keep this first sync mode additive: local deletions are not mirrored to the server.
- Added ADR-0023, Windows-native tests and full core CI coverage for scheduled sync behavior.

## 0.1.75-dev

- Added recursive Windows file-client copying, including empty directories and safe creation of missing destination directories.
- Added a persistent local transfer queue with add/list/run/retry commands, process locking, atomic saves and recovery after interruption.
- Preserve completed work and verify queued source snapshots and existing destination checksums before treating a transfer as complete; credentials remain process-only.
- Commit resumable server uploads atomically without replacing a concurrently created target.
- Documented the queue contract, usage and remaining live NAS/SMB acceptance work.

## 0.1.74-dev

- Grouped Web navigation into Server, Services and Management, with a compact mobile drawer and keyboard support.
- Split System into Equipment, Storage, Network and Updates while preserving file, user, storage, network and update controls.
- Simplified Home to essential server status, warnings and recent activity; moved identifiers and diagnostic metadata into expandable details.
- Show readable operation and security-event labels while retaining progress, failures and troubleshooting details.
- Filter navigation and management controls by the current user's permissions.
- Show a downloaded, verified update as complete instead of an active 100% operation.


## Unreleased

### Repository cleanup and reliability audit

- Removed the unused legacy `internal/coreupdate` version-comparison package.
- Removed the unregistered Core demo module and empty top-level scaffold directories.
- Simplified routine CI so Debian packages are built only by the explicit initial-installer workflow.
- Removed a redundant updater-helper build from the update-release workflow.
- Switched update-status reads to the dedicated `updates.read` permission.
- Removed an unused storage inspection wrapper.
- Hardened storage unmount handling for devices mounted at multiple targets and added socket deadline margin.
- Refreshed updater, storage, runtime, packaging and repository-layout documentation to match the implemented architecture.
- Verified routine CI with dependency lock checks, gofmt, schema/shell checks, Web typecheck/tests/build, Go tests/vet, integration smoke test and amd64/arm64 cross-builds.

### Current platform work


- Enable and start the privileged Core update helper automatically during Debian package installation.

- Restarted project architecture from a clean working tree.
- Preserved the previous implementation in `archive/pre-restart-2026-09-28`.
- Defined the Home-AI-Core v1 product scope, module boundaries, multi-node direction, AI trust model and future mobile-sync requirements.
- Selected Go for the control plane and TypeScript/React for the Web UI.
- Defined an unprivileged public Core with a separate future privileged helper boundary.
- Added SQLite control-plane state with embedded transactional migrations and persistent node registration.
- Added read-only Linux hardware discovery for CPU, memory, block devices, network interfaces and basic GPU/PCI inventory.
- Added `/health` and `/api/v1/system` Core API endpoints with request correlation IDs.
- Added Debian/systemd runtime contracts and baseline service hardening.
- Added locked Go dependencies, unit tests, vet, integration daemon smoke tests and Linux amd64/arm64 cross-build validation.
- Defined Core API v1 error/correlation contracts and added realtime WebSocket Event Envelope v1 with subscriptions, heartbeat and reconnect semantics.
- Added first-owner bootstrap, Argon2id password hashing, persistent sessions, RBAC permissions, CSRF protection and security audit records.
- Added persistent typed jobs, cooperative cancellation, durable event history and cursor-backed realtime delivery.
- Added Module SDK v1 with strict manifests, dependency/conflict planning, lifecycle contracts, capability discovery and persistent read-only module registry APIs.
- Added Phase 8A Web UI foundation with first-run/login flows, Dashboard/System/Modules/Jobs/Audit pages, realtime refresh and same-origin static serving from Core.
- Added Debian 13 installation packages for amd64/arm64, packaged-artifact smoke tests and physical-server installation tooling.

- Added Russian Web UI localization with RU/EN switching, browser-language detection and saved preference.
- Started Phase 9 with signed module repository metadata and Ed25519/SHA-256 package verification.
- Added driver-independent PCI GPU inventory with model resolution and Web UI driver status; defined automatic hardware driver reconciliation through the future privileged helper.
- Added the Debian pci.ids hardware database as a package dependency for reliable human-readable PCI device names.
- Added Web-driven Core update discovery and installation with persistent jobs, SHA-256 staging verification, automatic reconnect and a constrained root update helper.
- Use release-safe `.dev` asset filenames while preserving Debian `~dev` package version ordering so Web update manifests match GitHub release assets exactly.
- Added a 0.1.7-dev acceptance release to validate the first real Web-driven Core upgrade from 0.1.6-dev.
