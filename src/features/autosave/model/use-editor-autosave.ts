import type { Editor, JSONContent } from "@tiptap/react";
import { useCallback, useEffect, useRef, useState } from "react";

import { type AnnotationAnchorUpdate, saveRevision, type TiptapNode } from "~entities/project";
import { APIError } from "~shared/api";
import { markEditorSnapshotSynced, saveEditorSnapshot } from "~shared/lib";
import type { SaveState } from "~shared/ui";

interface UseEditorAutosaveInput {
  editor: Editor | null;
  nodeId: string;
  projectId: string;
  revision: number;
  onRevision: (revision: number) => void;
  getAnnotationAnchors?: () => AnnotationAnchorUpdate[];
}

interface UseEditorAutosaveResult {
  saveState: SaveState;
  flush: () => Promise<void>;
}

export function useEditorAutosave({
  editor,
  nodeId,
  projectId,
  revision,
  onRevision,
  getAnnotationAnchors,
}: UseEditorAutosaveInput): UseEditorAutosaveResult {
  const [saveState, setSaveState] = useState<SaveState>("saved");
  const baseRevision = useRef(revision);
  const dirty = useRef(false);
  const inFlight = useRef(false);
  const conflict = useRef(false);
  const localTimer = useRef<number | undefined>(undefined);
  const serverTimer = useRef<number | undefined>(undefined);
  const maxTimer = useRef<number | undefined>(undefined);
  const latestContent = useRef<JSONContent | undefined>(undefined);
  const getAnnotationAnchorsRef = useRef(getAnnotationAnchors);

  useEffect(() => {
    baseRevision.current = revision;
  }, [revision]);

  useEffect(() => {
    getAnnotationAnchorsRef.current = getAnnotationAnchors;
  }, [getAnnotationAnchors]);

  const persistLocal = useCallback(async () => {
    if (!editor || !latestContent.current) {
      return;
    }
    await saveEditorSnapshot({
      nodeId,
      projectId,
      baseRevision: baseRevision.current,
      content: latestContent.current as TiptapNode,
      plainText: editor.getText({ blockSeparator: "\n\n" }),
      savedAt: Date.now(),
      synced: false,
    });
  }, [editor, nodeId, projectId]);

  const flush = useCallback(async () => {
    if (!editor || !dirty.current || inFlight.current) {
      return;
    }
    window.clearTimeout(serverTimer.current);
    window.clearTimeout(maxTimer.current);
    await persistLocal();
    const content = latestContent.current ?? editor.getJSON();
    const savingBase = baseRevision.current;
    dirty.current = false;
    inFlight.current = true;
    setSaveState("saving");
    try {
      const response = await saveRevision(nodeId, {
        baseRevision: savingBase,
        content: content as TiptapNode,
        source: "manual",
        annotationAnchors: getAnnotationAnchorsRef.current?.(),
      });
      baseRevision.current = response.revision.revision;
      onRevision(response.revision.revision);
      await markEditorSnapshotSynced(nodeId, response.revision.revision);
      conflict.current = false;
      setSaveState(dirty.current ? "saving" : "saved");
    } catch (error) {
      dirty.current = true;
      if (error instanceof APIError && error.code === "revision_conflict") {
        conflict.current = true;
        setSaveState("conflict");
      } else {
        setSaveState(navigator.onLine ? "pending" : "local");
      }
    } finally {
      inFlight.current = false;
      if (dirty.current && !conflict.current) {
        serverTimer.current = window.setTimeout(() => {
          void flush();
        }, 1_500);
      }
    }
  }, [editor, nodeId, onRevision, persistLocal]);

  useEffect(() => {
    if (!editor) {
      return;
    }
    const handleUpdate = () => {
      latestContent.current = editor.getJSON();
      dirty.current = true;
      setSaveState("saving");
      window.clearTimeout(localTimer.current);
      localTimer.current = window.setTimeout(() => {
        void persistLocal();
      }, 300);
      window.clearTimeout(serverTimer.current);
      serverTimer.current = window.setTimeout(() => {
        void flush();
      }, 1_500);
      if (!maxTimer.current) {
        maxTimer.current = window.setTimeout(() => {
          maxTimer.current = undefined;
          void flush();
        }, 15_000);
      }
    };
    const handleOnline = () => {
      if (dirty.current && !conflict.current) {
        void flush();
      }
    };
    editor.on("update", handleUpdate);
    window.addEventListener("online", handleOnline);
    return () => {
      editor.off("update", handleUpdate);
      window.removeEventListener("online", handleOnline);
      window.clearTimeout(localTimer.current);
      window.clearTimeout(serverTimer.current);
      window.clearTimeout(maxTimer.current);
      if (dirty.current) {
        latestContent.current = editor.getJSON();
        void persistLocal();
      }
    };
  }, [editor, flush, persistLocal]);

  return { saveState, flush };
}
