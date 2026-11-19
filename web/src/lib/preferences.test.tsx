import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { Preferences, usePreferences } from "./preferences";
function Controls() {
  const { t, toggleLocale, toggleTheme } = usePreferences();
  return (
    <>
      <p>{t("workspace")}</p>
      <button onClick={toggleLocale}>language</button>
      <button onClick={toggleTheme}>theme</button>
    </>
  );
}
it("switches language and color theme and persists both", async () => {
  render(
    <Preferences>
      <Controls />
    </Preferences>,
  );
  expect(screen.getByText("文件空间")).toBeInTheDocument();
  await userEvent.click(screen.getByText("language"));
  expect(screen.getByText("File workspace")).toBeInTheDocument();
  expect(document.documentElement.lang).toBe("en");
  await userEvent.click(screen.getByText("theme"));
  expect(document.documentElement).toHaveClass("dark");
  expect(localStorage.getItem("okserver.theme")).toBe("dark");
  expect(localStorage.getItem("okserver.locale")).toBe("en");
});
