import type { Metadata } from "next";
import { IntegrationsDashboard } from "@/components/integrations-dashboard";

export const metadata: Metadata = { title: "Integrations" };

export default function IntegrationsPage() {
  return <IntegrationsDashboard />;
}
