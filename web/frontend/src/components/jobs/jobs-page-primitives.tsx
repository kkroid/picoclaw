import { IconLoader2 } from "@tabler/icons-react"
import type { ReactNode } from "react"

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { cn } from "@/lib/utils"

export function SummaryCard({
  title,
  value,
  detail,
  loading,
}: {
  title: string
  value: string
  detail: string
  loading?: boolean
}) {
  return (
    <Card className="border-border/60 bg-card/80" size="sm">
      <CardHeader>
        <CardDescription>{title}</CardDescription>
        <CardTitle className="text-2xl font-semibold">
          {loading ? <IconLoader2 className="size-5 animate-spin" /> : value}
        </CardTitle>
      </CardHeader>
      <CardContent className="text-muted-foreground text-xs">{detail}</CardContent>
    </Card>
  )
}

export function DetailSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section className="space-y-2 border-t border-border/60 pt-4 first:border-t-0 first:pt-0">
      <div className="text-sm font-semibold">{title}</div>
      {children}
    </section>
  )
}

export function KeyValue({
  label,
  value,
  code,
  inline,
}: {
  label: string
  value?: string
  code?: boolean
  inline?: boolean
}) {
  const content = value && value.trim() !== "" ? value : "-"
  return (
    <div className={cn("space-y-1", inline && "space-y-0")}>
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className={cn("text-sm", code && "font-mono break-all text-xs")}>{content}</div>
    </div>
  )
}

export function PathList({
  label,
  items,
}: {
  label: string
  items?: string[]
}) {
  if (!items || items.length === 0) {
    return <KeyValue label={label} value="-" />
  }
  return <KeyValue label={label} value={items.join("\n")} code />
}

export function FieldBlock({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <label className="space-y-1.5">
      <div className="text-muted-foreground text-xs">{label}</div>
      {children}
    </label>
  )
}

export function LoadingBlock({ label }: { label: string }) {
  return (
    <div className="text-muted-foreground flex items-center gap-2 py-6 text-sm">
      <IconLoader2 className="size-4 animate-spin" />
      {label}
    </div>
  )
}

export function ErrorBlock({ message }: { message: string }) {
  return (
    <div className="text-destructive rounded-lg bg-destructive/10 px-4 py-3 text-sm">
      {message}
    </div>
  )
}

export function EmptyBlock({ message }: { message: string }) {
  return (
    <div className="text-muted-foreground rounded-lg border border-dashed px-4 py-6 text-sm">
      {message}
    </div>
  )
}