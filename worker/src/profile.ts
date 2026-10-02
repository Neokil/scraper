import { chromium } from "playwright";
import type { Profile } from "./types.js";

type PersistentContextOptions = NonNullable<Parameters<typeof chromium.launchPersistentContext>[1]>;

export function contextOptions(profile: Profile): PersistentContextOptions {
  const viewport = profile.viewport ?? { width: 1920, height: 1080 };
  const options: PersistentContextOptions = {
    headless: false,
    viewport,
    args: ["--window-size=" + viewport.width + "," + viewport.height],
  };
  if (profile.userAgent) options.userAgent = profile.userAgent;
  if (profile.locale) options.locale = profile.locale;
  if (profile.timezoneId) options.timezoneId = profile.timezoneId;
  if (profile.deviceScaleFactor) options.deviceScaleFactor = profile.deviceScaleFactor;
  if (profile.isMobile) options.isMobile = true;
  if (profile.hasTouch) options.hasTouch = true;
  if (profile.colorScheme) options.colorScheme = profile.colorScheme;
  if (profile.reducedMotion) options.reducedMotion = profile.reducedMotion;
  if (profile.javaScriptEnabled !== undefined) options.javaScriptEnabled = profile.javaScriptEnabled;
  if (profile.ignoreHTTPSErrors) options.ignoreHTTPSErrors = true;
  if (profile.acceptDownloads !== undefined) options.acceptDownloads = profile.acceptDownloads;
  if (profile.geolocation) options.geolocation = profile.geolocation;
  if (profile.permissions) options.permissions = profile.permissions;
  if (profile.extraHTTPHeaders) options.extraHTTPHeaders = profile.extraHTTPHeaders;
  if (profile.proxy) {
    options.proxy = { server: profile.proxy.server };
    if (profile.proxy.bypass) options.proxy.bypass = profile.proxy.bypass;
    if (profile.proxy.username) options.proxy.username = profile.proxy.username;
    if (profile.proxy.password) options.proxy.password = profile.proxy.password;
  }
  return options;
}
