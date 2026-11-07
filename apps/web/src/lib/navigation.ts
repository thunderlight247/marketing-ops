export type NavKey = "overview" | "workspaces" | "settings";

export interface NavItem {
  key: NavKey;
  href: string;
  label: string;
  summary: string;
  emptyTitle: string;
  emptyBody: string;
}

export const NAV_ITEMS: readonly NavItem[] = [
  {
    key: "overview",
    href: "/",
    label: "Overview",
    summary: "System status and where the platform stands.",
    emptyTitle: "",
    emptyBody: "",
  },
  {
    key: "workspaces",
    href: "/workspaces",
    label: "Workspaces",
    summary: "Connected client workspaces, each with its own profile, brand rules, and history.",
    emptyTitle: "No workspaces yet",
    emptyBody: "Client workspaces will be created and managed here.",
  },
  {
    key: "settings",
    href: "/settings",
    label: "Settings",
    summary: "Operator preferences and platform configuration.",
    emptyTitle: "No settings configured",
    emptyBody:
      "Platform settings will be available here. For now, configuration is done through environment variables.",
  },
];

export function findNavItem(key: NavKey): NavItem {
  const item = NAV_ITEMS.find((candidate) => candidate.key === key);
  if (!item) throw new Error(`Unknown navigation key: ${key}`);
  return item;
}

/** True when `pathname` is the page for `href` or a page nested under it. */
export function isActive(pathname: string, href: string): boolean {
  if (href === "/") return pathname === "/";
  return pathname === href || pathname.startsWith(`${href}/`);
}
