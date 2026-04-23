"use client";

import { useEffect, useState } from "react";
import { LoaderCircle, MessageSquareText, Save } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { fetchChatCompletionsSettings, updateChatCompletionsSettings } from "@/lib/api";

export function ChatCompletionsSettingsCard() {
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [enabled, setEnabled] = useState(true);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const data = await fetchChatCompletionsSettings();
        if (!cancelled) {
          setEnabled(Boolean(data.enabled));
        }
      } catch (error) {
        if (!cancelled) {
          toast.error(error instanceof Error ? error.message : "加载接口开关失败");
        }
      } finally {
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const handleSave = async () => {
    setIsSaving(true);
    try {
      const data = await updateChatCompletionsSettings(enabled);
      setEnabled(Boolean(data.enabled));
      toast.success("接口开关已保存");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存接口开关失败");
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <Card className="rounded-3xl border border-stone-200/70 bg-white/92 shadow-[0_24px_60px_-42px_rgba(15,23,42,0.35)]">
      <CardContent className="space-y-6 p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex size-10 items-center justify-center rounded-2xl bg-stone-100">
              <MessageSquareText className="size-5 text-stone-600" />
            </div>
            <div>
              <h2 className="text-lg font-semibold tracking-tight text-stone-900">聊天补全接口</h2>
              <p className="text-sm text-stone-500">控制 `/v1/chat/completions` 是否可用，适合临时关闭对应调用入口。</p>
            </div>
          </div>
          <Badge variant={enabled ? "success" : "secondary"} className="w-fit rounded-md px-2.5 py-1">
            {enabled ? "已启用" : "已关闭"}
          </Badge>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-10">
            <LoaderCircle className="size-5 animate-spin text-stone-400" />
          </div>
        ) : (
          <>
            <label className="flex items-center justify-between rounded-2xl border border-stone-200 bg-stone-50/70 px-4 py-4">
              <div className="space-y-1 pr-4">
                <div className="text-sm font-medium text-stone-800">启用 `/v1/chat/completions`</div>
                <p className="text-sm text-stone-500">关闭后，该接口会直接返回禁用错误；`/v1/responses` 不受影响。</p>
              </div>
              <Checkbox checked={enabled} onCheckedChange={(checked) => setEnabled(checked === true)} />
            </label>

            <div className="flex justify-end">
              <Button
                className="h-10 rounded-xl bg-stone-950 px-5 text-white hover:bg-stone-800"
                onClick={() => void handleSave()}
                disabled={isSaving}
              >
                {isSaving ? <LoaderCircle className="size-4 animate-spin" /> : <Save className="size-4" />}
                保存配置
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
