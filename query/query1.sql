WITH 
all_blocks AS (
    SELECT block_number, COUNT(*) AS event_count
    FROM blockchain_events
    GROUP BY block_number
),
last_24h_blocks AS (
    SELECT block_number, COUNT(*) AS event_count
    FROM blockchain_events
    WHERE insert_time >= NOW() - INTERVAL '24 hours'
    GROUP BY block_number
),
most_event_block AS (
    SELECT block_number, COUNT(*) AS event_count
    FROM blockchain_events
    GROUP BY block_number
    ORDER BY event_count DESC
    LIMIT 1
),
total_events AS (
    SELECT COUNT(*) AS count FROM blockchain_events
),
total_events_24h AS (
    SELECT COUNT(*) AS count 
    FROM blockchain_events
    WHERE insert_time >= NOW() - INTERVAL '24 hours'
)
SELECT
    (SELECT count FROM total_events) AS total_events,
    (SELECT count FROM total_events_24h) AS total_events_24h,
    
    -- Formatted numeric outputs
    (SELECT ROUND(SUM(event_count)::numeric / COUNT(*), 2)::text FROM all_blocks) AS avg_events_per_block,
    (SELECT 
        CASE 
            WHEN COUNT(*) = 0 THEN '0.00'
            ELSE ROUND(SUM(event_count)::numeric / COUNT(*), 2)::text
        END 
     FROM last_24h_blocks) AS avg_events_per_block_last_24h,

    (SELECT event_count FROM most_event_block) AS max_event_count,
    (SELECT 
        CASE
            WHEN COUNT(*) = 0 THEN 0
            ELSE MAX(event_count)
        END
     FROM last_24h_blocks) AS max_event_count_per_24h;