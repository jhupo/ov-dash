UPDATE server_connections
SET collect_status = 'pending',
    collect_error = '采集状态已恢复',
    next_collect_at = now(),
    updated_at = now()
WHERE collect_status = 'collecting';
