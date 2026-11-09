/**
 * Platform settings persistence and defaults for the Marketing Operations Platform.
 */

export interface AgencyProfileSettings {
  agencyName: string;
  supportEmail: string;
  defaultTimezone: string;
  defaultCurrency: string;
  websiteUrl: string;
  brandTagline: string;
}

export interface AIEngineSettings {
  defaultProvider: "openrouter" | "openai" | "anthropic" | "stub";
  defaultModel: string;
  temperature: number;
  maxTokens: number;
  monthlyBudgetUsd: number;
  enableFallback: boolean;
  fallbackModel: string;
}

export interface NotificationSettings {
  slackWebhookUrl: string;
  discordWebhookUrl: string;
  notifyOnInboundLead: boolean;
  notifyOnWhatsAppDisconnect: boolean;
  notifyOnPublishFailure: boolean;
  dailyDigestEmail: string;
}

export interface SecuritySettings {
  sessionTimeoutHours: number;
  enforceBearerAuth: boolean;
  allowedIpRanges: string;
}

export interface PlatformSettings {
  agency: AgencyProfileSettings;
  ai: AIEngineSettings;
  notifications: NotificationSettings;
  security: SecuritySettings;
}

export const DEFAULT_PLATFORM_SETTINGS: PlatformSettings = {
  agency: {
    agencyName: "Marketing Operations Agency",
    supportEmail: "ops@marketingagency.com",
    defaultTimezone: "UTC",
    defaultCurrency: "USD",
    websiteUrl: "https://marketingagency.com",
    brandTagline: "Autonomous multi-client marketing ops and lead generation",
  },
  ai: {
    defaultProvider: "openrouter",
    defaultModel: "openai/gpt-4o",
    temperature: 0.7,
    maxTokens: 4000,
    monthlyBudgetUsd: 100,
    enableFallback: true,
    fallbackModel: "anthropic/claude-3.5-haiku",
  },
  notifications: {
    slackWebhookUrl: "",
    discordWebhookUrl: "",
    notifyOnInboundLead: true,
    notifyOnWhatsAppDisconnect: true,
    notifyOnPublishFailure: true,
    dailyDigestEmail: "",
  },
  security: {
    sessionTimeoutHours: 24,
    enforceBearerAuth: true,
    allowedIpRanges: "",
  },
};

const STORAGE_KEY = "marketing_ops_platform_settings_v1";

export function loadPlatformSettings(): PlatformSettings {
  if (typeof window === "undefined") {
    return DEFAULT_PLATFORM_SETTINGS;
  }
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULT_PLATFORM_SETTINGS;
    const parsed = JSON.parse(raw);
    return {
      agency: { ...DEFAULT_PLATFORM_SETTINGS.agency, ...(parsed.agency || {}) },
      ai: { ...DEFAULT_PLATFORM_SETTINGS.ai, ...(parsed.ai || {}) },
      notifications: { ...DEFAULT_PLATFORM_SETTINGS.notifications, ...(parsed.notifications || {}) },
      security: { ...DEFAULT_PLATFORM_SETTINGS.security, ...(parsed.security || {}) },
    };
  } catch {
    return DEFAULT_PLATFORM_SETTINGS;
  }
}

export function savePlatformSettings(settings: PlatformSettings): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
  } catch (err) {
    console.error("Failed to save platform settings to localStorage:", err);
  }
}
