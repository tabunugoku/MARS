import { cn } from "@/lib/utils";
import { Page } from "./layout";

export function Block({ className }: { className?: string }) {
  return <div className={cn("cn-shimmer rounded-md", className)} aria-hidden />;
}

export function CoverSkeleton() {
  return (
    <div>
      <Block className="aspect-[2/3] w-full rounded-[6px]" />
      <Block className="mt-2 h-4 w-4/5" />
      <Block className="mt-1 h-3 w-1/2" />
    </div>
  );
}

export function GridSkeleton({ count = 12 }: { count?: number }) {
  return (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(128px,1fr))] gap-x-4 gap-y-6 md:grid-cols-[repeat(auto-fill,minmax(150px,1fr))]">
      {Array.from({ length: count }, (_, i) => (
        <CoverSkeleton key={i} />
      ))}
    </div>
  );
}

export function PageSkeleton() {
  return (
    <Page>
      <Block className="mb-6 mt-2 h-8 w-56" />
      <GridSkeleton />
    </Page>
  );
}
