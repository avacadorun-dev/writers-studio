ALTER TABLE projects
    ADD COLUMN assistant_step_limit integer
        CHECK (assistant_step_limit IS NULL OR assistant_step_limit BETWEEN 1 AND 1000000);
