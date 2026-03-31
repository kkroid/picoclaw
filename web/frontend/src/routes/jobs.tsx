import { createFileRoute } from "@tanstack/react-router"

import { JobsPage } from "@/components/jobs/jobs-page"

export const Route = createFileRoute("/jobs")({
  component: JobsPage,
})