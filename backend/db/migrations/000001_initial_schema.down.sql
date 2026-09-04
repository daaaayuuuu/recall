DROP TABLE IF EXISTS game_deletion_jobs;
DROP TABLE IF EXISTS admin_audit_logs;
DROP TABLE IF EXISTS play_sessions;
DROP TABLE IF EXISTS share_links;

ALTER TABLE games
    DROP FOREIGN KEY fk_games_current_generation_run,
    DROP FOREIGN KEY fk_games_current_version;

DROP TABLE IF EXISTS game_generation_runs;
DROP TABLE IF EXISTS game_version_assets;
DROP TABLE IF EXISTS game_versions;
DROP TABLE IF EXISTS games;

ALTER TABLE users DROP FOREIGN KEY fk_users_avatar_asset;

DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS admin_sessions;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS users;
