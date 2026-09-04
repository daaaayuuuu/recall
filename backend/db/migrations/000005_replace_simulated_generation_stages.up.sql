UPDATE game_generation_runs
SET stage = 'queued', progress = 0
WHERE status = 'queued' AND stage <> 'completed';

UPDATE game_generation_runs
SET stage = 'transforming_images', progress = 0
WHERE status = 'running' AND stage <> 'completed';
