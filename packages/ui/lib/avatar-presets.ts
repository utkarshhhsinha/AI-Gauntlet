export interface AvatarPreset {
  id: string;
  url: string;
  label: string;
}

/**
 * Built-in real profile avatars sourced from `packages/ui/avatar/` and
 * served by the backend at `/api/preset-avatars/<id>.jpg`.
 */
export const AVATAR_PRESETS: readonly AvatarPreset[] = [
  { id: "avatar-01", url: "/api/preset-avatars/avatar-01.jpg", label: "Avatar 1" },
  { id: "avatar-02", url: "/api/preset-avatars/avatar-02.jpg", label: "Avatar 2" },
  { id: "avatar-03", url: "/api/preset-avatars/avatar-03.jpg", label: "Avatar 3" },
  { id: "avatar-04", url: "/api/preset-avatars/avatar-04.jpg", label: "Avatar 4" },
  { id: "avatar-05", url: "/api/preset-avatars/avatar-05.jpg", label: "Avatar 5" },
  { id: "avatar-06", url: "/api/preset-avatars/avatar-06.jpg", label: "Avatar 6" },
  { id: "avatar-07", url: "/api/preset-avatars/avatar-07.jpg", label: "Avatar 7" },
  { id: "avatar-08", url: "/api/preset-avatars/avatar-08.jpg", label: "Avatar 8" },
  { id: "avatar-09", url: "/api/preset-avatars/avatar-09.jpg", label: "Avatar 9" },
  { id: "avatar-10", url: "/api/preset-avatars/avatar-10.jpg", label: "Avatar 10" },
];

export function pickRandomAvatarPreset(): string {
  const idx = Math.floor(Math.random() * AVATAR_PRESETS.length);
  return AVATAR_PRESETS[idx]?.url ?? AVATAR_PRESETS[0]!.url;
}

/**
 * Deterministically maps any legacy `emoji:...` marker to one of the real
 * profile avatars so existing agents never render an emoji character.
 */
export function presetForEmojiFallback(emoji: string): string {
  let hash = 0;
  for (let i = 0; i < emoji.length; i++) {
    hash = (hash * 31 + emoji.charCodeAt(i)) >>> 0;
  }
  return AVATAR_PRESETS[hash % AVATAR_PRESETS.length]!.url;
}
