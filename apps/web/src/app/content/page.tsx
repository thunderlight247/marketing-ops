import type { Metadata } from "next";
import { PlaceholderPage } from "@/components/placeholder-page";

export const metadata: Metadata = { title: "Content" };

export default function ContentPage() {
  return <PlaceholderPage section="content" />;
}
