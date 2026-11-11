// Chooses the API implementation. The Go server is the default; the mock is
// imported only when VITE_API=mock, so a production build never contains it.
import { realApi, type Api } from "./real";

const impl: Promise<Api> =
  import.meta.env.VITE_API === "mock" ? import("./mock").then((m) => m.mockApi) : Promise.resolve(realApi);

/** Every call forwards to the chosen implementation. */
export const api = Object.fromEntries(
  Object.keys(realApi).map((k) => [k, (...args: unknown[]) => impl.then((i) => (i[k as keyof Api] as (...a: unknown[]) => unknown)(...args))]),
) as Api;
