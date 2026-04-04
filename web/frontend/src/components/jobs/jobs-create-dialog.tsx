import { IconChevronDown, IconLoader2, IconPlus } from "@tabler/icons-react"

import { FieldBlock } from "@/components/jobs/jobs-page-primitives"
import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"

type Translate = (key: string, options?: Record<string, unknown>) => string

export type CreateJobFormState = {
  requirementText: string
  title: string
  advancedOpen: boolean
  realChecks: boolean
  autoStartBuilder: boolean
}

export function createDefaultCreateJobForm(): CreateJobFormState {
  return {
    requirementText: "",
    title: "",
    advancedOpen: false,
    realChecks: true,
    autoStartBuilder: true,
  }
}

export function JobsCreateDialog({
  open,
  pending,
  form,
  t,
  onOpenChange,
  onFormChange,
  onSubmit,
  onCancel,
}: {
  open: boolean
  pending: boolean
  form: CreateJobFormState
  t: Translate
  onOpenChange: (open: boolean) => void
  onFormChange: (updater: (current: CreateJobFormState) => CreateJobFormState) => void
  onSubmit: () => void
  onCancel: () => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] min-h-[28rem] sm:min-h-0">
        <DialogHeader>
          <DialogTitle>{t("jobs.create.title")}</DialogTitle>
          <DialogDescription className="max-w-xl">{t("jobs.create.description")}</DialogDescription>
        </DialogHeader>

        <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-6 py-5">
          <FieldBlock label={t("jobs.create.fields.title")}>
            <Input
              value={form.title}
              onChange={(event) =>
                onFormChange((current) => ({
                  ...current,
                  title: event.target.value,
                }))
              }
              placeholder={t("jobs.create.placeholders.title")}
            />
          </FieldBlock>

          <FieldBlock label={t("jobs.create.fields.requirementText")}>
            <Textarea
              value={form.requirementText}
              onChange={(event) =>
                onFormChange((current) => ({
                  ...current,
                  requirementText: event.target.value,
                }))
              }
              rows={10}
              placeholder={t("jobs.create.placeholders.requirementText")}
              className="min-h-40 resize-y"
            />
          </FieldBlock>

          <Collapsible
            open={form.advancedOpen}
            onOpenChange={(open) =>
              onFormChange((current) => ({
                ...current,
                advancedOpen: open,
              }))
            }
            className="rounded-2xl border border-border/60 bg-muted/15"
          >
            <CollapsibleTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                className="flex h-auto w-full items-start justify-between rounded-2xl px-4 py-4 text-left"
              >
                <div className="space-y-1">
                  <div className="font-medium">{t("jobs.create.advanced.trigger")}</div>
                  <p className="text-muted-foreground text-sm">
                    {t("jobs.create.advanced.description")}
                  </p>
                </div>
                <IconChevronDown className="mt-0.5 size-4 shrink-0 transition-transform data-[state=open]:rotate-180" />
              </Button>
            </CollapsibleTrigger>

            <CollapsibleContent className="space-y-3 border-t border-border/60 px-4 py-4">
              <div className="flex items-start justify-between gap-4 rounded-xl border border-border/60 bg-background/80 px-4 py-3">
                <div className="space-y-1">
                  <div className="font-medium">{t("jobs.create.advanced.realChecks.label")}</div>
                  <p className="text-muted-foreground text-sm">
                    {t("jobs.create.advanced.realChecks.description")}
                  </p>
                </div>
                <Switch
                  checked={form.realChecks}
                  onCheckedChange={(checked) =>
                    onFormChange((current) => ({
                      ...current,
                      realChecks: checked,
                    }))
                  }
                  aria-label={t("jobs.create.advanced.realChecks.label")}
                />
              </div>

              <div className="flex items-start justify-between gap-4 rounded-xl border border-border/60 bg-background/80 px-4 py-3">
                <div className="space-y-1">
                  <div className="font-medium">{t("jobs.create.advanced.autoStartBuilder.label")}</div>
                  <p className="text-muted-foreground text-sm">
                    {t("jobs.create.advanced.autoStartBuilder.description")}
                  </p>
                </div>
                <Switch
                  checked={form.autoStartBuilder}
                  onCheckedChange={(checked) =>
                    onFormChange((current) => ({
                      ...current,
                      autoStartBuilder: checked,
                    }))
                  }
                  aria-label={t("jobs.create.advanced.autoStartBuilder.label")}
                />
              </div>
            </CollapsibleContent>
          </Collapsible>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onCancel} disabled={pending}>
            {t("common.cancel")}
          </Button>
          <Button onClick={onSubmit} disabled={pending}>
            {pending ? <IconLoader2 className="size-4 animate-spin" /> : <IconPlus className="size-4" />}
            {t("jobs.actions.createJob")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}