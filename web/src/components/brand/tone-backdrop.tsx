// The chart paper behind every screen: a screentone dot grid over the top of
// the content panel, fading out downwards. Nothing else is drawn behind the
// content. Pointer-transparent and hidden from assistive tech.
export function ToneBackdrop() {
  return (
    <div aria-hidden className="pointer-events-none absolute inset-0 overflow-hidden">
      <div className="tone absolute inset-0 [mask-image:linear-gradient(to_bottom,#000,transparent_70%)] opacity-70" />
    </div>
  );
}
