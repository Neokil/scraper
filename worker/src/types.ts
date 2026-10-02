export interface Viewport {
  width: number;
  height: number;
}

export interface Geolocation {
  latitude: number;
  longitude: number;
  accuracy?: number;
}

export interface ProxySettings {
  server: string;
  bypass?: string;
  username?: string;
  password?: string;
}

export interface Profile {
  name: string;
  description?: string;
  browser: "chromium";
  userAgent?: string;
  locale?: string;
  timezoneId?: string;
  viewport?: Viewport;
  deviceScaleFactor?: number;
  isMobile?: boolean;
  hasTouch?: boolean;
  colorScheme?: "light" | "dark" | "no-preference";
  reducedMotion?: "reduce" | "no-preference";
  javaScriptEnabled?: boolean;
  ignoreHTTPSErrors?: boolean;
  acceptDownloads?: boolean;
  geolocation?: Geolocation;
  permissions?: string[];
  extraHTTPHeaders?: Record<string, string>;
  proxy?: ProxySettings;
}

export interface CreateSessionRequest {
  id: string;
  browser: "chromium";
  profile: string;
  idle_timeout_seconds: number;
  profile_settings: Profile;
}

export interface CreatePageRequest {
  id: string;
  url?: string;
}

export interface PageState {
  id: string;
  session_id: string;
  url: string;
  title: string;
  status: string;
  created_at: string;
  last_activity: string;
}

export interface SessionState {
  id: string;
  worker: string;
  status: "running";
  browser: "chromium";
  profile: string;
  created_at: string;
  last_activity: string;
  idle_timeout_seconds: number;
  pages: PageState[];
}

export interface EventRecord {
  id: string;
  timestamp: string;
  session?: string;
  page?: string;
  type: string;
  message: string;
  url?: string;
  snapshot_id?: string;
  context?: Record<string, unknown>;
}

export interface Snapshot {
  id: string;
  session?: string;
  page?: string;
  class: "periodic" | "error" | "manual";
  format: "png" | "jpeg";
  created_at: string;
  size_bytes: number;
}

export interface WorkerRegistration {
  worker_id: string;
  hostname: string;
  version: string;
  base_url: string;
  vnc_url: string;
  incarnation_id: string;
  capacity: number;
  cpu_percent: number;
  memory_bytes: number;
  sessions: SessionState[];
  snapshots: Snapshot[];
}

export interface ActionRequest {
  selector?: string;
  timeout?: number;
  [key: string]: unknown;
}
