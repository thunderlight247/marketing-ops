import type { Lead, Campaign } from "@/lib/campaigns";

/**
 * Escapes a single CSV value according to RFC 4180.
 */
function escapeCsv(value: unknown): string {
  if (value === null || value === undefined) return '""';
  const str = String(value);
  if (str.includes('"') || str.includes(",") || str.includes("\n") || str.includes("\r")) {
    return `"${str.replace(/"/g, '""')}"`;
  }
  return `"${str}"`;
}

/**
 * Exports an array of leads to a downloadable CSV file in the browser.
 */
export function exportLeadsToCsv(leads: Lead[], campaigns: Campaign[], workspaceSlug = "workspace") {
  const headers = [
    "ID",
    "Contact Name",
    "Contact Phone",
    "Contact Email",
    "Status",
    "Source Kind",
    "Source Detail",
    "Campaign",
    "Follow Up Date",
    "Message",
    "Notes",
    "Created At",
  ];

  const campaignMap = new Map(campaigns.map((c) => [c.id, c.name]));

  const rows = leads.map((lead) => [
    lead.id,
    lead.contactName ?? "",
    lead.contactPhone ?? "",
    lead.contactEmail ?? "",
    lead.status,
    lead.sourceKind,
    lead.sourceDetail,
    lead.campaignId ? (campaignMap.get(lead.campaignId) ?? lead.campaignId) : "",
    lead.followUpAt ?? "",
    lead.message ?? "",
    lead.notes,
    lead.createdAt,
  ]);

  const csvContent = [
    headers.map(escapeCsv).join(","),
    ...rows.map((row) => row.map(escapeCsv).join(",")),
  ].join("\r\n");

  const blob = new Blob([csvContent], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  const dateStr = new Date().toISOString().slice(0, 10);
  link.setAttribute("href", url);
  link.setAttribute("download", `${workspaceSlug}-leads-${dateStr}.csv`);
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}

export interface ParsedCsvLead {
  contactName?: string;
  contactPhone?: string;
  contactEmail?: string;
  sourceKind: string;
  sourceDetail?: string;
  message?: string;
  notes?: string;
  followUpAt?: string;
}

/**
 * Parses raw CSV text into an array of structured lead rows.
 */
export function parseLeadsCsv(text: string): ParsedCsvLead[] {
  const lines = parseCsvLines(text);
  if (lines.length < 2) return [];

  const rawHeaders = lines[0].map((h) => h.toLowerCase().trim().replace(/[\s_-]+/g, ""));
  const headerIdx: Record<string, number> = {};

  rawHeaders.forEach((h, idx) => {
    if (["name", "contactname", "business", "businessname"].includes(h)) headerIdx.name = idx;
    if (["phone", "contactphone", "mobile", "telephone"].includes(h)) headerIdx.phone = idx;
    if (["email", "contactemail"].includes(h)) headerIdx.email = idx;
    if (["message", "notes", "description"].includes(h)) headerIdx.message = idx;
    if (["detail", "sourcedetail"].includes(h)) headerIdx.detail = idx;
    if (["followup", "followupdate", "followupat"].includes(h)) headerIdx.followUp = idx;
  });

  const parsed: ParsedCsvLead[] = [];

  for (let i = 1; i < lines.length; i++) {
    const row = lines[i];
    if (row.length === 0 || (row.length === 1 && !row[0].trim())) continue;

    const contactName = headerIdx.name !== undefined ? row[headerIdx.name]?.trim() : undefined;
    const contactPhone = headerIdx.phone !== undefined ? row[headerIdx.phone]?.trim() : undefined;
    const contactEmail = headerIdx.email !== undefined ? row[headerIdx.email]?.trim() : undefined;
    const message = headerIdx.message !== undefined ? row[headerIdx.message]?.trim() : undefined;
    const sourceDetail = headerIdx.detail !== undefined ? row[headerIdx.detail]?.trim() : "CSV import";
    const followUpAt = headerIdx.followUp !== undefined ? row[headerIdx.followUp]?.trim() : undefined;

    // Must have at least a name, phone, or email to be considered a lead
    if (contactName || contactPhone || contactEmail) {
      parsed.push({
        contactName: contactName || undefined,
        contactPhone: contactPhone || undefined,
        contactEmail: contactEmail || undefined,
        sourceKind: "import",
        sourceDetail,
        message: message || undefined,
        notes: "Imported via CSV",
        followUpAt: followUpAt || undefined,
      });
    }
  }

  return parsed;
}

function parseCsvLines(text: string): string[][] {
  const rows: string[][] = [];
  let currentRow: string[] = [];
  let currentField = "";
  let inQuotes = false;

  for (let i = 0; i < text.length; i++) {
    const char = text[i];
    const nextChar = text[i + 1];

    if (inQuotes) {
      if (char === '"' && nextChar === '"') {
        currentField += '"';
        i++; // skip escaped quote
      } else if (char === '"') {
        inQuotes = false;
      } else {
        currentField += char;
      }
    } else {
      if (char === '"') {
        inQuotes = true;
      } else if (char === ",") {
        currentRow.push(currentField);
        currentField = "";
      } else if (char === "\r" && nextChar === "\n") {
        currentRow.push(currentField);
        rows.push(currentRow);
        currentRow = [];
        currentField = "";
        i++;
      } else if (char === "\n" || char === "\r") {
        currentRow.push(currentField);
        rows.push(currentRow);
        currentRow = [];
        currentField = "";
      } else {
        currentField += char;
      }
    }
  }

  if (currentField || currentRow.length > 0) {
    currentRow.push(currentField);
    rows.push(currentRow);
  }

  return rows;
}
