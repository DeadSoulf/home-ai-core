import type { StringKey } from "./i18n";

export function jobLabelKey(type: string): StringKey {
  return type === "core.update.install" ? "jobUpdateInstall" : "backgroundTask";
}

const auditLabels: Record<string, StringKey> = {
  "auth.login": "auditSignIn",
  "auth.logout": "auditSignOut",
  "security.bootstrap": "auditSetup",
  "security.user.create": "auditUserCreate",
  "update.rollback": "auditRollback",
  "storage.mount": "mount",
  "storage.unmount": "unmount",
  "storage.format": "format",
  "storage.partition.create": "createPartition",
  "storage.partition.delete": "deletePartition",
  "storage.partition.delete_all": "deleteAllPartitions",
  "storage.label.rename": "renameDisk",
  "storage.disk.rename": "renamePhysicalDisk",
  "files.pool.create": "fileCreatePool",
  "files.folder.create": "fileCreateFolder",
  "files.directory.create": "fileCreateDirectory",
  "files.file.upload": "fileUpload",
  "files.file.upload.complete": "fileUpload",
  "files.entry.move": "fileMoveRename",
  "files.entry.trash": "fileTrash",
  "files.trash.restore": "fileRestore",
  "files.trash.purge": "fileDeletePermanently",
  "files.smb.install": "smbInstall",
  "files.smb.apply": "smbApply",
  "files.smb.set_password": "smbSetPassword",
  "ai.chat.message": "auditAIChat",
  "ai.tool.execute": "auditAITool",
};

export function auditLabelKey(action: string): StringKey {
  if (auditLabels[action]) return auditLabels[action];
  if (action.startsWith("files.smb.")) return "auditSMBChange";
  if (action.startsWith("files.")) return "auditFilesChange";
  if (action.startsWith("storage.")) return "auditStorageChange";
  if (action.startsWith("network.")) return "auditNetworkChange";
  return "auditOtherAction";
}

export function actorLabelKey(type: string): StringKey {
  return type === "user" ? "actorUser" : type === "anonymous" ? "actorAnonymous" : "actorSystem";
}
