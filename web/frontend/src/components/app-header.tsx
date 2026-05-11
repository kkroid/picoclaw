import {
  IconBrandGithub,
  IconLanguage,
  IconLoader2,
  IconMenu2,
  IconMoon,
  IconSun,
} from "@tabler/icons-react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { getOrchestratorStatus } from "@/api/system"
import { Button } from "@/components/ui/button.tsx"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu.tsx"
import { Separator } from "@/components/ui/separator.tsx"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { useTheme } from "@/hooks/use-theme.ts"
import {
  getOrchestratorLabel,
  getOrchestratorToneClass,
} from "@/lib/orchestrator-status"

export function AppHeader() {
  const { i18n, t } = useTranslation()
  const { theme, toggleTheme } = useTheme()

  const orchestratorQuery = useQuery({
    queryKey: ["system", "orchestrator-status"],
    queryFn: getOrchestratorStatus,
    refetchInterval: 5000,
  })

  const orchestrator = orchestratorQuery.data
  const orchestratorTone = getOrchestratorToneClass(orchestrator)
  const orchestratorLabel = getOrchestratorLabel(orchestrator, t)

  return (
    <header className="bg-background/95 supports-backdrop-filter:bg-background/60 border-b-border/50 sticky top-0 z-50 flex h-14 shrink-0 items-center justify-between border-b px-4 backdrop-blur">
      <div className="flex items-center gap-2">
        <SidebarTrigger className="text-muted-foreground hover:bg-accent hover:text-foreground flex h-9 w-9 items-center justify-center rounded-lg sm:hidden [&>svg]:size-5">
          <IconMenu2 />
        </SidebarTrigger>
        <Link to="/" className="hidden text-sm font-semibold tracking-normal sm:flex">
          OneAppFactory
        </Link>
      </div>

      <div className="text-muted-foreground flex items-center gap-1 text-sm font-medium md:gap-2">
        <Button
          variant="outline"
          size="sm"
          className="hidden h-8 gap-2 rounded-full px-3 md:inline-flex"
          asChild
        >
          <Link to="/">
            {orchestratorQuery.isLoading ? (
              <IconLoader2 className="size-3.5 animate-spin" />
            ) : (
              <span className={`inline-flex size-2 rounded-full ${orchestratorTone}`} />
            )}
            <span className="text-xs font-semibold">{orchestratorLabel}</span>
          </Link>
        </Button>

        <Separator
          className="mx-4 my-2 hidden md:block"
          orientation="vertical"
        />

        <Button variant="ghost" size="icon" className="size-8" asChild>
          <a
            href="https://github.com/sipeed/oneappfactory"
            target="_blank"
            rel="noreferrer"
            aria-label="GitHub"
          >
            <IconBrandGithub className="size-4.5" />
          </a>
        </Button>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="size-8">
              <IconLanguage className="size-4.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => i18n.changeLanguage("en")}>
              English
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => i18n.changeLanguage("zh")}>
              简体中文
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <Button
          variant="ghost"
          size="icon"
          className="size-8"
          onClick={toggleTheme}
          aria-label={t("header.theme.toggle")}
        >
          {theme === "dark" ? (
            <IconSun className="size-4.5" />
          ) : (
            <IconMoon className="size-4.5" />
          )}
        </Button>
      </div>
    </header>
  )
}
