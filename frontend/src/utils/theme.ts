import { theme } from "./constants";
import "ace-builds";
import { themesByName } from "ace-builds/src-noconflict/ext-themelist";

const THEME_STORAGE_KEY = "filebrowser.theme";

export const getTheme = (): UserTheme => {
  const stored = localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === "light" || stored === "dark") {
    return stored;
  }
  return (document.documentElement.className as UserTheme) || theme;
};

export const setTheme = (theme: UserTheme) => {
  const html = document.documentElement;
  if (!theme) {
    html.className = getMediaPreference();
  } else {
    html.className = theme;
  }
};

export const toggleTheme = (): void => {
  const activeTheme = getTheme();
  setTheme(activeTheme === "light" ? "dark" : "light");
  localStorage.setItem(THEME_STORAGE_KEY, document.documentElement.className);
};

export const getMediaPreference = (): UserTheme => {
  const hasDarkPreference = window.matchMedia(
    "(prefers-color-scheme: dark)"
  ).matches;
  if (hasDarkPreference) {
    return "dark";
  } else {
    return "light";
  }
};

export const getEditorTheme = (themeName: string) => {
  if (!themeName.startsWith("ace/theme/")) {
    themeName = `ace/theme/${themeName}`;
  }
  const themeKey = themeName.replace("ace/theme/", "");
  if (themesByName[themeKey] !== undefined) {
    return themeName;
  } else if (getTheme() === "dark") {
    return "ace/theme/twilight";
  } else {
    return "ace/theme/chrome";
  }
};
