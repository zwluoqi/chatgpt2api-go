"use client";
import { LoaderCircle } from "lucide-react";
import { useMemo, useState } from "react";

import { ImageLightbox } from "@/components/image-lightbox";
import type { ImageConversation, StoredImage } from "@/store/image-conversations";

type ImageResultsProps = {
  selectedConversation: ImageConversation | null;
  isSelectedGenerating: boolean;
  openLightbox: (imageId: string) => void;
  formatConversationTime: (value: string) => string;
};

export function ImageResults({
  selectedConversation,
  isSelectedGenerating,
  openLightbox,
  formatConversationTime,
}: ImageResultsProps) {
  const [referenceLightboxOpen, setReferenceLightboxOpen] = useState(false);
  const [referenceLightboxIndex, setReferenceLightboxIndex] = useState(0);

  const referenceLightboxImages = useMemo(
    () =>
      (selectedConversation?.referenceImages ?? []).map((image, index) => ({
        id: `${image.name}-${index}`,
        src: image.dataUrl,
      })),
    [selectedConversation?.referenceImages],
  );

  if (!selectedConversation) {
    return (
      <div className="flex h-full min-h-[420px] items-center justify-center text-center">
        <div className="w-full max-w-4xl">
          <h1
            className="text-3xl font-semibold tracking-tight text-stone-950 md:text-5xl"
            style={{
              fontFamily: '"Palatino Linotype","Book Antiqua","URW Palladio L","Times New Roman",serif',
            }}
          >
            Turn ideas into images
          </h1>
          <p
            className="mt-4 text-[15px] italic tracking-[0.01em] text-stone-500"
            style={{
              fontFamily: '"Palatino Linotype","Book Antiqua","URW Palladio L","Times New Roman",serif',
            }}
          >
            Describe a scene, a mood, or a character, and let the next image start here.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto flex w-full max-w-[980px] flex-col gap-4">
      <ImageLightbox
        images={referenceLightboxImages}
        currentIndex={referenceLightboxIndex}
        open={referenceLightboxOpen}
        onOpenChange={setReferenceLightboxOpen}
        onIndexChange={setReferenceLightboxIndex}
      />

      <div className="flex justify-end">
        <div className="w-full max-w-[min(820px,92%)] px-1 pt-1">
          <div className="ml-auto flex max-w-full flex-col items-end gap-2.5 text-right">
            <div className="w-fit max-w-[min(32rem,100%)] whitespace-pre-wrap break-words text-[15px] leading-6 text-stone-700 sm:leading-7">
              {selectedConversation.prompt}
            </div>
            {selectedConversation.referenceImages?.length ? (
              <div
                className="grid w-fit auto-rows-fr gap-3"
                style={{
                  gridTemplateColumns: `repeat(${Math.min(selectedConversation.referenceImages.length, 3)}, minmax(0, 1fr))`,
                }}
              >
                {selectedConversation.referenceImages.map((image, index) => (
                  <button
                    key={`${image.name}-${index}`}
                    type="button"
                    onClick={() => {
                      setReferenceLightboxIndex(index);
                      setReferenceLightboxOpen(true);
                    }}
                    className="group relative aspect-square min-h-[112px] overflow-hidden rounded-[18px] border border-stone-200/80 bg-stone-100/60 text-left transition hover:border-stone-300 sm:min-h-[136px]"
                    aria-label={`预览参考图 ${image.name || index + 1}`}
                  >
                    <img
                      src={image.dataUrl}
                      alt={image.name || `参考图 ${index + 1}`}
                      className="absolute inset-0 h-full w-full object-cover transition duration-200 group-hover:scale-[1.02]"
                    />
                  </button>
                ))}
              </div>
            ) : null}
          </div>
        </div>
      </div>

      <div className="flex justify-start">
        <div className="w-full p-1">
          <div className="mb-4 flex flex-wrap items-center gap-2 text-xs text-stone-500">
            <span className="rounded-full bg-stone-100 px-3 py-1">{selectedConversation.mode === "edit" ? "编辑图" : "文生图"}</span>
            <span className="rounded-full bg-stone-100 px-3 py-1">{selectedConversation.model}</span>
            <span className="rounded-full bg-stone-100 px-3 py-1">{selectedConversation.count} 张</span>
            <span className="rounded-full bg-stone-100 px-3 py-1">
              {formatConversationTime(selectedConversation.createdAt)}
            </span>
            {isSelectedGenerating && (
              <span className="rounded-full bg-amber-50 px-3 py-1 text-amber-700">处理中</span>
            )}
          </div>

          {selectedConversation.status === "error" && selectedConversation.images.length === 0 ? (
            <div className="border-l-2 border-rose-300 bg-rose-50/70 px-4 py-4 text-sm leading-6 text-rose-600">
              {selectedConversation.error || "生成失败"}
            </div>
          ) : null}

          {selectedConversation.images.length > 0 ? (
            <div className="columns-1 gap-4 space-y-4 sm:columns-2 xl:columns-3">
              {selectedConversation.images.map((image, index) => (
                <div key={image.id} className="break-inside-avoid overflow-hidden rounded-[22px]">
                  <ImageResultCard image={image} index={index} onOpen={openLightbox} />
                </div>
              ))}
            </div>
          ) : null}

          {selectedConversation.status === "error" && selectedConversation.images.length > 0 ? (
            <div className="mt-4 border-l-2 border-amber-300 bg-amber-50/70 px-4 py-3 text-sm leading-6 text-amber-700">
              {selectedConversation.error}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function ImageResultCard({
  image,
  index,
  onOpen,
}: {
  image: StoredImage;
  index: number;
  onOpen: (imageId: string) => void;
}) {
  if (image.status === "success" && image.b64_json) {
    return (
      <button type="button" onClick={() => onOpen(image.id)} className="group block w-full cursor-zoom-in">
        <img
          src={`data:image/png;base64,${image.b64_json}`}
          alt={`Generated result ${index + 1}`}
          className="block h-auto w-full transition duration-200 group-hover:brightness-90"
        />
      </button>
    );
  }

  if (image.status === "error") {
    return (
      <div className="flex min-h-[320px] items-center justify-center bg-rose-50 px-6 py-8 text-center text-sm leading-6 text-rose-600">
        {image.error || "生成失败"}
      </div>
    );
  }

  return (
    <div className="flex min-h-[320px] flex-col items-center justify-center gap-3 bg-stone-100/80 px-6 py-8 text-center text-stone-500">
      <div className="rounded-full bg-white p-3 shadow-sm">
        <LoaderCircle className="size-5 animate-spin" />
      </div>
      <p className="text-sm">正在生成图片...</p>
    </div>
  );
}
