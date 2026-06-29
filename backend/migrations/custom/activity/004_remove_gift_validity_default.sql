-- Remove the temporary database default introduced by 003.
-- Application code must provide gift_validity_days explicitly for every activity.

ALTER TABLE {{TABLE_PREFIX}}custom_red_packet_rain_configs
    ALTER COLUMN gift_validity_days DROP DEFAULT;
