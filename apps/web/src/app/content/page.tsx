import type { Metadata } from "next";
import { ContentDashboard } from "@/components/content-dashboard";

export const metadata: Metadata = { title: "Content" };

export default function ContentPage() {
  return <ContentDashboard />;
}
