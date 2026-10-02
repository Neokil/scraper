export function log(level: "info" | "warn" | "error", message: string, fields: Record<string, unknown> = {}): void {
  process.stdout.write(JSON.stringify({
    timestamp: new Date().toISOString(),
    level,
    message,
    ...fields,
  }) + "\n");
}
