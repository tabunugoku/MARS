import { Composition } from "remotion";
import { DURATION, Promo } from "./Promo";

export const RemotionRoot = () => (
  <>
    <Composition id="promo-en" component={Promo} durationInFrames={DURATION} fps={30} width={1920} height={1080} defaultProps={{ lang: "en" as const }} />
    <Composition id="promo-ja" component={Promo} durationInFrames={DURATION} fps={30} width={1920} height={1080} defaultProps={{ lang: "ja" as const }} />
  </>
);
