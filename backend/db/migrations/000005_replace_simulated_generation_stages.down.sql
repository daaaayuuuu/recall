UPDATE game_generation_runs
SET stage = 'preparing_assets', progress = 0
WHERE status IN ('queued', 'running') AND stage <> 'completed';
