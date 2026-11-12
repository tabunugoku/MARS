import { useRef } from "react";

/** Bookkeeping for a bulk-edit dialog that saves in chunks. A failed save may
 *  already have written earlier chunks, so the caller must hear about it.
 *
 *  - saved(): the save worked. The dialog closes and onDone(true) runs once.
 *  - saveFailed(): the save failed. If the dialog is still open it stays open
 *    with its draft; onDone(false) runs when it is then closed. If the user
 *    already closed it during the save, onDone(false) runs at once.
 *  - close(next): the dialog's open/close handler. Closing after a failure
 *    (and before a successful retry) runs onDone(false) once.
 *
 *  An open dialog with no failure never calls onDone. The open state is kept
 *  in a ref updated by close(), since the `open` prop lags behind it. The
 *  parent mounts the dialog fresh per open, so it starts open. */
export function useBulkDialog(onOpenChange: (open: boolean) => void, onDone: (ok: boolean) => void) {
  const openRef = useRef(true);
  // A failed save whose outcome onDone has not reported yet.
  const pendingFailure = useRef(false);

  const close = (next: boolean) => {
    onOpenChange(next);
    if (next) return;
    openRef.current = false;
    if (pendingFailure.current) {
      pendingFailure.current = false;
      onDone(false);
    }
  };

  const saved = () => {
    pendingFailure.current = false;
    openRef.current = false;
    onOpenChange(false);
    onDone(true);
  };

  const saveFailed = () => {
    if (openRef.current) {
      pendingFailure.current = true;
      return;
    }
    onDone(false);
  };

  return { close, saved, saveFailed };
}
