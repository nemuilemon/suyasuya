BEGIN;

-- Every task binds the shared base at creation, including future task kinds.
CREATE FUNCTION bind_task_base_prompt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO job_config_versions(job_id,config_key,config_version_id)
 SELECT NEW.id,config_key,id FROM agent_config_versions
 WHERE config_key='agent.loop' AND is_active
 ON CONFLICT DO NOTHING;
 RETURN NEW;
END;
$$;
CREATE TRIGGER jobs_bind_base_prompt AFTER INSERT ON jobs
 FOR EACH ROW EXECUTE FUNCTION bind_task_base_prompt();

-- Keep historical successful tasks unchanged; bind a base for pending/retryable jobs.
INSERT INTO job_config_versions(job_id,config_key,config_version_id)
 SELECT j.id,c.config_key,c.id FROM jobs j CROSS JOIN agent_config_versions c
 WHERE j.status <> 'succeeded' AND c.config_key='agent.loop' AND c.is_active
 ON CONFLICT DO NOTHING;
COMMIT;
