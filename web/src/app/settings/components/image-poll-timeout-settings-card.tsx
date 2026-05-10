"use client";

import { useEffect, useState } from "react";
import { Clock3, LoaderCircle, Save } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { fetchImagePollTimeoutSettings, updateImagePollTimeoutSettings } from "@/lib/api";

export function ImagePollTimeoutSettingsCard() {
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [seconds, setSeconds] = useState("");
  const [savedSeconds, setSavedSeconds] = useState(180);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const data = await fetchImagePollTimeoutSettings();
        if (!cancelled) {
          const value = Math.max(1, Number(data.seconds) || 180);
          setSeconds(String(value));
          setSavedSeconds(value);
        }
      } catch (error) {
        if (!cancelled) {
          toast.error(error instanceof Error ? error.message : "加载图片轮询超时失败");
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
    const value = Math.max(0, Number(seconds) || 0);
    if (value <= 0) {
      toast.error("请输入大于 0 的秒数");
      return;
    }

    setIsSaving(true);
    try {
      const data = await updateImagePollTimeoutSettings(value);
      const normalized = Math.max(1, Number(data.seconds) || value);
      setSeconds(String(normalized));
      setSavedSeconds(normalized);
      toast.success("图片轮询超时已保存");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存图片轮询超时失败");
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
              <Clock3 className="size-5 text-stone-600" />
            </div>
            <div>
              <h2 className="text-lg font-semibold tracking-tight text-stone-900">图片轮询超时</h2>
              <p className="text-sm text-stone-500">控制等待上游图片结果的最长时间，单位秒。</p>
            </div>
          </div>
          <Badge variant="info" className="w-fit rounded-md px-2.5 py-1">
            当前 {savedSeconds}s
          </Badge>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-10">
            <LoaderCircle className="size-5 animate-spin text-stone-400" />
          </div>
        ) : (
          <>
            <div className="space-y-2">
              <label className="text-sm font-medium text-stone-700">超时时间（秒）</label>
              <Input
                value={seconds}
                onChange={(event) => setSeconds(event.target.value)}
                placeholder="180"
                inputMode="numeric"
                className="h-11 rounded-xl border-stone-200 bg-white"
              />
              <p className="text-sm text-stone-500">用于 `/v1/images/generations` 和 `/v1/images/edits` 的等待上限。</p>
            </div>

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
