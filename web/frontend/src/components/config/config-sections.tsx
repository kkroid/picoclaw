import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"

import {
  type CoreConfigForm,
  type LauncherForm,
} from "@/components/config/form-model"
import { Field, SwitchCardField } from "@/components/shared-form"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"

type UpdateCoreField = <K extends keyof CoreConfigForm>(
  key: K,
  value: CoreConfigForm[K],
) => void

type UpdateLauncherField = <K extends keyof LauncherForm>(
  key: K,
  value: LauncherForm[K],
) => void

interface ConfigSectionCardProps {
  title: string
  description?: string
  children: ReactNode
}

function ConfigSectionCard({
  title,
  description,
  children,
}: ConfigSectionCardProps) {
  return (
    <Card size="sm">
      <CardHeader className="border-border border-b">
        <CardTitle>{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent className="pt-0">
        <div className="divide-border/70 divide-y">{children}</div>
      </CardContent>
    </Card>
  )
}

interface AppFactorySectionProps {
  form: CoreConfigForm
  onFieldChange: UpdateCoreField
}

export function AppFactorySection({
  form,
  onFieldChange,
}: AppFactorySectionProps) {
  const { t } = useTranslation()

  return (
    <ConfigSectionCard title={t("pages.config.sections.appfactory")}>
      <Field
        label={t("pages.config.workspace")}
        hint={t("pages.config.workspace_hint")}
        layout="setting-row"
      >
        <Input
          value={form.workspace}
          onChange={(e) => onFieldChange("workspace", e.target.value)}
          placeholder="~/.appfactory/workspace"
        />
      </Field>

      <SwitchCardField
        label={t("pages.config.builder_runtime_enabled")}
        hint={t("pages.config.builder_runtime_enabled_hint")}
        layout="setting-row"
        checked={form.builderRuntimeEnabled}
        onCheckedChange={(checked) =>
          onFieldChange("builderRuntimeEnabled", checked)
        }
      />

      {form.builderRuntimeEnabled && (
        <>
          <Field
            label={t("pages.config.default_model")}
            hint={t("pages.config.default_model_hint")}
            layout="setting-row"
          >
            <Input
              value={form.defaultModelPrimary}
              onChange={(e) =>
                onFieldChange("defaultModelPrimary", e.target.value)
              }
              placeholder="qwen2.5-coder-14b-local"
            />
          </Field>

          <Field
            label={t("pages.config.default_model_fallbacks")}
            hint={t("pages.config.model_fallbacks_hint")}
            layout="setting-row"
            controlClassName="md:max-w-md"
          >
            <Textarea
              value={form.defaultModelFallbacksText}
              className="min-h-[88px]"
              onChange={(e) =>
                onFieldChange("defaultModelFallbacksText", e.target.value)
              }
            />
          </Field>

          <Field
            label={t("pages.config.upgrade_model")}
            hint={t("pages.config.upgrade_model_hint")}
            layout="setting-row"
          >
            <Input
              value={form.upgradeModelPrimary}
              onChange={(e) =>
                onFieldChange("upgradeModelPrimary", e.target.value)
              }
              placeholder="qwen2.5-coder-32b-local"
            />
          </Field>

          <Field
            label={t("pages.config.upgrade_model_fallbacks")}
            hint={t("pages.config.model_fallbacks_hint")}
            layout="setting-row"
            controlClassName="md:max-w-md"
          >
            <Textarea
              value={form.upgradeModelFallbacksText}
              className="min-h-[88px]"
              onChange={(e) =>
                onFieldChange("upgradeModelFallbacksText", e.target.value)
              }
            />
          </Field>

          <Field
            label={t("pages.config.max_attempts_before_upgrade")}
            hint={t("pages.config.max_attempts_before_upgrade_hint")}
            layout="setting-row"
          >
            <Input
              type="number"
              min={0}
              value={form.maxAttemptsBeforeUpgrade}
              onChange={(e) =>
                onFieldChange("maxAttemptsBeforeUpgrade", e.target.value)
              }
            />
          </Field>

          <Field
            label={t("pages.config.max_files_before_upgrade")}
            hint={t("pages.config.max_files_before_upgrade_hint")}
            layout="setting-row"
          >
            <Input
              type="number"
              min={0}
              value={form.maxFilesBeforeUpgrade}
              onChange={(e) =>
                onFieldChange("maxFilesBeforeUpgrade", e.target.value)
              }
            />
          </Field>

          <Field
            label={t("pages.config.max_schema_drift_before_upgrade")}
            hint={t("pages.config.max_schema_drift_before_upgrade_hint")}
            layout="setting-row"
          >
            <Input
              type="number"
              min={0}
              value={form.maxSchemaDriftBeforeUpgrade}
              onChange={(e) =>
                onFieldChange("maxSchemaDriftBeforeUpgrade", e.target.value)
              }
            />
          </Field>

          <Field
            label={t("pages.config.max_unrelated_operation_rate")}
            hint={t("pages.config.max_unrelated_operation_rate_hint")}
            layout="setting-row"
          >
            <Input
              type="number"
              min={0}
              max={1}
              step="0.01"
              value={form.maxUnrelatedOperationRate}
              onChange={(e) =>
                onFieldChange("maxUnrelatedOperationRate", e.target.value)
              }
            />
          </Field>

          <SwitchCardField
            label={t("pages.config.upgrade_on_validation_fail")}
            hint={t("pages.config.upgrade_on_validation_fail_hint")}
            layout="setting-row"
            checked={form.upgradeOnValidationFail}
            onCheckedChange={(checked) =>
              onFieldChange("upgradeOnValidationFail", checked)
            }
          />

          <SwitchCardField
            label={t("pages.config.upgrade_on_patch_parse_fail")}
            hint={t("pages.config.upgrade_on_patch_parse_fail_hint")}
            layout="setting-row"
            checked={form.upgradeOnPatchParseFail}
            onCheckedChange={(checked) =>
              onFieldChange("upgradeOnPatchParseFail", checked)
            }
          />

          <SwitchCardField
            label={t("pages.config.upgrade_on_scope_violation")}
            hint={t("pages.config.upgrade_on_scope_violation_hint")}
            layout="setting-row"
            checked={form.upgradeOnScopeViolation}
            onCheckedChange={(checked) =>
              onFieldChange("upgradeOnScopeViolation", checked)
            }
          />

          <SwitchCardField
            label={t("pages.config.upgrade_on_semantic_conflict")}
            hint={t("pages.config.upgrade_on_semantic_conflict_hint")}
            layout="setting-row"
            checked={form.upgradeOnSemanticConflict}
            onCheckedChange={(checked) =>
              onFieldChange("upgradeOnSemanticConflict", checked)
            }
          />
        </>
      )}
    </ConfigSectionCard>
  )
}

interface LauncherSectionProps {
  launcherForm: LauncherForm
  onFieldChange: UpdateLauncherField
  disabled: boolean
  autoStartEnabled: boolean
  autoStartHint: string
  autoStartDisabled: boolean
  onAutoStartChange: (checked: boolean) => void
}

export function LauncherSection({
  launcherForm,
  onFieldChange,
  disabled,
  autoStartEnabled,
  autoStartHint,
  autoStartDisabled,
  onAutoStartChange,
}: LauncherSectionProps) {
  const { t } = useTranslation()

  return (
    <ConfigSectionCard title={t("pages.config.sections.launcher")}>
      <SwitchCardField
        label={t("pages.config.lan_access")}
        hint={t("pages.config.lan_access_hint")}
        layout="setting-row"
        checked={launcherForm.publicAccess}
        disabled={disabled}
        onCheckedChange={(checked) => onFieldChange("publicAccess", checked)}
      />

      <Field
        label={t("pages.config.server_port")}
        hint={t("pages.config.server_port_hint")}
        layout="setting-row"
      >
        <Input
          type="number"
          min={1}
          max={65535}
          value={launcherForm.port}
          disabled={disabled}
          onChange={(e) => onFieldChange("port", e.target.value)}
        />
      </Field>

      <Field
        label={t("pages.config.allowed_cidrs")}
        hint={t("pages.config.allowed_cidrs_hint")}
        layout="setting-row"
        controlClassName="md:max-w-md"
      >
        <Textarea
          value={launcherForm.allowedCIDRsText}
          disabled={disabled}
          placeholder={t("pages.config.allowed_cidrs_placeholder")}
          className="min-h-[88px]"
          onChange={(e) => onFieldChange("allowedCIDRsText", e.target.value)}
        />
      </Field>

      <SwitchCardField
        label={t("pages.config.autostart_label")}
        hint={autoStartHint}
        layout="setting-row"
        checked={autoStartEnabled}
        disabled={autoStartDisabled}
        onCheckedChange={onAutoStartChange}
      />
    </ConfigSectionCard>
  )
}