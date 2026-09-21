BEGIN;
ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS agent_name text NOT NULL DEFAULT '';
ALTER TABLE agent_config_versions ADD COLUMN user_id uuid REFERENCES users(id);
UPDATE agent_config_versions SET user_id=created_by;
ALTER TABLE agent_config_versions ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE agent_config_versions DROP CONSTRAINT agent_config_versions_config_key_version_key;
ALTER TABLE agent_config_versions ADD UNIQUE(user_id,config_key,version);
DROP INDEX agent_config_one_active;
CREATE UNIQUE INDEX agent_config_one_active ON agent_config_versions(user_id,config_key) WHERE is_active;

CREATE FUNCTION set_agent_config_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.user_id IS NULL THEN NEW.user_id:=NEW.created_by; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER agent_config_set_owner BEFORE INSERT ON agent_config_versions
 FOR EACH ROW EXECUTE FUNCTION set_agent_config_owner();

CREATE OR REPLACE FUNCTION bind_task_base_prompt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO job_config_versions(job_id,config_key,config_version_id)
 SELECT NEW.id,config_key,id FROM agent_config_versions
 WHERE config_key='agent.loop' AND user_id=NEW.user_id AND is_active ON CONFLICT DO NOTHING;
 RETURN NEW;
END;
$$;
COMMIT;
