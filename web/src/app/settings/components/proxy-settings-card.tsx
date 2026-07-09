"use client";

import { useEffect, useState } from "react";
import { Link2, LoaderCircle, PlugZap, Save } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { fetchProxy, testProxy, updateProxy, type ProxyTestResult } from "@/lib/api";

type LineTest = { url: string; result: ProxyTestResult };

function parseLines(text: string): string[] {
  return text
    .split(/[\n,;]+/)
    .map((line) => line.trim())
    .filter(Boolean);
}

export function ProxySettingsCard() {
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [proxyText, setProxyText] = useState("");
  const [testResults, setTestResults] = useState<LineTest[]>([]);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const data = await fetchProxy();
        if (!cancelled) {
          const urls = data.proxy.urls?.length ? data.proxy.urls : data.proxy.url ? [data.proxy.url] : [];
          setProxyText(urls.join("\n"));
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

  const urls = parseLines(proxyText);

  const handleTest = async () => {
    if (urls.length === 0) {
      toast.error("请先填写代理地址");
      return;
    }
    setIsTesting(true);
    setTestResults([]);
    try {
      const results = await Promise.all(
        urls.map(async (url) => ({ url, result: (await testProxy(url)).result })),
      );
      setTestResults(results);
      const okCount = results.filter((r) => r.result.ok).length;
      if (okCount === results.length) {
        toast.success(`全部可用（${okCount}/${results.length}）`);
      } else {
        toast.error(`可用 ${okCount}/${results.length}，其余不可用`);
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
      const data = await updateProxy({ enabled: urls.length > 0, urls });
      const saved = data.proxy.urls?.length ? data.proxy.urls : data.proxy.url ? [data.proxy.url] : [];
      setProxyText(saved.join("\n"));
      toast.success(`代理设置已保存（${saved.length} 条）`);
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
              <p className="text-sm text-stone-500">为请求 chatgpt.com 的出站流量配置代理，可填多条按轮询使用，保存后立即生效。</p>
            </div>
          </div>
          <Badge variant={urls.length ? "success" : "secondary"} className="w-fit rounded-md px-2.5 py-1">
            {urls.length ? `已配置 ${urls.length} 条` : "未配置"}
          </Badge>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-10">
            <LoaderCircle className="size-5 animate-spin text-stone-400" />
          </div>
        ) : (
          <>
            <div className="space-y-2">
              <label className="text-sm font-medium text-stone-700">代理地址（每行一条）</label>
              <Textarea
                value={proxyText}
                onChange={(event) => {
                  setProxyText(event.target.value);
                  setTestResults([]);
                }}
                placeholder={"http://user:pass@127.0.0.1:7890\nsocks5://127.0.0.1:1080"}
                rows={5}
                className="rounded-xl border-stone-200 bg-white font-mono text-sm"
              />
              <p className="text-sm text-stone-500">
                每行一条，多条按轮询分摊出站流量。留空表示不使用代理。支持 `http`、`https`、`socks4`、`socks4a`、`socks5`、`socks5h`。
              </p>
            </div>

            {testResults.length > 0 ? (
              <div className="space-y-1.5">
                {testResults.map((t, index) => (
                  <div
                    key={`${index}-${t.url}`}
                    className={`flex items-center justify-between gap-3 rounded-xl border px-4 py-2.5 text-sm leading-6 ${
                      t.result.ok
                        ? "border-emerald-200 bg-emerald-50 text-emerald-800"
                        : "border-rose-200 bg-rose-50 text-rose-800"
                    }`}
                  >
                    <span className="truncate font-mono text-xs">{t.url}</span>
                    <span className="shrink-0">
                      {t.result.ok
                        ? `可用 · HTTP ${t.result.status} · ${t.result.latency_ms}ms`
                        : `不可用 · ${t.result.error ?? "未知"} · ${t.result.latency_ms}ms`}
                    </span>
                  </div>
                ))}
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
                测试全部
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
