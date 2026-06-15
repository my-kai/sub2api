-- Add explicit priority to image generation upstream channel JSON.
-- Lower values are selected first; existing list order is converted to 100, 200, ...
-- so deployed configurations keep their previous failover order after upgrade.

UPDATE {{TABLE_PREFIX}}image_generation_config
SET upstream_channels = (
    SELECT COALESCE(
        jsonb_agg(
            CASE
                WHEN channel ? 'priority' THEN channel
                ELSE channel || jsonb_build_object('priority', ordinality * 100)
            END
            ORDER BY ordinality
        ),
        '[]'::jsonb
    )
    FROM jsonb_array_elements(upstream_channels) WITH ORDINALITY AS item(channel, ordinality)
);
