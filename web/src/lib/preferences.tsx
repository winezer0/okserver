import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

import { zh } from "../locales/zh";
import { en } from "../locales/en";
type Key = keyof typeof zh;

function stored(key: string) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}
function persist(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* Private browsing. */
  }
}
const Context = createContext<{
  locale: "zh" | "en";
  theme: "light" | "dark";
  toggleLocale: () => void;
  toggleTheme: () => void;
  t: (key: Key) => string;
} | null>(null);
export function Preferences({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<"zh" | "en">(() =>
    stored("okserver.locale") === "en" ? "en" : "zh",
  );
  const [theme, setTheme] = useState<"light" | "dark">(() =>
    stored("okserver.theme") === "dark" ||
    (!stored("okserver.theme") && window.matchMedia?.("(prefers-color-scheme: dark)").matches)
      ? "dark"
      : "light",
  );
  useEffect(() => {
    document.documentElement.lang = locale === "zh" ? "zh-CN" : "en";
    persist("okserver.locale", locale);
    document.title = `${locale === "zh" ? zh.workspace : en.workspace} · okserver`;
  }, [locale]);
  useEffect(() => {
    document.documentElement.classList.toggle("dark", theme === "dark");
    document.documentElement.style.colorScheme = theme;
    persist("okserver.theme", theme);
  }, [theme]);
  return (
    <Context.Provider
      value={{
        locale,
        theme,
        toggleLocale: () => setLocale((value) => (value === "zh" ? "en" : "zh")),
        toggleTheme: () => setTheme((value) => (value === "light" ? "dark" : "light")),
        t: (key) => (locale === "zh" ? zh : en)[key],
      }}
    >
      {children}
    </Context.Provider>
  );
}
export function usePreferences() {
  const value = useContext(Context);
  if (!value) throw new Error("Missing Preferences provider");
  return value;
}
