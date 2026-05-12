"use client";

import { useDeferredValue, useEffect, useMemo, useRef, useState } from "react";
import {
  CalendarRange,
  ChevronDown,
  ChevronUp,
  Copy,
  Image as ImageIcon,
  LoaderCircle,
  RefreshCw,
  Search,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

import { ImageLightbox } from "@/components/image-lightbox";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { deleteLogs, fetchLogs, type LogEntry, type LogImage } from "@/lib/api";
import { cn } from "@/lib/utils";
import webConfig from "@/constants/common-env";
import { getStoredAuthKey } from "@/store/auth";

const typeOptions = [
  { label: "全部类型", value: "all" },
  { label: "调用日志", value: "call" },
] as const;

const limitOptions = ["50", "100", "200"] as const;

const knownDetailKeys = new Set([
  "endpoint",
  "model",
  "started_at",
  "ended_at",
  "duration_ms",
  "status",
  "request_text",
  "account_token_prefix",
  "input_images",
  "output_images",
  "error",
  "end_reason",
  "end_reason_label",
  "upstream_text",
]);

function getString(detail: Record<string, unknown>, key: string) {
  const value = detail[key];
  return typeof value === "string" ? value : "";
}

function getNumber(detail: Record<string, unknown>, key: string) {
  const value = detail[key];
  return typeof value === "number" ? value : null;
}

function getImages(detail: Record<string, unknown>, key: string): LogImage[] {
  const value = detail[key];
  if (!Array.isArray(value)) {
    return [];
  }
  return value.flatMap((item) => {
    if (!item || typeof item !== "object") {
      return [];
    }
    const record = item as Record<string, unknown>;
    const b64 = typeof record.b64 === "string" ? record.b64.trim() : "";
    const file = typeof record.file === "string" ? record.file.trim() : "";
    if (!b64 && !file) {
      return [];
    }
    return [
      {
        mime: typeof record.mime === "string" && record.mime.trim() ? record.mime : "image/png",
        name: typeof record.name === "string" ? record.name : "",
        b64,
        file,
      },
    ];
  });
}

function buildImageSrc(logId: string, image: LogImage, authKey: string) {
  if (image.file) {
    const baseUrl = webConfig.apiUrl.replace(/\/$/, "");
    const search = authKey ? `?auth_key=${encodeURIComponent(authKey)}` : "";
    return `${baseUrl}/api/logs/assets/${encodeURIComponent(logId)}/${encodeURIComponent(image.file)}${search}`;
  }
  return `data:${image.mime || "image/png"};base64,${image.b64 || ""}`;
}

function formatDuration(value: number | null) {
  if (value === null || value < 0) {
    return "—";
  }
  if (value < 1000) {
    return `${value} ms`;
  }
  return `${(value / 1000).toFixed(2)} s`;
}

function getStatusMeta(status: string) {
  if (status === "success") {
    return { label: "成功", variant: "success" as const };
  }
  if (status === "failed") {
    return { label: "失败", variant: "danger" as const };
  }
  return { label: status || "未知", variant: "secondary" as const };
}

function buildSearchText(item: LogEntry) {
  const detail = item.detail || {};
  return [
    item.summary,
    item.type,
    item.time,
    getString(detail, "endpoint"),
    getString(detail, "model"),
    getString(detail, "request_text"),
    getString(detail, "error"),
    getString(detail, "end_reason"),
    getString(detail, "end_reason_label"),
    getString(detail, "upstream_text"),
    getString(detail, "account_token_prefix"),
  ]
    .join(" ")
    .toLowerCase();
}

function getExtraDetail(detail: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(detail).filter(([key]) => !knownDetailKeys.has(key)));
}

export default function LogsPage() {
  const requestIdRef = useRef(0);

  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [expandedIds, setExpandedIds] = useState<string[]>([]);
  const [query, setQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState<(typeof typeOptions)[number]["value"]>("all");
  const [startDate, setStartDate] = useState("");
  const [endDate, setEndDate] = useState("");
  const [limit, setLimit] = useState<(typeof limitOptions)[number]>("100");
  const [isLoading, setIsLoading] = useState(true);
  const [isDeleting, setIsDeleting] = useState(false);
  const [lightboxOpen, setLightboxOpen] = useState(false);
  const [lightboxIndex, setLightboxIndex] = useState(0);
  const [lightboxImages, setLightboxImages] = useState<Array<{ id: string; src: string }>>([]);
  const [assetAuthKey, setAssetAuthKey] = useState("");

  const deferredQuery = useDeferredValue(query);
  const hasInvalidDateRange = Boolean(startDate && endDate && startDate > endDate);

  const loadLogs = async (silent = false) => {
    if (hasInvalidDateRange) {
      return;
    }
    const requestId = requestIdRef.current + 1;
    requestIdRef.current = requestId;

    if (!silent) {
      setIsLoading(true);
    }
    try {
      const data = await fetchLogs({
        type: typeFilter === "all" ? undefined : typeFilter,
        startDate: startDate || undefined,
        endDate: endDate || undefined,
        limit: Number(limit),
      });
      if (requestId !== requestIdRef.current) {
        return;
      }
      setLogs(data.items);
      setSelectedIds((prev) => prev.filter((id) => data.items.some((item) => item.id === id)));
      setExpandedIds((prev) => prev.filter((id) => data.items.some((item) => item.id === id)));
    } catch (error) {
      if (requestId !== requestIdRef.current) {
        return;
      }
      toast.error(error instanceof Error ? error.message : "加载日志失败");
    } finally {
      if (!silent && requestId === requestIdRef.current) {
        setIsLoading(false);
      }
    }
  };

  useEffect(() => {
    if (hasInvalidDateRange) {
      return;
    }
    void loadLogs();
  }, [typeFilter, startDate, endDate, limit]);

  useEffect(() => {
    void getStoredAuthKey().then((value) => setAssetAuthKey(value));
  }, []);

  const filteredLogs = useMemo(() => {
    const normalizedQuery = deferredQuery.trim().toLowerCase();
    if (!normalizedQuery) {
      return logs;
    }
    return logs.filter((item) => buildSearchText(item).includes(normalizedQuery));
  }, [deferredQuery, logs]);

  const stats = useMemo(() => {
    return filteredLogs.reduce(
      (acc, item) => {
        const detail = item.detail || {};
        const status = getString(detail, "status");
        const imageCount = getImages(detail, "input_images").length + getImages(detail, "output_images").length;
        acc.total += 1;
        if (status === "success") {
          acc.success += 1;
        }
        if (status === "failed") {
          acc.failed += 1;
        }
        if (imageCount > 0) {
          acc.withImages += 1;
        }
        return acc;
      },
      { total: 0, success: 0, failed: 0, withImages: 0 },
    );
  }, [filteredLogs]);

  const allVisibleSelected = filteredLogs.length > 0 && filteredLogs.every((item) => selectedIds.includes(item.id));

  const handleToggleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedIds(filteredLogs.map((item) => item.id));
      return;
    }
    setSelectedIds([]);
  };

  const handleToggleSelected = (id: string, checked: boolean) => {
    setSelectedIds((prev) => {
      if (checked) {
        return prev.includes(id) ? prev : [...prev, id];
      }
      return prev.filter((item) => item !== id);
    });
  };

  const toggleExpanded = (id: string) => {
    setExpandedIds((prev) => (prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id]));
  };

  const handleDelete = async (ids: string[]) => {
    const normalizedIds = Array.from(new Set(ids.filter(Boolean)));
    if (normalizedIds.length === 0) {
      return;
    }
    setIsDeleting(true);
    try {
      const data = await deleteLogs(normalizedIds);
      setLogs((prev) => prev.filter((item) => !normalizedIds.includes(item.id)));
      setSelectedIds((prev) => prev.filter((id) => !normalizedIds.includes(id)));
      setExpandedIds((prev) => prev.filter((id) => !normalizedIds.includes(id)));
      toast.success(`已删除 ${data.removed} 条日志`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除日志失败");
    } finally {
      setIsDeleting(false);
    }
  };

  const openLightbox = (logId: string, images: LogImage[], startIndex: number) => {
    const prepared = images.map((image, index) => ({
      id: `${index}-${image.name || "image"}`,
      src: buildImageSrc(logId, image, assetAuthKey),
    }));
    setLightboxImages(prepared);
    setLightboxIndex(startIndex);
    setLightboxOpen(true);
  };

  return (
    <>
      <div className="space-y-5">
        <section className="grid gap-4 lg:grid-cols-[1.35fr_0.65fr]">
          <Card className="border-stone-200/70 bg-white/80 backdrop-blur">
            <CardContent className="flex h-full flex-col justify-between gap-5 p-6">
              <div className="space-y-2">
                <div className="inline-flex items-center gap-2 rounded-full border border-stone-200 bg-stone-50 px-3 py-1 text-xs font-medium text-stone-600">
                  <CalendarRange className="size-3.5" />
                  调用日志
                </div>
                <div>
                  <h1 className="text-2xl font-semibold tracking-tight text-stone-950">功能日志</h1>
                  <p className="mt-2 max-w-2xl text-sm leading-6 text-stone-500">
                    查看最近接口调用记录，支持按类型、日期范围筛选，并展开查看请求文本、错误信息与输入输出图片。
                  </p>
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-3">
                <Button
                  className="rounded-xl bg-stone-950 px-4 text-white hover:bg-stone-800"
                  onClick={() => void loadLogs()}
                  disabled={isLoading || hasInvalidDateRange}
                >
                  {isLoading ? <LoaderCircle className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                  刷新日志
                </Button>
                <Button
                  variant="outline"
                  className="rounded-xl"
                  onClick={() => void handleDelete(selectedIds)}
                  disabled={isDeleting || selectedIds.length === 0}
                >
                  {isDeleting ? <LoaderCircle className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
                  删除选中
                </Button>
              </div>
            </CardContent>
          </Card>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">
            {[
              { label: "当前结果", value: stats.total, accent: "text-stone-950" },
              { label: "成功调用", value: stats.success, accent: "text-emerald-600" },
              { label: "失败调用", value: stats.failed, accent: "text-rose-600" },
              { label: "含图片记录", value: stats.withImages, accent: "text-sky-600" },
            ].map((item) => (
              <Card key={item.label} className="border-white/70 bg-white/80">
                <CardContent className="p-5">
                  <div className="text-sm text-stone-500">{item.label}</div>
                  <div className={cn("mt-3 text-3xl font-semibold tracking-tight", item.accent)}>{item.value}</div>
                </CardContent>
              </Card>
            ))}
          </div>
        </section>

        <Card className="border-white/70 bg-white/85">
          <CardContent className="space-y-4 p-6">
            <div className="grid gap-3 lg:grid-cols-[1.4fr_0.8fr_0.8fr_0.7fr]">
              <div className="relative">
                <Search className="pointer-events-none absolute top-1/2 left-4 size-4 -translate-y-1/2 text-stone-400" />
                <Input
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="搜索摘要、模型、接口、错误、请求文本"
                  className="pl-11"
                />
              </div>
              <Select value={typeFilter} onValueChange={(value) => setTypeFilter(value as (typeof typeOptions)[number]["value"])}>
                <SelectTrigger className="h-11 rounded-2xl bg-white/90">
                  <SelectValue placeholder="全部类型" />
                </SelectTrigger>
                <SelectContent>
                  {typeOptions.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Input type="date" value={startDate} onChange={(event) => setStartDate(event.target.value)} />
              <Input type="date" value={endDate} onChange={(event) => setEndDate(event.target.value)} />
            </div>

            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex flex-wrap items-center gap-3 text-sm text-stone-500">
                <label className="inline-flex items-center gap-2">
                  <Checkbox checked={allVisibleSelected} onCheckedChange={(checked) => handleToggleSelectAll(checked === true)} />
                  全选当前结果
                </label>
                <span>已选 {selectedIds.length} 条</span>
                {hasInvalidDateRange ? <span className="text-rose-600">结束日期不能早于开始日期</span> : null}
              </div>
              <div className="flex items-center gap-3">
                <span className="text-sm text-stone-500">服务端加载上限</span>
                <Select value={limit} onValueChange={(value) => setLimit(value as (typeof limitOptions)[number])}>
                  <SelectTrigger className="h-10 w-28 rounded-2xl bg-white/90">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {limitOptions.map((item) => (
                      <SelectItem key={item} value={item}>
                        {item} 条
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          </CardContent>
        </Card>

        <div className="space-y-4">
          {isLoading ? (
            <Card className="border-dashed border-stone-300/80 bg-white/60">
              <CardContent className="flex min-h-52 items-center justify-center gap-3 p-6 text-stone-500">
                <LoaderCircle className="size-5 animate-spin" />
                正在加载日志
              </CardContent>
            </Card>
          ) : filteredLogs.length === 0 ? (
            <Card className="border-dashed border-stone-300/80 bg-white/60">
              <CardContent className="flex min-h-52 items-center justify-center p-6 text-center text-sm leading-6 text-stone-500">
                当前没有匹配的日志记录。
              </CardContent>
            </Card>
          ) : (
            filteredLogs.map((item) => {
              const detail = item.detail || {};
              const status = getStatusMeta(getString(detail, "status"));
              const endpoint = getString(detail, "endpoint");
              const model = getString(detail, "model");
              const duration = getNumber(detail, "duration_ms");
              const requestText = getString(detail, "request_text");
              const errorText = getString(detail, "error");
              const endReason = getString(detail, "end_reason");
              const endReasonLabel = getString(detail, "end_reason_label");
              const upstreamText = getString(detail, "upstream_text");
              const tokenPrefix = getString(detail, "account_token_prefix");
              const inputImages = getImages(detail, "input_images");
              const outputImages = getImages(detail, "output_images");
              const allImages = [...inputImages, ...outputImages];
              const extraDetail = getExtraDetail(detail);
              const expanded = expandedIds.includes(item.id);

              return (
                <Card key={item.id} className="overflow-hidden border-white/70 bg-white/85">
                  <CardContent className="p-0">
                    <div className="flex flex-col gap-4 p-5">
                      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
                        <div className="flex items-start gap-3">
                          <Checkbox
                            className="mt-1"
                            checked={selectedIds.includes(item.id)}
                            onCheckedChange={(checked) => handleToggleSelected(item.id, checked === true)}
                          />
                          <div className="space-y-3">
                            <div className="flex flex-wrap items-center gap-2">
                              <Badge variant={status.variant}>{status.label}</Badge>
                              <Badge variant="info">{item.type}</Badge>
                              {allImages.length > 0 ? (
                                <Badge variant="violet" className="gap-1">
                                  <ImageIcon className="size-3.5" />
                                  {allImages.length} 张图片
                                </Badge>
                              ) : null}
                              <span className="text-xs text-stone-400">{item.time}</span>
                            </div>
                            <div>
                              <div className="text-base font-semibold tracking-tight text-stone-950">{item.summary}</div>
                              <div className="mt-2 flex flex-wrap gap-x-5 gap-y-1 text-sm text-stone-500">
                                <span>接口：{endpoint || "—"}</span>
                                <span>模型：{model || "—"}</span>
                                <span>耗时：{formatDuration(duration)}</span>
                                <span>Token 前缀：{tokenPrefix || "—"}</span>
                                <span>结束原因：{endReasonLabel || endReason || "—"}</span>
                              </div>
                            </div>
                          </div>
                        </div>

                        <div className="flex flex-wrap items-center gap-2">
                          {requestText ? (
                            <Button
                              variant="outline"
                              size="sm"
                              className="rounded-xl"
                              onClick={async () => {
                                try {
                                  await navigator.clipboard.writeText(requestText);
                                  toast.success("请求文本已复制");
                                } catch {
                                  toast.error("复制失败");
                                }
                              }}
                            >
                              <Copy className="size-4" />
                              复制请求
                            </Button>
                          ) : null}
                          <Button
                            variant="outline"
                            size="sm"
                            className="rounded-xl"
                            onClick={() => toggleExpanded(item.id)}
                          >
                            {expanded ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
                            {expanded ? "收起详情" : "展开详情"}
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            className="rounded-xl text-rose-600 hover:text-rose-700"
                            onClick={() => void handleDelete([item.id])}
                            disabled={isDeleting}
                          >
                            <Trash2 className="size-4" />
                            删除
                          </Button>
                        </div>
                      </div>

                      {errorText ? (
                        <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm leading-6 text-rose-700">
                          {errorText}
                        </div>
                      ) : null}
                    </div>

                    {expanded ? (
                      <div className="border-t border-stone-100 bg-stone-50/70 px-5 py-5">
                        <div className="grid gap-5 xl:grid-cols-2">
                          <div className="space-y-5">
                            <section className="space-y-2">
                              <div className="text-sm font-medium text-stone-700">请求文本</div>
                              <div className="rounded-2xl border border-stone-200 bg-white px-4 py-3 text-sm leading-6 whitespace-pre-wrap text-stone-700">
                                {requestText || "—"}
                              </div>
                            </section>

                            {upstreamText ? (
                              <section className="space-y-2">
                                <div className="flex items-center justify-between gap-3">
                                  <div className="text-sm font-medium text-stone-700">上游返回文本</div>
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    className="rounded-xl"
                                    onClick={async () => {
                                      try {
                                        await navigator.clipboard.writeText(upstreamText);
                                        toast.success("上游文本已复制");
                                      } catch {
                                        toast.error("复制失败");
                                      }
                                    }}
                                  >
                                    <Copy className="size-4" />
                                    复制文本
                                  </Button>
                                </div>
                                <div className="max-h-96 overflow-auto rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 whitespace-pre-wrap text-amber-900">
                                  {upstreamText}
                                </div>
                              </section>
                            ) : null}

                            <section className="space-y-2">
                              <div className="text-sm font-medium text-stone-700">基础字段</div>
                              <div className="grid gap-3 rounded-2xl border border-stone-200 bg-white p-4 sm:grid-cols-2">
                                <div>
                                  <div className="text-xs text-stone-400">开始时间</div>
                                  <div className="mt-1 text-sm text-stone-700">{getString(detail, "started_at") || "—"}</div>
                                </div>
                                <div>
                                  <div className="text-xs text-stone-400">结束时间</div>
                                  <div className="mt-1 text-sm text-stone-700">{getString(detail, "ended_at") || "—"}</div>
                                </div>
                                <div>
                                  <div className="text-xs text-stone-400">状态</div>
                                  <div className="mt-1 text-sm text-stone-700">{status.label}</div>
                                </div>
                                <div>
                                  <div className="text-xs text-stone-400">耗时</div>
                                  <div className="mt-1 text-sm text-stone-700">{formatDuration(duration)}</div>
                                </div>
                                <div>
                                  <div className="text-xs text-stone-400">结束原因</div>
                                  <div className="mt-1 text-sm text-stone-700">{endReasonLabel || endReason || "—"}</div>
                                </div>
                              </div>
                            </section>
                          </div>

                          <div className="space-y-5">
                            <section className="space-y-2">
                              <div className="text-sm font-medium text-stone-700">输入图片</div>
                              {inputImages.length === 0 ? (
                                <div className="rounded-2xl border border-dashed border-stone-200 bg-white px-4 py-8 text-center text-sm text-stone-400">
                                  无输入图片
                                </div>
                              ) : (
                                <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                                  {inputImages.map((image, index) => (
                                    <button
                                      key={`input-${index}-${image.name}`}
                                      type="button"
                                      className="group overflow-hidden rounded-2xl border border-stone-200 bg-white text-left"
                                      onClick={() => openLightbox(item.id, inputImages, index)}
                                    >
                                      <img
                                        src={buildImageSrc(item.id, image, assetAuthKey)}
                                        alt=""
                                        className="aspect-square w-full object-cover"
                                      />
                                      <div className="truncate px-3 py-2 text-xs text-stone-500 group-hover:text-stone-700">
                                        {image.name || `输入图 ${index + 1}`}
                                      </div>
                                    </button>
                                  ))}
                                </div>
                              )}
                            </section>

                            <section className="space-y-2">
                              <div className="text-sm font-medium text-stone-700">输出图片</div>
                              {outputImages.length === 0 ? (
                                <div className="rounded-2xl border border-dashed border-stone-200 bg-white px-4 py-8 text-center text-sm text-stone-400">
                                  无输出图片
                                </div>
                              ) : (
                                <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                                  {outputImages.map((image, index) => (
                                    <button
                                      key={`output-${index}-${image.name}`}
                                      type="button"
                                      className="group overflow-hidden rounded-2xl border border-stone-200 bg-white text-left"
                                      onClick={() => openLightbox(item.id, outputImages, index)}
                                    >
                                      <img
                                        src={buildImageSrc(item.id, image, assetAuthKey)}
                                        alt=""
                                        className="aspect-square w-full object-cover"
                                      />
                                      <div className="truncate px-3 py-2 text-xs text-stone-500 group-hover:text-stone-700">
                                        {image.name || `输出图 ${index + 1}`}
                                      </div>
                                    </button>
                                  ))}
                                </div>
                              )}
                            </section>

                            <section className="space-y-2">
                              <div className="text-sm font-medium text-stone-700">附加数据</div>
                              <pre className="overflow-x-auto rounded-2xl border border-stone-200 bg-white p-4 text-xs leading-6 text-stone-600">
                                {Object.keys(extraDetail).length > 0 ? JSON.stringify(extraDetail, null, 2) : "无"}
                              </pre>
                            </section>
                          </div>
                        </div>
                      </div>
                    ) : null}
                  </CardContent>
                </Card>
              );
            })
          )}
        </div>
      </div>

      <ImageLightbox
        images={lightboxImages}
        currentIndex={lightboxIndex}
        open={lightboxOpen}
        onOpenChange={setLightboxOpen}
        onIndexChange={setLightboxIndex}
      />
    </>
  );
}
