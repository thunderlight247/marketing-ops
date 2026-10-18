export type NavKey =
  "overview" | "workspaces" | "content" | "calendar" | "integrations" | "analytics" | "settings";

export interface NavItem {
  key: NavKey;
  href: string;
  label: string;
  summary: string;
  emptyTitle: string;
  emptyBody: string;
  plannedFor: string;
}

export const NAV_ITEMS: readonly NavItem[] = [
  {
    key: "overview",
    href: "/",
    label: "Overview",
    summary: "System status and where the platform stands.",
    emptyTitle: "",
    emptyBody: "",
    plannedFor: "",
  },
  {
    key: "workspaces",
    href: "/workspaces",
    label: "Workspaces",
    summary: "Client businesses you manage, each with its own profile, brand rules, and history.",
    emptyTitle: "No workspaces yet",
    emptyBody: "Client workspaces will be created and managed here.",
    plannedFor: "Milestone 2",
  },
  {
    key: "content",
    href: "/content",
    label: "Content",
    summary: "Drafts, reviews, and approvals for blog articles and social posts.",
    emptyTitle: "No content yet",
    emptyBody:
      "Nothing is drafted or published in this version. Nothing will publish without your approval.",
    plannedFor: "Milestones 4 and 5",
  },
  {
    key: "calendar",
    href: "/calendar",
    label: "Calendar",
    summary: "What is planned and scheduled to go out for each client.",
    emptyTitle: "Nothing scheduled",
    emptyBody: "Content planning and scheduled publishing will appear here.",
    plannedFor: "Milestones 4 and 9",
  },
  {
    key: "integrations",
    href: "/integrations",
    label: "Integrations",
    summary:
      "Connections to client websites, CMSs, and social accounts, and what each one supports.",
    emptyTitle: "No integrations connected",
    emptyBody: "No client credentials are stored in this version.",
    plannedFor: "Milestone 3",
  },
  {
    key: "analytics",
    href: "/analytics",
    label: "Analytics",
    summary: "Search, traffic, and enquiry numbers reported by connected sources.",
    emptyTitle: "No analytics data",
    emptyBody: "Numbers appear here only when a connected source provides them.",
    plannedFor: "Milestone 10",
  },
  {
    key: "settings",
    href: "/settings",
    label: "Settings",
    summary: "Operator preferences and security settings.",
    emptyTitle: "No settings yet",
    emptyBody: "Settings will be added alongside the features that need them.",
    plannedFor: "Added as needed",
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
