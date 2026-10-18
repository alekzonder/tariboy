-- The loop schedule: an RFC 5545 DTSTART + RRULE pair, empty when unset.
ALTER TABLE agents ADD COLUMN schedule_rrule TEXT NOT NULL DEFAULT '';
