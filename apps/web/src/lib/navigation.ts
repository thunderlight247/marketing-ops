export type NavKey =
  "overview" | "workspaces" | "content" | "calendar" | "integrations" | "analytics" | "settings";

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
    summary: "Client businesses you manage, each with its own profile, brand rules, and history.",
    emptyTitle: "No workspaces yet",
    emptyBody: "Client workspaces will be created and managed here.",
  },
  {
    key: "content",
    href: "/content",
    label: "Content",
    summary: "Drafts, reviews, and approvals for blog articles and social posts.",
    emptyTitle: "No content yet",
    emptyBody:
      "Nothing is drafted or published in this version. Nothing will publish without your approval.",
  },
  {
    key: "calendar",
    href: "/calendar",
    label: "Calendar",
    summary: "Scheduled and upcoming content across all your workspaces.",
    emptyTitle: "Nothing scheduled yet",
    emptyBody: "Content you schedule for publishing will appear here as a timeline. Create content in a workspace to get started.",
  },
  {
    key: "integrations",
    href: "/integrations",
    label: "Integrations",
    summary:
      "Connections to client websites, CMSs, and social accounts, and what each one supports.",
    emptyTitle: "No integrations connected",
    emptyBody: "No client credentials are stored in this version.",
  },
  {
    key: "analytics",
    href: "/analytics",
    label: "Analytics",
    summary: "Search rankings, traffic, and lead data from connected sources.",
    emptyTitle: "No analytics data yet",
    emptyBody: "Connect an integration that supports analytics — such as Google Search Console — and numbers will appear here automatically.",
  },
  {
    key: "settings",
    href: "/settings",
    label: "Settings",
    summary: "Operator preferences and platform configuration.",
    emptyTitle: "No settings configured",
    emptyBody: "Platform settings will be available here. For now, configuration is done through environment variables.",
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
