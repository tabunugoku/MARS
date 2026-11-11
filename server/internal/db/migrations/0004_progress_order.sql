-- Ordering for progress writes from one reader session. A reader names
-- itself (writer) and numbers its writes (seq); a write that arrives after a
-- later one from the same writer is ignored, so a page sent at unload can
-- never be overwritten by an older page still in flight.
ALTER TABLE read_progress ADD COLUMN writer TEXT NOT NULL DEFAULT '';
ALTER TABLE read_progress ADD COLUMN seq INTEGER NOT NULL DEFAULT 0;
