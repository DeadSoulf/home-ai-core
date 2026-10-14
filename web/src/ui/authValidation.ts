export type AuthFields = {username: string; password: string; bootstrapToken?: string; displayName?: string; confirmation?: string};
export function validateAuth(fields: AuthFields, setup: boolean): Record<string, import("./messages").UIKey> {
  const errors: Record<string, import("./messages").UIKey> = {};
  if (!fields.username.trim()) errors.username = "required";
  else if (!/^[a-z0-9][a-z0-9._-]{2,63}$/i.test(fields.username.trim())) errors.username = "usernameInvalid";
  const bytes = new TextEncoder().encode(fields.password).length;
  if (!bytes) errors.password = "required";
  else if (setup && bytes < 12 || bytes > 1024) errors.password = setup ? "passwordSize" : "passwordTooLong";
  if (setup) {
    if (!fields.bootstrapToken?.trim()) errors.bootstrapToken = "required";
    if ([...(fields.displayName?.trim() || "")].length > 128) errors.displayName = "nameTooLong";
    if (fields.confirmation !== fields.password) errors.confirmation = "passwordMismatch";
  }
  return errors;
}
