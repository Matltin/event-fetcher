-- Top 5 most frequent events of all time
SELECT
    event_name,
    COUNT(*) AS event_count,
    MIN(block_number) as first_block,
    MAX(block_number) as last_block
FROM blockchain_events
GROUP BY event_name
ORDER BY event_count DESC
LIMIT 5;
