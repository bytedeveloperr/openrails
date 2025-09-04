-- -- 00002_billing_user_roles_interface.up.sql
-- -- Minimal billing ↔ authorization interface for simple user_roles schema

-- BEGIN;

-- -- Idempotency store for role mutations
-- CREATE TABLE IF NOT EXISTS user_role_idempotency_keys (
--   key UUID NOT NULL,
--   operation TEXT NOT NULL,
--   result_id UUID NULL,
--   created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
--   PRIMARY KEY (key, operation)
-- );

-- COMMENT ON TABLE user_role_idempotency_keys IS 'Stores idempotency outcomes for billing→authorization mutations.';

-- -- Ensure unique mapping to avoid duplicates
-- DO $$ BEGIN
--   ALTER TABLE user_roles
--     ADD CONSTRAINT ux_user_roles_user_role UNIQUE (user_id, role_id);
-- EXCEPTION WHEN duplicate_table OR duplicate_object THEN NULL; END $$;

-- -- Open/ensure a role grant (simple schema: insert if missing)
-- DO $$ BEGIN
-- CREATE OR REPLACE FUNCTION billing_open_or_ensure_user_role(
--   in_user_id UUID,
--   in_role_slug TEXT,
--   in_start_at TIMESTAMPTZ DEFAULT NULL,  -- ignored in simple mapping
--   in_end_at TIMESTAMPTZ DEFAULT NULL,    -- ignored in simple mapping
--   in_source_type TEXT DEFAULT NULL,      -- ignored; kept for API compatibility
--   in_source_id UUID DEFAULT NULL,        -- ignored; kept for API compatibility
--   in_idempotency_key UUID DEFAULT NULL
-- ) RETURNS TABLE(user_role_id UUID, action TEXT)
-- LANGUAGE plpgsql
-- SECURITY DEFINER
-- SET search_path = pg_catalog, public
-- AS $$
-- DECLARE v_role_id UUID; v_op TEXT := 'billing.open';
-- BEGIN
--   IF in_idempotency_key IS NOT NULL THEN
--     SELECT k.result_id INTO user_role_id FROM user_role_idempotency_keys k
--     WHERE k.key = in_idempotency_key AND k.operation = v_op;
--     IF user_role_id IS NOT NULL THEN action := 'idempotent'; RETURN; END IF;
--   END IF;

--   SELECT id INTO v_role_id FROM roles WHERE slug = in_role_slug;
--   IF v_role_id IS NULL THEN RAISE EXCEPTION 'Unknown role slug: %', in_role_slug; END IF;

--   -- Insert if missing, else fetch existing id
--   INSERT INTO user_roles (id, user_id, role_id, created_at, updated_at)
--   VALUES (gen_random_uuid(), in_user_id, v_role_id, current_timestamp, current_timestamp)
--   ON CONFLICT (user_id, role_id) DO UPDATE SET updated_at = EXCLUDED.updated_at
--   RETURNING id INTO user_role_id;
--   action := 'upserted';

--   IF in_idempotency_key IS NOT NULL THEN
--     BEGIN
--       INSERT INTO user_role_idempotency_keys(key, operation, result_id)
--       VALUES (in_idempotency_key, v_op, user_role_id);
--     EXCEPTION WHEN unique_violation THEN NULL; END;
--   END IF;
--   RETURN;
-- END; $$;
-- END $$;

-- -- Extend (no-op for simple mapping; just returns existing row id)
-- DO $$ BEGIN
-- CREATE OR REPLACE FUNCTION billing_extend_user_role(
--   in_user_id UUID,
--   in_role_slug TEXT,
--   in_new_end_at TIMESTAMPTZ DEFAULT NULL,
--   in_idempotency_key UUID DEFAULT NULL
-- ) RETURNS TABLE(user_role_id UUID, action TEXT)
-- LANGUAGE plpgsql
-- SECURITY DEFINER
-- SET search_path = pg_catalog, public
-- AS $$
-- DECLARE v_role_id UUID; v_op TEXT := 'billing.extend';
-- BEGIN
--   IF in_idempotency_key IS NOT NULL THEN
--     SELECT k.result_id INTO user_role_id FROM user_role_idempotency_keys k
--     WHERE k.key = in_idempotency_key AND k.operation = v_op;
--     IF user_role_id IS NOT NULL THEN action := 'idempotent'; RETURN; END IF;
--   END IF;

--   SELECT id INTO v_role_id FROM roles WHERE slug = in_role_slug;
--   IF v_role_id IS NULL THEN RAISE EXCEPTION 'Unknown role slug: %', in_role_slug; END IF;

--   SELECT id INTO user_role_id FROM user_roles WHERE user_id = in_user_id AND role_id = v_role_id;
--   IF user_role_id IS NULL THEN
--     -- behave like open
--     RETURN QUERY SELECT * FROM billing_open_or_ensure_user_role(in_user_id, in_role_slug, NULL, NULL, NULL, NULL, in_idempotency_key);
--     RETURN;
--   END IF;
--   action := 'noop';

--   IF in_idempotency_key IS NOT NULL THEN
--     BEGIN
--       INSERT INTO user_role_idempotency_keys(key, operation, result_id)
--       VALUES (in_idempotency_key, v_op, user_role_id);
--     EXCEPTION WHEN unique_violation THEN NULL; END;
--   END IF;
--   RETURN;
-- END; $$;
-- END $$;

-- -- Close (simple mapping: delete the row if present)
-- DO $$ BEGIN
-- CREATE OR REPLACE FUNCTION billing_close_user_role(
--   in_user_id UUID,
--   in_role_slug TEXT,
--   in_effective_at TIMESTAMPTZ DEFAULT NULL,
--   in_revoke_reason TEXT DEFAULT NULL,
--   in_idempotency_key UUID DEFAULT NULL
-- ) RETURNS TABLE(user_role_id UUID, action TEXT)
-- LANGUAGE plpgsql
-- SECURITY DEFINER
-- SET search_path = pg_catalog, public
-- AS $$
-- DECLARE v_role_id UUID; v_op TEXT := 'billing.close';
-- BEGIN
--   IF in_idempotency_key IS NOT NULL THEN
--     SELECT k.result_id INTO user_role_id FROM user_role_idempotency_keys k
--     WHERE k.key = in_idempotency_key AND k.operation = v_op;
--     IF user_role_id IS NOT NULL THEN action := 'idempotent'; RETURN; END IF;
--   END IF;

--   SELECT id INTO v_role_id FROM roles WHERE slug = in_role_slug;
--   IF v_role_id IS NULL THEN RAISE EXCEPTION 'Unknown role slug: %', in_role_slug; END IF;

--   SELECT id INTO user_role_id FROM user_roles WHERE user_id = in_user_id AND role_id = v_role_id;
--   IF user_role_id IS NULL THEN action := 'noop'; RETURN; END IF;

--   DELETE FROM user_roles WHERE id = user_role_id;
--   action := 'deleted';

--   IF in_idempotency_key IS NOT NULL THEN
--     BEGIN
--       INSERT INTO user_role_idempotency_keys(key, operation, result_id)
--       VALUES (in_idempotency_key, v_op, user_role_id);
--     EXCEPTION WHEN unique_violation THEN NULL; END;
--   END IF;
--   RETURN;
-- END; $$;
-- END $$;

-- -- Create billing_writer role for secure function access
-- DO $$ BEGIN
--   CREATE ROLE billing_writer;
-- EXCEPTION WHEN duplicate_object THEN
--   RAISE NOTICE 'Role billing_writer already exists, skipping creation';
-- END $$;

-- -- Grant execute permissions on the billing functions to billing_writer role
-- GRANT EXECUTE ON FUNCTION billing_open_or_ensure_user_role(UUID, TEXT, TIMESTAMPTZ, TIMESTAMPTZ, TEXT, UUID, UUID) TO billing_writer;
-- GRANT EXECUTE ON FUNCTION billing_extend_user_role(UUID, TEXT, TIMESTAMPTZ, UUID) TO billing_writer;
-- GRANT EXECUTE ON FUNCTION billing_close_user_role(UUID, TEXT, TIMESTAMPTZ, TEXT, UUID) TO billing_writer;

-- -- Grant necessary table permissions to billing_writer
-- GRANT SELECT ON roles TO billing_writer;
-- GRANT SELECT, INSERT, UPDATE ON user_roles TO billing_writer;
-- GRANT INSERT, SELECT ON user_role_idempotency_keys TO billing_writer;

-- -- Note: After running this migration, create a database user for your billing service:
-- -- CREATE USER billing_service_user WITH LOGIN PASSWORD 'your_secure_password';
-- -- GRANT billing_writer TO billing_service_user;

-- COMMIT;

