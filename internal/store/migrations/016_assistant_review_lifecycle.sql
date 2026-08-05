UPDATE assistant_turns turn_state
SET
    state = CASE
        WHEN change_set.state IN ('stale', 'failed') THEN 'failed'
        ELSE 'completed'
    END,
    error_code = CASE
        WHEN change_set.state = 'stale' THEN COALESCE(turn_state.error_code, 'stale_change_set')
        WHEN change_set.state = 'failed' THEN COALESCE(turn_state.error_code, 'change_set_failed')
        ELSE turn_state.error_code
    END,
    completed_at = COALESCE(turn_state.completed_at, change_set.updated_at)
FROM assistant_change_sets change_set
WHERE change_set.turn_id = turn_state.id
    AND change_set.state IN ('applied', 'rejected', 'stale', 'failed', 'undone')
    AND turn_state.state = 'awaiting_review';

UPDATE assistant_runs run_state
SET status='active', error_code=NULL, ended_at=NULL, updated_at=now()
WHERE run_state.status='awaiting_review'
    AND NOT EXISTS (
        SELECT 1
        FROM assistant_change_sets change_set
        WHERE change_set.run_id=run_state.id AND change_set.state='staged'
    );
