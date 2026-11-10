import type { Lead, Campaign } from "@/lib/campaigns";
import type { LeadEngineLead } from "@/lib/leadEngine";

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
 * Exports an array of Lead Engine leads to a downloadable CSV file.
 */
export function exportLeadEngineLeadsToCsv(leads: LeadEngineLead[], workspaceSlug = "workspace") {
  const headers = [
    "ID",
    "Business Name",
    "Phone",
    "Category",
    "City",
    "Address",
    "Rating",
    "Review Count",
    "Website",
    "Status",
    "Pitch",
    "Place ID",
    "Created At",
  ];

  const rows = leads.map((lead) => [
    lead.id,
    lead.name,
    lead.phone,
    lead.category,
    lead.city,
    lead.address,
    lead.rating ?? "",
    // A source without ratings (OpenStreetMap) has no review count to report;
    // exporting 0 would look like a real "no reviews" measurement.
    lead.rating === null ? "" : lead.reviewCount,
    lead.websiteUrl,
    lead.status,
    lead.pitch,
    lead.placeId ?? "",
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
  link.setAttribute("download", `${workspaceSlug}-lead-engine-${dateStr}.csv`);
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}

/**
 * Exports an array of leads to a downloadable CSV file in the browser.
 */
export function exportLeadsToCsv(
  leads: Lead[],
  campaigns: Campaign[],
  workspaceSlug = "workspace",
) {
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

  const rawHeaders = lines[0].map((h) =>
    h
      .toLowerCase()
      .trim()
      .replace(/[\s_-]+/g, ""),
  );
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
    const sourceDetail =
      headerIdx.detail !== undefined ? row[headerIdx.detail]?.trim() : "CSV import";
    const followUpAt =
      headerIdx.followUp !== undefined ? row[headerIdx.followUp]?.trim() : undefined;

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

export interface ParsedLeadEngineCsvItem {
  name: string;
  phone: string;
  category?: string;
  city?: string;
  address?: string;
  websiteUrl?: string;
}

/**
 * Parses CSV specifically for Lead Engine outreach import.
 * Detects Name, Phone, Category, City, Address, and Website columns.
 */
export function parseLeadEngineCsv(text: string): ParsedLeadEngineCsvItem[] {
  const lines = parseCsvLines(text);
  if (lines.length < 2) return [];

  const rawHeaders = lines[0].map((h) =>
    h
      .toLowerCase()
      .trim()
      .replace(/[\s_-]+/g, ""),
  );
  const headerIdx: Record<string, number> = {};

  rawHeaders.forEach((h, idx) => {
    if (["name", "businessname", "business", "company", "companyname", "contactname"].includes(h)) {
      headerIdx.name = idx;
    }
    if (["phone", "phonenumber", "mobile", "whatsapp", "telephone", "contactphone"].includes(h)) {
      headerIdx.phone = idx;
    }
    if (["category", "industry", "type", "sector"].includes(h)) {
      headerIdx.category = idx;
    }
    if (["city", "location", "town", "region"].includes(h)) {
      headerIdx.city = idx;
    }
    if (["address", "street", "streetaddress"].includes(h)) {
      headerIdx.address = idx;
    }
    if (["website", "websiteurl", "url", "link", "web"].includes(h)) {
      headerIdx.website = idx;
    }
  });

  const parsed: ParsedLeadEngineCsvItem[] = [];

  for (let i = 1; i < lines.length; i++) {
    const row = lines[i];
    if (row.length === 0 || (row.length === 1 && !row[0].trim())) continue;

    const phone = headerIdx.phone !== undefined ? row[headerIdx.phone]?.trim() : "";
    if (!phone) continue; // Phone is mandatory for WhatsApp outreach

    const name =
      headerIdx.name !== undefined ? row[headerIdx.name]?.trim() : "Prospective Business";
    const category = headerIdx.category !== undefined ? row[headerIdx.category]?.trim() : "";
    const city = headerIdx.city !== undefined ? row[headerIdx.city]?.trim() : "";
    const address = headerIdx.address !== undefined ? row[headerIdx.address]?.trim() : "";
    const websiteUrl = headerIdx.website !== undefined ? row[headerIdx.website]?.trim() : "";

    parsed.push({
      name: name || "Prospective Business",
      phone,
      category: category || undefined,
      city: city || undefined,
      address: address || undefined,
      websiteUrl: websiteUrl || undefined,
    });
  }

  return parsed;
}
