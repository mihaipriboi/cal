import { Globe } from "lucide-react";
import { useState } from "react";
import { deviceTimeZone } from "../lib/date";
import { usePlanner } from "../stores/planner";

const KEEP_KEY = "cal.zoneBanner.keep";

function readKept(): string | null {
  try {
    return localStorage.getItem(KEEP_KEY);
  } catch {
    return null;
  }
}

// ZoneBanner offers to switch Cal to this device's time zone when it differs
// from Settings → Timezone, which "today", reminders and subscribed calendars
// follow. Keep hides it on this device until either zone changes.
export function ZoneBanner() {
  const settings = usePlanner((state) => state.settings);
  const updateSettings = usePlanner((state) => state.updateSettings);
  const [kept, setKept] = useState(readKept);
  const device = deviceTimeZone();
  const current = settings.timezone || "UTC";
  const pair = `${device}|${current}`;
  if (device === current || kept === pair) return null;

  const keep = () => {
    try {
      localStorage.setItem(KEEP_KEY, pair);
    } catch {
      /* private mode: hidden until reload */
    }
    setKept(pair);
  };

  return (
    <div className="zone-banner" role="status">
      <Globe size={14} aria-hidden />
      <span className="zone-banner-text">
        This device is in <b>{device}</b>; Cal uses <b>{current}</b>.
      </span>
      <span className="zone-banner-actions">
        <button type="button" className="btn btn-primary" onClick={() => void updateSettings({ ...settings, timezone: device })}>
          Switch
        </button>
        <button type="button" className="btn btn-ghost" onClick={keep}>
          Keep
        </button>
      </span>
    </div>
  );
}
