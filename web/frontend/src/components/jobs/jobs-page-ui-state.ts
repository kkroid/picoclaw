import { useState } from "react"

import type { PublicJobRecord } from "@/api/system"
import {
  createDefaultCreateJobForm,
  type CreateJobFormState,
} from "@/components/jobs/jobs-create-dialog"
import {
  createDefaultDeliveryForm,
  createDefaultFollowUpForm,
  type DeliveryFormState,
  type FollowUpFormState,
} from "@/components/jobs/jobs-delivery-utils"
import type { JobWorkspaceTab } from "@/components/jobs/jobs-workspace-controller"

export function useJobsPageUiState({
  jobItems,
}: {
  jobItems: PublicJobRecord[]
}) {
  const [selectedJobId, setSelectedJobId] = useState("")
  const [detailTab, setDetailTab] = useState<JobWorkspaceTab>("overview")
  const [showSystemStatus, setShowSystemStatus] = useState(false)
  const [showCreateJobSheet, setShowCreateJobSheet] = useState(false)
  const [createJobForm, setCreateJobForm] = useState<CreateJobFormState>(
    createDefaultCreateJobForm(),
  )
  const [showDeliveryForm, setShowDeliveryForm] = useState(false)
  const [showFollowUpForm, setShowFollowUpForm] = useState(false)
  const [deliveryForm, setDeliveryForm] = useState<DeliveryFormState>(createDefaultDeliveryForm())
  const [followUpForm, setFollowUpForm] = useState<FollowUpFormState>(createDefaultFollowUpForm())

  const effectiveSelectedJobId = jobItems.some((item) => item.job_id === selectedJobId)
    ? selectedJobId
    : (jobItems[0]?.job_id ?? "")

  const resetCreateJobForm = () => {
    setShowCreateJobSheet(false)
    setCreateJobForm(createDefaultCreateJobForm())
  }

  return {
    selectedJobId,
    setSelectedJobId,
    effectiveSelectedJobId,
    detailTab,
    setDetailTab,
    showSystemStatus,
    setShowSystemStatus,
    showCreateJobSheet,
    setShowCreateJobSheet,
    createJobForm,
    setCreateJobForm,
    resetCreateJobForm,
    showDeliveryForm,
    setShowDeliveryForm,
    showFollowUpForm,
    setShowFollowUpForm,
    deliveryForm,
    setDeliveryForm,
    followUpForm,
    setFollowUpForm,
  }
}