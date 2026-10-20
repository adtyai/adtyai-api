/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { zodResolver } from "@hookform/resolvers/zod";
import { Link } from "@tanstack/react-router";
import { Info, RefreshCw, Save, ShieldAlert } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useForm, type Resolver } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";

import { ConfirmDialog } from "@/components/confirm-dialog";
import { ErrorState } from "@/components/error-state";
import { SectionPageLayout } from "@/components/layout";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { backupContactTypeLabelKey } from "@/features/system-settings/utils/status-labels";
import { ROLE } from "@/lib/roles";
import { getServerErrorMessage } from "@/lib/server-error-message";
import { useAuthStore } from "@/stores/auth-store";

import { SettingsForm } from "../components/settings-form-layout";
import { useChatwootConfig, usePutChatwootConfig } from "./hooks/use-chatwoot";
import {
  CHATWOOT_FORM_DEFAULTS,
  buildChatwootPut,
  chatwootPutNeedsConfirm,
  configToForm,
  formatHmacFingerprint,
} from "./lib/put";
import type { ChatwootFormValues, ChatwootPutInput } from "./types";

const schema = z.object({
  script: z.string(),
  website_token: z.string(),
  base_url: z.string(),
  hmac_secret: z.string(),
});

type FormValues = z.infer<typeof schema>;

function chatwootWarningText(t: (k: string) => string, w: string): string {
  if (w.startsWith("hmac_secret_missing")) {
    return t(
      "HMAC secret is not set: user identity verification is disabled. Set the inbox HMAC secret and enable identity validation in Chatwoot.",
    );
  }
  return w;
}

export function ChatwootSettings() {
  const { t } = useTranslation();
  const backupContactTypeText = (type: string | undefined | null) => {
    if (!type) return "—";
    const k = backupContactTypeLabelKey(type);
    return k ? t(k) : type;
  };
  const chatwootQuery = useChatwootConfig();
  const putChatwoot = usePutChatwootConfig();
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN,
  );
  const [confirmOpen, setConfirmOpen] = useState(false);
  const pendingPutRef = useRef<ChatwootPutInput | null>(null);

  const formDefaults = useMemo(
    () =>
      chatwootQuery.data
        ? configToForm(chatwootQuery.data)
        : CHATWOOT_FORM_DEFAULTS,
    [chatwootQuery.data],
  );

  const initialRef = useRef<ChatwootFormValues>(formDefaults);
  initialRef.current = formDefaults;

  const form = useForm<FormValues>({
    resolver: zodResolver(schema) as unknown as Resolver<FormValues>,
    defaultValues: formDefaults,
  });

  useEffect(() => {
    form.reset(formDefaults);
  }, [formDefaults, form]);

  const { isDirty, isSubmitting } = form.formState;
  const saving = putChatwoot.isPending || isSubmitting;

  async function doPut(body: ChatwootPutInput) {
    try {
      const res = await putChatwoot.mutateAsync(body);
      if (!res.success) {
        if (res.code === "forbidden" || res.code === "confirm_required") {
          toast.error(
            res.code === "forbidden"
              ? t("Only root can update Chatwoot settings")
              : getServerErrorMessage(res, t("Save failed")),
          );
          return;
        }
        toast.error(getServerErrorMessage(res, t("Save failed")));
        return;
      }
      toast.success(t("Saved"));
      form.reset({
        script: res.data?.script ?? body.script ?? form.getValues("script"),
        website_token:
          res.data?.website_token ??
          body.website_token ??
          form.getValues("website_token"),
        base_url:
          res.data?.base_url ?? body.base_url ?? form.getValues("base_url"),
        hmac_secret: "",
      });
      void chatwootQuery.refetch();
    } catch (err) {
      const status = (err as { status?: number })?.status;
      if (status === 403) {
        toast.error(t("Only root can update Chatwoot settings"));
        return;
      }
      toast.error(getServerErrorMessage(err, t("Save failed")));
    }
  }

  async function onSubmit(values: FormValues) {
    if (!isRoot) {
      toast.error(t("Only root can update Chatwoot settings"));
      return;
    }
    const body = buildChatwootPut(values, initialRef.current);
    if (!body) {
      toast.info(t("No changes to save"));
      return;
    }
    if (chatwootPutNeedsConfirm(body)) {
      pendingPutRef.current = body;
      setConfirmOpen(true);
      return;
    }
    await doPut({ ...body, confirm: false });
  }

  async function onConfirmSave() {
    const body = pendingPutRef.current;
    pendingPutRef.current = null;
    setConfirmOpen(false);
    if (!body) return;
    await doPut({ ...body, confirm: true });
  }

  const isForbidden =
    (chatwootQuery.error as { status?: number } | null)?.status === 403;

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t("Chatwoot customer support")}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-8"
            onClick={() => void chatwootQuery.refetch()}
            disabled={chatwootQuery.isFetching}
          >
            <RefreshCw className="size-3.5" />
            {t("Refresh")}
          </Button>
          <Button
            type="button"
            size="sm"
            className="h-8"
            disabled={!isDirty || saving || !isRoot}
            onClick={() => void form.handleSubmit(onSubmit)()}
          >
            <Save className="size-3.5" />
            {saving ? t("Saving...") : t("Save Changes")}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className="flex h-full min-h-0 w-full flex-col gap-4">
            <p className="text-muted-foreground text-sm">
              {t(
                "Visitors load the Chatwoot SDK from the base URL and website token. The script field is not injected. HMAC is write-only; GET only returns secret_set and fingerprint. Backup contact is read-only here.",
              )}
            </p>

            {(chatwootQuery.data?.warnings ?? []).map((w) => (
              <div
                key={w}
                role="alert"
                className="flex gap-2.5 rounded-lg border border-amber-200 bg-amber-50 px-3.5 py-2.5 text-[13px] leading-5 text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/30 dark:text-amber-100"
              >
                <ShieldAlert className="mt-0.5 size-4 shrink-0" />
                <span>{chatwootWarningText(t, w)}</span>
              </div>
            ))}

            <div className="grid gap-3 md:grid-cols-2">
              <div className="flex gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100">
                <Info className="mt-0.5 size-4 shrink-0" />
                <div>
                  {t(
                    "Backup contact is owned by Business parameters. This page only shows it read-only.",
                  )}{" "}
                  <Link
                    to="/system-settings/business-params"
                    className="font-medium underline underline-offset-2"
                  >
                    {t("Open business parameters")}
                  </Link>
                </div>
              </div>
              <div className="flex gap-2.5 rounded-lg border border-amber-200 bg-amber-50 px-3.5 py-2.5 text-[13px] leading-5 text-amber-950 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-100">
                <ShieldAlert className="mt-0.5 size-4 shrink-0" />
                <div>
                  {t(
                    "Changing script or HMAC secret requires Root confirmation.",
                  )}
                </div>
              </div>
            </div>

            {!isRoot && chatwootQuery.data ? (
              <p className="text-muted-foreground text-xs">
                {t("Only root can update Chatwoot settings")}
              </p>
            ) : null}

            {chatwootQuery.isLoading ? (
              <div className="text-muted-foreground flex min-h-40 items-center justify-center text-sm">
                {t("Loading settings...")}
              </div>
            ) : null}

            {isForbidden ? (
              <ErrorState
                title={t("Insufficient permissions")}
                description={t(
                  "Admin permission is required to manage Chatwoot settings.",
                )}
              />
            ) : null}

            {chatwootQuery.isError && !chatwootQuery.data && !isForbidden ? (
              <ErrorState
                title={t("Failed to load settings")}
                onRetry={() => void chatwootQuery.refetch()}
              />
            ) : null}

            {chatwootQuery.data ? (
              <Form {...form}>
                <SettingsForm
                  onSubmit={form.handleSubmit(onSubmit)}
                  autoComplete="off"
                  className="bg-card space-y-6 rounded-xl border p-5 shadow-xs"
                >
                  <FormField
                    control={form.control}
                    name="script"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t("Chatwoot script")}</FormLabel>
                        <FormControl>
                          <Textarea
                            rows={6}
                            autoComplete="off"
                            placeholder="<script>...</script>"
                            disabled={saving || !isRoot}
                            className="font-mono text-xs"
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            "Not injected into the visitor page. Used only as a health-probe URL fallback when base URL is empty. Changing it requires confirmation.",
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <div className="grid gap-4 md:grid-cols-2">
                    <FormField
                      control={form.control}
                      name="website_token"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t("Website token")}</FormLabel>
                          <FormControl>
                            <Input
                              autoComplete="off"
                              disabled={saving || !isRoot}
                              {...field}
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              "Public Chatwoot website token (readable on GET).",
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name="base_url"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t("Chatwoot base URL")}</FormLabel>
                          <FormControl>
                            <Input
                              autoComplete="off"
                              placeholder="https://app.chatwoot.com"
                              disabled={saving || !isRoot}
                              {...field}
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              "Visitor widget loads the Chatwoot SDK from this https URL. Empty disables the widget.",
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>

                  <FormField
                    control={form.control}
                    name="hmac_secret"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t("HMAC secret (leave blank unless updating)")}
                        </FormLabel>
                        <div className="mb-2 flex flex-wrap items-center gap-2 text-xs">
                          {chatwootQuery.data.hmac_secret_set ? (
                            <Badge variant="secondary">{t("Secret set")}</Badge>
                          ) : (
                            <Badge variant="outline">
                              {t("Secret missing")}
                            </Badge>
                          )}
                          <span className="text-muted-foreground font-mono">
                            {t("Fingerprint")}:{" "}
                            {formatHmacFingerprint(
                              chatwootQuery.data.hmac_secret_fingerprint,
                            )}
                          </span>
                        </div>
                        <FormControl>
                          <Input
                            type="password"
                            autoComplete="new-password"
                            placeholder={t(
                              "Leave blank to keep current secret",
                            )}
                            disabled={saving || !isRoot}
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            "Never re-displayed after save; only fingerprint is shown. Empty value does not overwrite. Setting a new secret requires confirmation.",
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <div className="space-y-3">
                    <div>
                      <h3 className="text-sm font-medium">
                        {t("Backup contact (read-only)")}
                      </h3>
                      <p className="text-muted-foreground text-xs">
                        {/* 后端 backup_contact_note 是英文开发说明，按总管定不显示 */}
                        {t(
                          "Backup contact lives in business parameters; shown here for reference.",
                        )}
                      </p>
                    </div>
                    <div className="bg-muted/40 grid gap-3 rounded-lg border px-3.5 py-3 text-sm md:grid-cols-3">
                      <div>
                        <div className="text-muted-foreground text-xs">
                          {t("Contact type")}
                        </div>
                        <div className="text-xs">
                          {backupContactTypeText(
                            chatwootQuery.data.backup_contact_type,
                          )}
                        </div>
                      </div>
                      <div className="md:col-span-2">
                        <div className="text-muted-foreground text-xs">
                          {t("Contact")}
                        </div>
                        <div className="font-mono text-xs break-all">
                          {chatwootQuery.data.backup_contact || "—"}
                        </div>
                      </div>
                    </div>
                    <Link
                      to="/system-settings/business-params"
                      className="text-primary text-xs font-medium underline underline-offset-2"
                    >
                      {t("Open business parameters")}
                    </Link>
                  </div>
                </SettingsForm>
              </Form>
            ) : null}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={(open) => {
          setConfirmOpen(open);
          if (!open) pendingPutRef.current = null;
        }}
        title={t("Confirm Chatwoot sensitive save")}
        desc={t(
          "You are changing the Chatwoot script and/or HMAC secret. Continue?",
        )}
        confirmText={t("Confirm and save")}
        destructive
        isLoading={saving}
        handleConfirm={() => void onConfirmSave()}
      />
    </>
  );
}
