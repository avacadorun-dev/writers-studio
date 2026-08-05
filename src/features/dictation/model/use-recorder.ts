import { useSyncExternalStore } from "react";

import { recorderController } from "./recorder-controller";

export function useRecorder() {
  const snapshot = useSyncExternalStore(
    recorderController.subscribe,
    recorderController.getSnapshot,
    recorderController.getSnapshot,
  );
  return { snapshot, controller: recorderController };
}
