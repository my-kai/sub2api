-- Store the expiration window for red-packet-rain rewards after they enter gift balance.

ALTER TABLE {{TABLE_PREFIX}}custom_red_packet_rain_configs
    ADD COLUMN IF NOT EXISTS gift_validity_days INTEGER NOT NULL DEFAULT 30;

ALTER TABLE {{TABLE_PREFIX}}custom_red_packet_rain_configs
    DROP CONSTRAINT IF EXISTS {{TABLE_PREFIX}}custom_red_packet_rain_configs_gift_validity_days_check;

ALTER TABLE {{TABLE_PREFIX}}custom_red_packet_rain_configs
    ADD CONSTRAINT {{TABLE_PREFIX}}custom_red_packet_rain_configs_gift_validity_days_check
        CHECK (gift_validity_days > 0);
