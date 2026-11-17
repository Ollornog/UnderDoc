-- SPDX-License-Identifier: MIT
-- How a document is shown is kept with it (ADR-16, M-8): its width (slim, wide or full) and where its contents show (a
-- marker, a side panel, or hidden). NULL means the default of the workspace.

ALTER TABLE objects ADD COLUMN width text CHECK (width IN ('slim', 'wide', 'full'));
ALTER TABLE objects ADD COLUMN contents text CHECK (contents IN ('marker', 'side', 'hidden'));
