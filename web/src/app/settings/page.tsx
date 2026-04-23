"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  Eye,
  EyeOff,
  Import,
  Link2,
  LoaderCircle,
  Pencil,
  Plus,
  Save,
  Search,
  ServerCog,
  Trash2,
  Unplug,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  createCPAPool,
  deleteCPAPool,
  fetchCPAPoolFiles,
  fetchCPAPools,
  startCPAImport,
  updateCPAPool,
  type CPAPool,
  type CPARemoteFile,
} from "@/lib/api";
import { ChatCompletionsSettingsCard } from "./components/chat-completions-settings-card";
import { ProxySettingsCard } from "./components/proxy-settings-card";

const PAGE_SIZE_OPTIONS = ["50", "100", "200"] as const;

function normalizeFiles(items: CPARemoteFile[]) {
  const seen = new Set<string>();
  const files: CPARemoteFile[] = [];
  for (const item of items) {
    const name = String(item.name || "").trim();
    if (!name || seen.has(name)) {
      continue;
    }
    seen.add(name);
    files.push({
      name,
      email: String(item.email || "").trim(),
    });
  }
  return files;
}

export default function SettingsPage() {
  const didLoadRef = useRef(false);
  const pollTimerRef = useRef<number | null>(null);

  const [pools, setPools] = useState<CPAPool[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingPool, setEditingPool] = useState<CPAPool | null>(null);
  const [formName, setFormName] = useState("");
  const [formBaseUrl, setFormBaseUrl] = useState("");
  const [formSecretKey, setFormSecretKey] = useState("");
  const [showSecret, setShowSecret] = useState(false);
  const [isSaving, setIsSaving] = useState(false);

  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [loadingFilesId, setLoadingFilesId] = useState<string | null>(null);

  const [browserOpen, setBrowserOpen] = useState(false);
  const [browserPool, setBrowserPool] = useState<CPAPool | null>(null);
  const [remoteFiles, setRemoteFiles] = useState<CPARemoteFile[]>([]);
  const [selectedNames, setSelectedNames] = useState<string[]>([]);
  const [fileQuery, setFileQuery] = useState("");
  const [filePage, setFilePage] = useState(1);
  const [pageSize, setPageSize] = useState<(typeof PAGE_SIZE_OPTIONS)[number]>("100");
  const [isStartingImport, setIsStartingImport] = useState(false);

  const loadPools = async () => {
    setIsLoading(true);
    try {
      const poolData = await fetchCPAPools();
      setPools(poolData.pools);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "加载 CPA 连接失败");
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (didLoadRef.current) {
      return;
    }
    didLoadRef.current = true;
    void loadPools();
  }, []);

  useEffect(() => {
    const runningPoolIds = pools
      .filter((pool) => pool.import_job?.status === "pending" || pool.import_job?.status === "running")
      .map((pool) => pool.id);
    const hasRunningJobs = runningPoolIds.length > 0;
    if (!hasRunningJobs) {
      if (pollTimerRef.current !== null) {
        window.clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
      return;
    }

    pollTimerRef.current = window.setInterval(() => {
      void fetchCPAPools()
        .then((poolData) => {
          setPools(poolData.pools);
        })
        .catch((error) => {
          if (pollTimerRef.current !== null) {
            window.clearInterval(pollTimerRef.current);
            pollTimerRef.current = null;
          }
          toast.error(error instanceof Error ? error.message : "查询导入进度失败");
        });
    }, 1500);

    return () => {
      if (pollTimerRef.current !== null) {
        window.clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, [pools]);

  const openAddDialog = () => {
    setEditingPool(null);
    setFormName("");
    setFormBaseUrl("");
    setFormSecretKey("");
    setShowSecret(false);
    setDialogOpen(true);
  };

  const openEditDialog = (pool: CPAPool) => {
    setEditingPool(pool);
    setFormName(pool.name);
    setFormBaseUrl(pool.base_url);
    setFormSecretKey("");
    setShowSecret(false);
    setDialogOpen(true);
  };

  const handleSave = async () => {
    if (!formBaseUrl.trim()) {
      toast.error("请输入 CPA 地址");
      return;
    }
    if (!editingPool && !formSecretKey.trim()) {
      toast.error("请输入 Secret Key");
      return;
    }

    setIsSaving(true);
    try {
      if (editingPool) {
        const data = await updateCPAPool(editingPool.id, {
          name: formName.trim(),
          base_url: formBaseUrl.trim(),
          secret_key: formSecretKey.trim() || undefined,
        });
        setPools(data.pools);
        toast.success("连接已更新");
      } else {
        const data = await createCPAPool({
          name: formName.trim(),
          base_url: formBaseUrl.trim(),
          secret_key: formSecretKey.trim(),
        });
        setPools(data.pools);
        toast.success("连接已添加");
      }
      setDialogOpen(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存失败");
    } finally {
      setIsSaving(false);
    }
  };

  const handleDelete = async (pool: CPAPool) => {
    setDeletingId(pool.id);
    try {
      const data = await deleteCPAPool(pool.id);
      setPools(data.pools);
      toast.success("连接已删除");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除失败");
    } finally {
      setDeletingId(null);
    }
  };

  const handleBrowseFiles = async (pool: CPAPool) => {
    setLoadingFilesId(pool.id);
    try {
      const data = await fetchCPAPoolFiles(pool.id);
      const files = normalizeFiles(data.files);
      setBrowserPool(pool);
      setRemoteFiles(files);
      setSelectedNames([]);
      setFileQuery("");
      setFilePage(1);
      setBrowserOpen(true);
      toast.success(`读取成功，共 ${files.length} 个远程账号`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取远程账号失败");
    } finally {
      setLoadingFilesId(null);
    }
  };

  const filteredFiles = useMemo(() => {
    const query = fileQuery.trim().toLowerCase();
    if (!query) {
      return remoteFiles;
    }
    return remoteFiles.filter((item) => {
      return item.email.toLowerCase().includes(query) || item.name.toLowerCase().includes(query);
    });
  }, [fileQuery, remoteFiles]);

  const currentPageSize = Number(pageSize);
  const filePageCount = Math.max(1, Math.ceil(filteredFiles.length / currentPageSize));
  const safeFilePage = Math.min(filePage, filePageCount);
  const pagedFiles = filteredFiles.slice((safeFilePage - 1) * currentPageSize, safeFilePage * currentPageSize);
  const allFilteredSelected = filteredFiles.length > 0 && filteredFiles.every((item) => selectedNames.includes(item.name));

  const toggleFile = (name: string, checked: boolean) => {
    setSelectedNames((prev) => {
      if (checked) {
        return Array.from(new Set([...prev, name]));
      }
      return prev.filter((item) => item !== name);
    });
  };

  const handleToggleSelectAllFiltered = (checked: boolean) => {
    if (checked) {
      setSelectedNames(Array.from(new Set([...selectedNames, ...filteredFiles.map((item) => item.name)])));
      return;
    }
    const filteredSet = new Set(filteredFiles.map((item) => item.name));
    setSelectedNames((prev) => prev.filter((name) => !filteredSet.has(name)));
  };

  const handleStartImport = async () => {
    if (!browserPool) {
      return;
    }
    if (selectedNames.length === 0) {
      toast.error("请先选择要导入的账号");
      return;
    }

    setIsStartingImport(true);
    try {
      const result = await startCPAImport(browserPool.id, selectedNames);
      setPools((prev) =>
        prev.map((pool) => (pool.id === browserPool.id ? { ...pool, import_job: result.import_job } : pool)),
      );
      setBrowserOpen(false);
      toast.success("导入任务已启动");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "启动导入失败");
    } finally {
      setIsStartingImport(false);
    }
  };

  return (
    <>
      <section className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="space-y-1">
          <div className="text-xs font-semibold tracking-[0.18em] text-stone-500 uppercase">Settings</div>
          <h1 className="text-2xl font-semibold tracking-tight">设置</h1>
        </div>
      </section>

      <section className="space-y-6">
        <ChatCompletionsSettingsCard />

        <ProxySettingsCard />

        <Card className="rounded-2xl border-white/80 bg-white/90 shadow-sm">
          <CardContent className="space-y-6 p-6">
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-3">
                <div className="flex size-10 items-center justify-center rounded-xl bg-stone-100">
                  <ServerCog className="size-5 text-stone-600" />
                </div>
                <div>
                  <h2 className="text-lg font-semibold tracking-tight">CPA 连接管理</h2>
                  <p className="text-sm text-stone-500">先配置连接，再按需查询远程账号并选择导入到本地号池。</p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {pools.length > 0 ? <Badge className="rounded-md px-2.5 py-1">{pools.length} 个连接</Badge> : null}
                <Button className="h-9 rounded-xl bg-stone-950 px-4 text-white hover:bg-stone-800" onClick={openAddDialog}>
                  <Plus className="size-4" />
                  添加连接
                </Button>
              </div>
            </div>

            {isLoading ? (
              <div className="flex items-center justify-center py-10">
                <LoaderCircle className="size-5 animate-spin text-stone-400" />
              </div>
            ) : pools.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-3 rounded-xl bg-stone-50 px-6 py-10 text-center">
                <ServerCog className="size-8 text-stone-300" />
                <div className="space-y-1">
                  <p className="text-sm font-medium text-stone-600">暂无 CPA 连接</p>
                  <p className="text-sm text-stone-400">点击「添加连接」保存你的 CLIProxyAPI 信息。</p>
                </div>
              </div>
            ) : (
              <div className="space-y-3">
                {pools.map((pool) => {
                  const isBusy = deletingId === pool.id || loadingFilesId === pool.id;
                  const importJob = pool.import_job ?? null;
                  return (
                    <div key={pool.id} className="flex flex-col gap-3 rounded-xl border border-stone-200 bg-white px-4 py-3">
                      <div className="flex items-center justify-between gap-3">
                        <div className="min-w-0">
                          <div className="text-sm font-medium text-stone-800">{pool.name || pool.base_url}</div>
                          <div className="truncate text-xs text-stone-400">{pool.base_url}</div>
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            className="rounded-lg p-2 text-stone-400 transition hover:bg-stone-100 hover:text-stone-700"
                            onClick={() => openEditDialog(pool)}
                            disabled={isBusy}
                            title="编辑"
                          >
                            <Pencil className="size-4" />
                          </button>
                          <button
                            type="button"
                            className="rounded-lg p-2 text-stone-400 transition hover:bg-rose-50 hover:text-rose-500"
                            onClick={() => void handleDelete(pool)}
                            disabled={isBusy}
                            title="删除"
                          >
                            {deletingId === pool.id ? <LoaderCircle className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
                          </button>
                        </div>
                      </div>

                      <div className="flex items-center gap-2">
                        <Button
                          variant="outline"
                          className="h-8 rounded-lg border-stone-200 bg-white px-3 text-xs text-stone-600"
                          onClick={() => void handleBrowseFiles(pool)}
                          disabled={isBusy}
                        >
                          {loadingFilesId === pool.id ? <LoaderCircle className="size-3.5 animate-spin" /> : <Import className="size-3.5" />}
                          同步
                        </Button>
                      </div>

                      {importJob ? (
                        <div className="space-y-2 rounded-xl bg-stone-50 px-3 py-3">
                          <div className="text-xs font-medium tracking-[0.16em] text-stone-400 uppercase">导入任务</div>
                          {(() => {
                            const progress = importJob.total > 0 ? Math.round((importJob.completed / importJob.total) * 100) : 0;
                            return (
                              <div className="rounded-lg border border-stone-200 bg-white px-3 py-3">
                                <div className="flex items-center justify-between gap-3">
                                  <div className="min-w-0">
                                    <div className="text-sm font-medium text-stone-700">
                                      状态 {importJob.status}，已处理 {importJob.completed}/{importJob.total}
                                    </div>
                                    <div className="truncate text-xs text-stone-400">
                                      任务 {importJob.job_id.slice(0, 8)} · {importJob.created_at}
                                    </div>
                                  </div>
                                  <Badge
                                    variant={importJob.status === "completed" ? "success" : importJob.status === "failed" ? "danger" : "info"}
                                    className="rounded-md"
                                  >
                                    {progress}%
                                  </Badge>
                                </div>
                                <div className="mt-3 h-2 overflow-hidden rounded-full bg-stone-200">
                                  <div className="h-full rounded-full bg-stone-900 transition-all" style={{ width: `${progress}%` }} />
                                </div>
                                <div className="mt-2 flex flex-wrap gap-2 text-xs text-stone-500">
                                  <span>新增 {importJob.added}</span>
                                  <span>跳过 {importJob.skipped}</span>
                                  <span>刷新 {importJob.refreshed}</span>
                                  <span>失败 {importJob.failed}</span>
                                </div>
                              </div>
                            );
                          })()}
                        </div>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            )}

            <div className="rounded-xl bg-stone-50 px-4 py-3 text-sm leading-6 text-stone-500">
              <p className="font-medium text-stone-600">使用说明</p>
              <ul className="mt-1 list-inside list-disc space-y-0.5">
                <li>页面进入后先读取系统里已配置的 CPA 连接。</li>
                <li>点击某个连接的「同步」后，会先读取远程账号列表并展示给前端选择。</li>
                <li>确认选择后，后端后台下载对应 access_token 并导入本地号池。</li>
                <li>前端只轮询导入进度，不直接参与 download。</li>
              </ul>
            </div>
          </CardContent>
        </Card>
      </section>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent showCloseButton={false} className="rounded-2xl p-6">
          <DialogHeader className="gap-2">
            <DialogTitle>{editingPool ? "编辑连接" : "添加连接"}</DialogTitle>
            <DialogDescription className="text-sm leading-6">
              {editingPool ? "修改 CPA 连接信息" : "添加一个新的 CLIProxyAPI 连接"}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-sm font-medium text-stone-700">名称（可选）</label>
              <Input
                value={formName}
                onChange={(event) => setFormName(event.target.value)}
                placeholder="例如：主号池、备用池"
                className="h-11 rounded-xl border-stone-200 bg-white"
              />
            </div>
            <div className="space-y-2">
              <label className="flex items-center gap-1.5 text-sm font-medium text-stone-700">
                <Link2 className="size-3.5" />
                CPA 地址
              </label>
              <Input
                value={formBaseUrl}
                onChange={(event) => setFormBaseUrl(event.target.value)}
                placeholder="http://your-cpa-host:8317"
                className="h-11 rounded-xl border-stone-200 bg-white"
              />
            </div>
            <div className="space-y-2">
              <label className="flex items-center gap-1.5 text-sm font-medium text-stone-700">
                <Unplug className="size-3.5" />
                Management Secret Key
              </label>
              <div className="relative">
                <Input
                  type={showSecret ? "text" : "password"}
                  value={formSecretKey}
                  onChange={(event) => setFormSecretKey(event.target.value)}
                  placeholder={editingPool ? "留空则不修改密钥" : "CPA 管理密钥"}
                  className="h-11 rounded-xl border-stone-200 bg-white pr-10"
                />
                <button
                  type="button"
                  className="absolute top-1/2 right-3 -translate-y-1/2 text-stone-400 transition hover:text-stone-600"
                  onClick={() => setShowSecret((prev) => !prev)}
                >
                  {showSecret ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                </button>
              </div>
            </div>
          </div>
          <DialogFooter className="pt-2">
            <Button
              variant="secondary"
              className="h-10 rounded-xl bg-stone-100 px-5 text-stone-700 hover:bg-stone-200"
              onClick={() => setDialogOpen(false)}
              disabled={isSaving}
            >
              取消
            </Button>
            <Button
              className="h-10 rounded-xl bg-stone-950 px-5 text-white hover:bg-stone-800"
              onClick={() => void handleSave()}
              disabled={isSaving}
            >
              {isSaving ? <LoaderCircle className="size-4 animate-spin" /> : <Save className="size-4" />}
              {editingPool ? "保存修改" : "添加"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={browserOpen} onOpenChange={setBrowserOpen}>
        <DialogContent showCloseButton={false} className="max-h-[90vh] max-w-5xl rounded-2xl p-6">
          <DialogHeader className="gap-2">
            <DialogTitle>选择要导入的账号</DialogTitle>
            <DialogDescription className="text-sm leading-6">
              {browserPool ? `来自 ${browserPool.name || browserPool.base_url}` : "读取到的远程账号列表"}
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div className="relative min-w-[260px]">
              <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-stone-400" />
              <Input
                value={fileQuery}
                onChange={(event) => {
                  setFileQuery(event.target.value);
                  setFilePage(1);
                }}
                placeholder="搜索 email 或文件名"
                className="h-10 rounded-xl border-stone-200 bg-white pl-10"
              />
            </div>
            <div className="flex items-center gap-2">
              <Select
                value={pageSize}
                onValueChange={(value) => {
                  setPageSize(value as (typeof PAGE_SIZE_OPTIONS)[number]);
                  setFilePage(1);
                }}
              >
                <SelectTrigger className="h-10 w-[120px] rounded-xl border-stone-200 bg-white">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PAGE_SIZE_OPTIONS.map((item) => (
                    <SelectItem key={item} value={item}>
                      {item} / 页
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                className="h-10 rounded-xl border-stone-200 bg-white px-4 text-stone-700"
                onClick={() => handleToggleSelectAllFiltered(!allFilteredSelected)}
              >
                {allFilteredSelected ? "取消全选" : "全选筛选结果"}
              </Button>
            </div>
          </div>

          <div className="rounded-xl border border-stone-200">
            <div className="flex items-center justify-between border-b border-stone-100 px-4 py-3 text-sm text-stone-500">
              <div className="flex items-center gap-3">
                <Checkbox checked={allFilteredSelected} onCheckedChange={(checked) => handleToggleSelectAllFiltered(Boolean(checked))} />
                <span>筛选结果 {filteredFiles.length} 个</span>
              </div>
              <span>已选 {selectedNames.length} 个</span>
            </div>
            <div className="max-h-[420px] overflow-auto">
              {pagedFiles.length === 0 ? (
                <div className="flex items-center justify-center py-12 text-sm text-stone-400">没有匹配的远程账号</div>
              ) : (
                <div className="divide-y divide-stone-100">
                  {pagedFiles.map((item) => (
                    <label key={item.name} className="flex cursor-pointer items-center gap-3 px-4 py-3 hover:bg-stone-50">
                      <Checkbox
                        checked={selectedNames.includes(item.name)}
                        onCheckedChange={(checked) => toggleFile(item.name, Boolean(checked))}
                      />
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium text-stone-700">{item.email || item.name}</div>
                        <div className="truncate text-xs text-stone-400">{item.name}</div>
                      </div>
                    </label>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className="flex items-center justify-between text-sm text-stone-500">
            <span>
              第 {filteredFiles.length === 0 ? 0 : (safeFilePage - 1) * currentPageSize + 1} -{" "}
              {Math.min(safeFilePage * currentPageSize, filteredFiles.length)} 条，共 {filteredFiles.length} 条
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                className="h-9 rounded-xl border-stone-200 bg-white px-3"
                onClick={() => setFilePage((prev) => Math.max(1, prev - 1))}
                disabled={safeFilePage <= 1}
              >
                上一页
              </Button>
              <span>
                {safeFilePage}/{filePageCount}
              </span>
              <Button
                variant="outline"
                className="h-9 rounded-xl border-stone-200 bg-white px-3"
                onClick={() => setFilePage((prev) => Math.min(filePageCount, prev + 1))}
                disabled={safeFilePage >= filePageCount}
              >
                下一页
              </Button>
            </div>
          </div>

          <DialogFooter className="pt-2">
            <Button
              variant="secondary"
              className="h-10 rounded-xl bg-stone-100 px-5 text-stone-700 hover:bg-stone-200"
              onClick={() => setBrowserOpen(false)}
              disabled={isStartingImport}
            >
              取消
            </Button>
            <Button
              className="h-10 rounded-xl bg-stone-950 px-5 text-white hover:bg-stone-800"
              onClick={() => void handleStartImport()}
              disabled={isStartingImport || selectedNames.length === 0}
            >
              {isStartingImport ? <LoaderCircle className="size-4 animate-spin" /> : <Import className="size-4" />}
              导入选中账号
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
