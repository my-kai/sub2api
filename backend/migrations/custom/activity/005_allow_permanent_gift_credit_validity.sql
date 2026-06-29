-- Allow activity rewards to create permanent gift-credit grants.
-- A value of 0 is explicit product semantics for no expiration; negative values remain invalid.

ALTER TABLE {{TABLE_PREFIX}}custom_red_packet_rain_configs
    DROP CONSTRAINT IF EXISTS {{TABLE_PREFIX}}custom_red_packet_rain_configs_gift_validity_days_check;

ALTER TABLE {{TABLE_PREFIX}}custom_red_packet_rain_configs
    ADD CONSTRAINT {{TABLE_PREFIX}}custom_red_packet_rain_configs_gift_validity_days_check
        CHECK (gift_validity_days >= 0);
