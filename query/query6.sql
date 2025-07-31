-- Gives a daily count of events for trend analysis
SELECT 
    DATE(insert_time) AS day,
    COUNT(*) AS event_count
FROM blockchain_events
GROUP BY DATE(insert_time)
ORDER BY day DESC
LIMIT 30;
