-- SPDX-License-Identifier: MIT
-- Who invited an account (B-7). A group admin manages only the persons who are in none but their groups, or whom they
-- invited, and only who invited a person gives them a new sign-in link (besides administrators and delegated
-- administrators). Empty for accounts that came in another way.
ALTER TABLE accounts ADD COLUMN invited_by text NOT NULL DEFAULT '';
