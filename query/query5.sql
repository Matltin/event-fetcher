-- Unknown Events: Events without decoded names
SELECT
    event_signature,
    COUNT(*) as occurrences,
    MIN(block_number) as first_seen,
    MAX(block_number) as last_seen,
    array_agg(DISTINCT tx_hash) as sample_txs
FROM blockchain_events
WHERE event_name IS NULL OR event_signature IS NULL 
GROUP BY event_signature
ORDER BY occurrences DESC
LIMIT 10
;