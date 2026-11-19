import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...values: ClassValue[]) {
  return twMerge(clsx(values));
}
export function clientID() {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}
export function formatBytes(size: number) {
  if (size < 1024) return `${size} B`;
  const exponent = Math.min(Math.floor(Math.log(size) / Math.log(1024)), 4);
  return `${(size / 1024 ** exponent).toFixed(2)} ${["B", "KiB", "MiB", "GiB", "TiB"][exponent]}`;
}
