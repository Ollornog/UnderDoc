-- SPDX-License-Identifier: MIT
-- The text of files reaches the search (ADR-24, M-2). A file is found by the text of its current version, and a page also
-- by the text of its attachments, so a new version, a text read later (in the browser, on the server, by a model) and an
-- attachment added or removed each put the object into the queue of 019_search.sql. Changes of the file object itself
-- (name, place, trash) already do through the trigger on objects.

-- A text belongs to a blob: every file whose current version is that blob, and every page with it as an attachment.
CREATE FUNCTION search_queue_text() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    t text := coalesce(NEW.tenant, OLD.tenant);
    b text := coalesce(NEW.blob, OLD.blob);
BEGIN
    PERFORM search_enqueue(t, ARRAY(
        SELECT v.file FROM file_versions v
        WHERE v.tenant = t AND v.blob = b
          AND v.number = (SELECT max(number) FROM file_versions w WHERE w.tenant = t AND w.file = v.file)
        UNION
        SELECT a.page FROM attachments a WHERE a.tenant = t AND a.blob = b));
    RETURN NULL;
END
$$;

CREATE TRIGGER file_texts_to_search AFTER INSERT OR UPDATE OR DELETE ON file_texts
    FOR EACH ROW EXECUTE FUNCTION search_queue_text();

-- A new version is the file as it is now.
CREATE FUNCTION search_queue_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM search_enqueue(NEW.tenant, ARRAY[NEW.file]);
    RETURN NULL;
END
$$;

CREATE TRIGGER file_versions_to_search AFTER INSERT ON file_versions
    FOR EACH ROW EXECUTE FUNCTION search_queue_version();

-- An attachment added, renamed or removed changes what its page is found by.
CREATE FUNCTION search_queue_attachment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM search_enqueue(OLD.tenant, ARRAY[OLD.page]);
    ELSE
        PERFORM search_enqueue(NEW.tenant, ARRAY[NEW.page]);
    END IF;
    RETURN NULL;
END
$$;

CREATE TRIGGER attachments_to_search AFTER INSERT OR UPDATE OR DELETE ON attachments
    FOR EACH ROW EXECUTE FUNCTION search_queue_attachment();
