UPDATE server_connections
SET collect_interval_seconds = 60
WHERE collect_interval_seconds <> 60;

DELETE FROM tasks
WHERE id LIKE 'srvcol_%'
  AND id NOT IN (
    SELECT 'srvcol_' || id
    FROM server_connections
  );
