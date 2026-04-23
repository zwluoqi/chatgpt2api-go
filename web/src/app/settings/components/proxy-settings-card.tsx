"use client";

import { useEffect, useState } from "react";
import { Link2, LoaderCircle, PlugZap, Save } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { fetchProxy, testProxy, updateProxy, type ProxyTestResult } from "@/lib/api";

export function ProxySettingsCard() {
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [proxyURL, setProxyURL] = useState("");
  const [testResult, setTestResult] = useState<ProxyTestResult | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const data = await fetchProxy();
        if (!cancelled) {
          setProxyURL(data.proxy.url || "");
        }
      } catch (error) {
        if (!cancelled) {
          toast.error(error instanceof Error ? error.message : "加载代理设置失败");
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

  const handleTest = async () => {
    const candidate = proxyURL.trim();
    if (!candidate) {
      toast.error("请先填写代理地址");
      return;
    }
    setIsTesting(true);
    setTestResult(null);
    try {
      const data = await testProxy(candidate);
      setTestResult(data.result);
      if (data.result.ok) {
        toast.success(`代理可用（${data.result.latency_ms} ms，HTTP ${data.result.status}）`);
      } else {
        toast.error(`代理不可用：${data.result.error ?? "未知错误"}`);
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "测试代理失败");
    } finally {
      setIsTesting(false);
    }
  };

  const handleSave = async () => {
    setIsSaving(true);
    try {
      const data = await updateProxy({
        enabled: proxyURL.trim() !== "",
        url: proxyURL.trim(),
      });
      setProxyURL(data.proxy.url || "");
      toast.success("代理设置已保存");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存代理设置失败");
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
              <Link2 className="size-5 text-stone-600" />
            </div>
            <div>
              <h2 className="text-lg font-semibold tracking-tight text-stone-900">上游代理</h2>
              <p className="text-sm text-stone-500">为请求 chatgpt.com 的出站流量配置代理，保存后立即生效。</p>
            </div>
          </div>
          <Badge variant={proxyURL.trim() ? "success" : "secondary"} className="w-fit rounded-md px-2.5 py-1">
            {proxyURL.trim() ? "已配置" : "未配置"}
          </Badge>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-10">
            <LoaderCircle className="size-5 animate-spin text-stone-400" />
          </div>
        ) : (
          <>
            <div className="space-y-2">
              <label className="text-sm font-medium text-stone-700">代理地址</label>
              <Input
                value={proxyURL}
                onChange={(event) => {
                  setProxyURL(event.target.value);
                  setTestResult(null);
                }}
                placeholder="http://user:pass@127.0.0.1:7890"
                className="h-11 rounded-xl border-stone-200 bg-white"
              />
              <p className="text-sm text-stone-500">
                留空表示不使用代理。支持 `http`、`https`、`socks4`、`socks4a`、`socks5`、`socks5h`。
              </p>
            </div>

            {testResult ? (
              <div
                className={`rounded-xl border px-4 py-3 text-sm leading-6 ${
                  testResult.ok
                    ? "border-emerald-200 bg-emerald-50 text-emerald-800"
                    : "border-rose-200 bg-rose-50 text-rose-800"
                }`}
              >
                {testResult.ok
                  ? `代理可用：HTTP ${testResult.status}，用时 ${testResult.latency_ms} ms`
                  : `代理不可用：${testResult.error ?? "未知错误"}（用时 ${testResult.latency_ms} ms）`}
              </div>
            ) : null}

            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                className="h-10 rounded-xl border-stone-200 bg-white px-5 text-stone-700"
                onClick={() => void handleTest()}
                disabled={isTesting || isLoading}
              >
                {isTesting ? <LoaderCircle className="size-4 animate-spin" /> : <PlugZap className="size-4" />}
                测试代理
              </Button>
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
